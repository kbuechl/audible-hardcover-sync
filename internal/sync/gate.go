package sync

import (
	"context"
	"log/slog"
	"slices"

	"github.com/kbuechl/audible-hardcover-sync/internal/audible"
	"github.com/kbuechl/audible-hardcover-sync/internal/state"
)

// Sync modes (Preferences: "Which books sync?"). An empty mode is treated
// as ModeAll so callers that never set one keep today's behavior.
const (
	ModeAll           = "all"
	ModeSkipShared    = "skip_shared"
	ModeConfirmShared = "confirm_shared"
	ModeConfirmAll    = "confirm_all"
)

// Held reports whether an undecided book waits for the user's review under
// mode. It is the one definition of "waiting" used by the sync, the review
// notification, the Review filter and /api/status.
func Held(mode string, shared bool, d state.Decision) bool {
	if d != state.DecisionNone {
		return false
	}
	return mode == ModeConfirmAll || (mode == ModeConfirmShared && shared)
}

// allowed reports whether the sync may write item to Hardcover. A per-book
// decision always beats the mode.
func allowed(mode string, item audible.LibraryItem, rb *state.ReviewBook) bool {
	d := decisionOf(rb)
	switch d {
	case state.DecisionSync:
		return true
	case state.DecisionNotWanted:
		return false
	}
	if Held(mode, item.Shared, d) {
		return false
	}
	return !(mode == ModeSkipShared && item.Shared)
}

func decisionOf(rb *state.ReviewBook) state.Decision {
	if rb == nil {
		return state.DecisionNone
	}
	return rb.Decision
}

// seeReviewBook records item in the review list and returns its row. A
// failure is logged; the returned nil row then counts as undecided.
func seeReviewBook(d Deps, item audible.LibraryItem, status state.Status) *state.ReviewBook {
	err := d.Store.SeeReviewBook(state.ReviewBook{
		ASIN: item.ASIN, Title: item.Title, Author: item.Author, CoverURL: item.CoverURL,
		Shared: item.Shared, Status: status, PercentComplete: item.PercentComplete,
	})
	if err != nil {
		slog.Error("record review book", "asin", item.ASIN, "error", err)
		return nil
	}
	rb, err := d.Store.ReviewBook(item.ASIN)
	if err != nil {
		slog.Error("read review book", "asin", item.ASIN, "error", err)
		return nil
	}
	return rb
}

// markSynced gives an undecided book the Sync decision once it is on
// Hardcover, so a later mode change never holds it back.
func markSynced(d Deps, item audible.LibraryItem, rb *state.ReviewBook) {
	if decisionOf(rb) != state.DecisionNone {
		return
	}
	if err := d.Store.SetDecision([]string{item.ASIN}, state.DecisionSync); err != nil {
		slog.Error("record sync decision", "asin", item.ASIN, "error", err)
	}
}

// recordAction logs a Hardcover write for "Remove from Hardcover". A
// failure is only logged.
func recordAction(d Deps, a state.SyncAction) {
	if err := d.Store.RecordAction(a); err != nil {
		slog.Error("record sync action", "asin", a.ASIN, "kind", a.Kind, "error", err)
	}
}

// readIDs lists a user_book's reads; ok is false (and the error logged)
// when Hardcover could not be asked.
func readIDs(ctx context.Context, d Deps, item audible.LibraryItem, userBookID int64) ([]int64, bool) {
	ids, err := d.Hardcover.ReadIDs(ctx, userBookID)
	if err != nil {
		slog.Error("list hardcover reads", "asin", item.ASIN, "user_book_id", userBookID, "error", err)
		return nil, false
	}
	return ids, true
}

// createdSince returns the ids in after that are not in before.
func createdSince(before, after []int64) []int64 {
	var out []int64
	for _, id := range after {
		if !slices.Contains(before, id) {
			out = append(out, id)
		}
	}
	return out
}
