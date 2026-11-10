package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kbuechl/audible-hardcover-sync/internal/hardcover"

	"github.com/kbuechl/audible-hardcover-sync/internal/state"
	syncpkg "github.com/kbuechl/audible-hardcover-sync/internal/sync"
)

// Review list filters.
const (
	ReviewWaiting   = "waiting"
	ReviewSynced    = "synced"
	ReviewNotWanted = "not_wanted"
)

// ReviewItem is one book on the Review page.
type ReviewItem struct {
	ASIN            string
	Title, Author   string
	CoverURL        string
	Shared          bool
	Status          state.Status
	PercentComplete float64
	Decision        state.Decision
	CanRemove       bool // the sync has written this book to Hardcover
}

// ReviewBooks lists books for the Review page. filter is waiting, synced
// or not_wanted; q is a case-insensitive title substring.
func (a *App) ReviewBooks(filter, q string) ([]ReviewItem, error) {
	var books []state.ReviewBook
	var err error
	switch filter {
	case ReviewSynced:
		books, err = a.store.ListReview(state.DecisionSync)
	case ReviewNotWanted:
		books, err = a.store.ListReview(state.DecisionNotWanted)
	case ReviewWaiting:
		books, err = a.waitingBooks()
	default:
		return nil, fmt.Errorf("unknown review filter %q", filter)
	}
	if err != nil {
		return nil, err
	}
	q = strings.ToLower(strings.TrimSpace(q))
	items := make([]ReviewItem, 0, len(books))
	for _, b := range books {
		if q != "" && !strings.Contains(strings.ToLower(b.Title), q) {
			continue
		}
		can, err := a.store.HasActions(b.ASIN)
		if err != nil {
			return nil, err
		}
		items = append(items, ReviewItem{
			ASIN: b.ASIN, Title: b.Title, Author: b.Author, CoverURL: b.CoverURL,
			Shared: b.Shared, Status: b.Status, PercentComplete: b.PercentComplete,
			Decision: b.Decision, CanRemove: can,
		})
	}
	return items, nil
}

// waitingBooks returns the undecided books the current mode holds.
func (a *App) waitingBooks() ([]state.ReviewBook, error) {
	p, err := a.Preferences()
	if err != nil {
		return nil, err
	}
	undecided, err := a.store.ListReview(state.DecisionNone)
	if err != nil {
		return nil, err
	}
	var out []state.ReviewBook
	for _, b := range undecided {
		if syncpkg.Held(p.SyncMode, b.Shared, b.Decision) {
			out = append(out, b)
		}
	}
	return out, nil
}

// WaitingCount is the number of books waiting for the user's review.
func (a *App) WaitingCount() (int, error) {
	books, err := a.waitingBooks()
	return len(books), err
}

// SetReviewDecision records d for every ASIN. An unknown ASIN wraps
// state.ErrUnknownASIN and changes nothing.
func (a *App) SetReviewDecision(asins []string, d state.Decision) error {
	return a.store.SetDecision(asins, d)
}

// HiddenFromMatches returns the ASINs among asins that the Matches list
// should not show: Not wanted books, and books held under the current mode.
func (a *App) HiddenFromMatches(asins []string) (map[string]bool, error) {
	p, err := a.Preferences()
	if err != nil {
		return nil, err
	}
	hidden := map[string]bool{}
	for _, asin := range asins {
		rb, err := a.store.ReviewBook(asin)
		if err != nil {
			return nil, err
		}
		// A book with no row is undecided and not shared.
		shared, d := false, state.DecisionNone
		if rb != nil {
			shared, d = rb.Shared, rb.Decision
		}
		if d == state.DecisionNotWanted || syncpkg.Held(p.SyncMode, shared, d) {
			hidden[asin] = true
		}
	}
	return hidden, nil
}

// ErrNothingToRemove means the sync has logged no Hardcover write for the
// book, so there is nothing for Remove to undo.
var ErrNothingToRemove = errors.New("the sync has not added this book to Hardcover")

// hardcoverRemover is the part of the Hardcover client Remove uses.
type hardcoverRemover interface {
	UserBookDetails(ctx context.Context, userBookID int64) (*hardcover.UserBookDetails, error)
	DeleteUserBook(ctx context.Context, userBookID int64) error
	DeleteRead(ctx context.Context, readID int64) error
	UpdateUserBookStatus(ctx context.Context, userBookID int64, statusID int, rating *int, userDate *time.Time) ([]int64, error)
}

func (a *App) remover() (hardcoverRemover, error) {
	if a.removeHC != nil {
		return a.removeHC, nil
	}
	hc := a.Hardcover()
	if hc == nil {
		return nil, errors.New("connect Hardcover first")
	}
	return hc, nil
}

// RemovePreview tells the user what Remove will take off Hardcover:
// Deletes is what the sync wrote, AlsoDeletes is data the user added
// themselves that goes with it.
type RemovePreview struct {
	Deletes     []string
	AlsoDeletes []string
}

