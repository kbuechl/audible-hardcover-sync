package app

import (
	"fmt"
	"strings"

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