// actionsFor returns the logged writes for asin, or ErrNothingToRemove.
func (a *App) actionsFor(asin string) ([]state.SyncAction, error) {
	acts, err := a.store.Actions(asin)
	if err != nil {
		return nil, err
	}
	if len(acts) == 0 {
		return nil, ErrNothingToRemove
	}
	return acts, nil
}

// PreviewRemove describes what Remove(asin) would delete.
func (a *App) PreviewRemove(ctx context.Context, asin string) (RemovePreview, error) {
	acts, err := a.actionsFor(asin)
	if err != nil {
		return RemovePreview{}, err
	}
	hc, err := a.remover()
	if err != nil {
		return RemovePreview{}, err
	}
	p := RemovePreview{Deletes: []string{}, AlsoDeletes: []string{}}
	var insert *state.SyncAction
	for i := len(acts) - 1; i >= 0; i-- {
		switch acts[i].Kind {
		case state.ActionReadStarted:
			p.Deletes = append(p.Deletes, "the re-listen read the sync started")
		case state.ActionPromoted:
			p.Deletes = append(p.Deletes, "the reads the sync added, and moves the book back to "+statusName(acts[i].PrevStatusID))
		case state.ActionInserted:
			insert = &acts[i]
		}
	}
	// Only deleting the whole entry takes the user's own data with it.
	if insert == nil {
		return p, nil
	}
	d, err := hc.UserBookDetails(ctx, insert.UserBookID)
	if err != nil || d == nil {
		return p, err // nil, nil: the entry is already gone
	}
	// The insert is the oldest action, so it is listed last.
	p.Deletes = append(p.Deletes, "the book from your Hardcover library")
	p.AlsoDeletes = userData(acts, d)
	return p, nil
}

// userData lists what on the entry d the logged actions did not write.
func userData(acts []state.SyncAction, d *hardcover.UserBookDetails) []string {
	out := []string{}
	var sent *int
	ours := map[int64]bool{}
	for _, act := range acts {
		if act.Rating != nil {
			sent = act.Rating
		}
		if act.ReadID != 0 {
			ours[act.ReadID] = true
		}
		for _, id := range act.CreatedReadIDs {
			ours[id] = true
		}
	}
	if d.Rating != nil && (sent == nil || float64(*sent) != *d.Rating) {
		out = append(out, fmt.Sprintf("your %s-star rating", strconv.FormatFloat(*d.Rating, 'f', -1, 64)))
	}
	if d.HasReview {
		out = append(out, "your review")
	}
	if d.HasNotes {
		out = append(out, "your private notes")
	}
	n := 0
	for _, id := range d.ReadIDs {
		if !ours[id] {
			n++
		}
	}
	switch {
	case n == 1:
		out = append(out, "1 read you added")
	case n > 1:
		out = append(out, fmt.Sprintf("%d reads you added", n))
	}
	return out
}

// Remove undoes the sync's logged Hardcover writes for asin, newest first,
// then forgets them and the book's sync state and marks it Not wanted so
// the next sync leaves it alone. Rows already gone on Hardcover count as removed. A Hardcover
// error stops the walk and keeps the log and decision, so Remove can be
// retried.
func (a *App) Remove(ctx context.Context, asin string) error {
	if !a.running.TryLock() {
		return ErrSyncRunning
	}
	defer a.running.Unlock()

	acts, err := a.actionsFor(asin)
	if err != nil {
		return err
	}
	hc, err := a.remover()
	if err != nil {
		return err
	}
	for i := len(acts) - 1; i >= 0; i-- {
		if err := undo(ctx, hc, acts[i]); err != nil {
			return fmt.Errorf("remove %q from Hardcover: %w", asin, err)
		}
	}
	if err := a.store.ClearActions(asin); err != nil {
		return err
	}
	// Without its sync state the book counts as new, so turning it back
	// to Sync adds it again instead of looking unchanged.
	if err := a.store.Forget(asin); err != nil {
		return err
	}
	return a.store.SetDecision([]string{asin}, state.DecisionNotWanted)
}

// statusName is the shelf name Remove moves a promoted book back to.
func statusName(statusID int) string {
	if statusID == hardcover.StatusCurrentlyReading {
		return "Currently Reading"
	}
	return "Want to Read"
}

// undo reverses one logged write.
func undo(ctx context.Context, hc hardcoverRemover, act state.SyncAction) error {
	switch act.Kind {
	case state.ActionReadStarted:
		return hc.DeleteRead(ctx, act.ReadID)
	case state.ActionInserted:
		return hc.DeleteUserBook(ctx, act.UserBookID)
	case state.ActionPromoted:
		// Only reads the promotion created; the user's own reads stay.
		for _, id := range act.CreatedReadIDs {
			if err := hc.DeleteRead(ctx, id); err != nil {
				return err
			}
		}
		d, err := hc.UserBookDetails(ctx, act.UserBookID)
		if err != nil || d == nil {
			return err // nil, nil: the entry is gone, nothing to move back
		}
		_, err = hc.UpdateUserBookStatus(ctx, act.UserBookID, act.PrevStatusID, nil, nil)
		return err
	}
	return fmt.Errorf("unknown action kind %q", act.Kind)
}
