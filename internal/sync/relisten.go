package sync

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/kbuechl/audible-hardcover-sync/internal/audible"
	"github.com/kbuechl/audible-hardcover-sync/internal/hardcover"
	"github.com/kbuechl/audible-hardcover-sync/internal/state"
)

// relistenRestartPercent is the highest Audible percent at which an
// unfinished, previously finished book counts as restarted. A real
// re-listen starts near the beginning; jumping back after falling asleep
// past the end lands mid-book and must not count.
const relistenRestartPercent = 10

// tailReplaySeconds is how close to the end an unfinished book with an
// Audible finish timestamp can be and still count as finished. Playing the
// last minutes of a finished book (or a preview autoplaying after it)
// flips it to unfinished on Audible without changing its percent.
const tailReplaySeconds = 5 * 60

// now is swapped in tests.
var now = time.Now

// isTailReplay reports whether an unfinished book was finished before and
// is still within tailReplaySeconds of the end.
func isTailReplay(item audible.LibraryItem) bool {
	if item.IsFinished || item.FinishedAt == nil || item.RuntimeSeconds <= 0 {
		return false
	}
	remaining := float64(item.RuntimeSeconds) * (100 - item.PercentComplete) / 100
	return remaining < tailReplaySeconds
}

// relistenStart reports whether a book that is Read on Hardcover is being
// listened to again on Audible, and when that listen started. Hardcover's
// Read status is the source of truth for "finished"; the evidence for a
// new listen comes only from Audible, so progress synced to Hardcover from
// elsewhere (Kobo, CWA) never starts one. Two cases count:
//
//  1. Re-listen after an Audible finish: Audible restamps
//     finished_at_timestamp when a finished book is restarted, so a stamp
//     newer than Hardcover's last finish, with the book back near the
//     beginning, marks the restart.
//  2. First listen after reading elsewhere: Audible never finished the
//     book (no stamp) and its percent went up since the last sync.
func relistenStart(item audible.LibraryItem, status state.Status, existing hardcover.UserBook, prior *state.BookState) (time.Time, bool) {
	if existing.StatusID != hardcover.StatusRead || status != state.StatusInProgress {
		return time.Time{}, false
	}
	if item.FinishedAt != nil {
		newer := existing.LastReadDate == nil || item.FinishedAt.After(*existing.LastReadDate)
		if newer && item.PercentComplete <= relistenRestartPercent {
			return *item.FinishedAt, true
		}
		return time.Time{}, false
	}
	if prior != nil && !prior.Unmatched && item.PercentComplete > prior.PercentComplete {
		return now(), true
	}
	return time.Time{}, false
}

// hardcoverSeconds converts Audible's percent into seconds of the
// Hardcover edition, falling back to Audible's runtime.
func hardcoverSeconds(item audible.LibraryItem, existing hardcover.UserBook) int {
	if existing.AudioSeconds > 0 {
		return hardcover.ProgressSecondsFor(item.PercentComplete, existing.AudioSeconds)
	}
	return progressSeconds(item)
}

// startRelisten opens a new read on Hardcover and remembers its id. The
// book's status on Hardcover is left as Read.
func startRelisten(ctx context.Context, d Deps, item audible.LibraryItem, status state.Status, existing hardcover.UserBook, bookID int64, startedAt time.Time) bool {
	readID, err := d.Hardcover.StartRead(ctx, existing.ID, startedAt, hardcoverSeconds(item, existing))
	if err != nil {
		slog.Error("start hardcover re-listen", "asin", item.ASIN, "book_id", bookID, "error", err)
		return false
	}
	slog.Info("re-listen started", "asin", item.ASIN, "title", item.Title, "read_id", readID)
	persist(d, item, status, item.PercentComplete, bookID, &readID)
	return true
}

// continueRelisten moves an open re-listen read forward, or finishes it
// when Audible says the book is finished again.
func continueRelisten(ctx context.Context, d Deps, item audible.LibraryItem, status state.Status, existing hardcover.UserBook, prior *state.BookState, bookID int64) bool {
	readID := *prior.RelistenReadID
	if !slices.Contains(existing.OpenReadIDs, readID) {
		// The read was finished or deleted on Hardcover, e.g. by CWA when
		// the book was finished on an e-reader (CWA writes to the newest
		// open read). Hardcover's answer wins: stop tracking it.
		slog.Info("re-listen read closed on Hardcover", "asin", item.ASIN, "read_id", readID)
		persist(d, item, status, item.PercentComplete, bookID, nil)
		return false
	}
	if status == state.StatusFinished {
		finishedAt := now()
		if item.FinishedAt != nil {
			finishedAt = *item.FinishedAt
		}
		if err := d.Hardcover.FinishRead(ctx, readID, finishedAt); err != nil {
			slog.Error("finish hardcover re-listen", "asin", item.ASIN, "read_id", readID, "error", err)
			return false
		}
		slog.Info("re-listen finished", "asin", item.ASIN, "title", item.Title, "read_id", readID)
		persist(d, item, status, item.PercentComplete, bookID, nil)
		return true
	}
	if item.PercentComplete <= prior.PercentComplete {
		return false
	}
	if err := d.Hardcover.UpdateReadProgress(ctx, readID, hardcoverSeconds(item, existing)); err != nil {
		slog.Error("set hardcover re-listen progress", "asin", item.ASIN, "read_id", readID, "error", err)
		return false
	}
	persist(d, item, status, item.PercentComplete, bookID, &readID)
	return true
}

// syncExisting handles a book that is already in the user's Hardcover
// library. Its status, rating and dates are never changed; only progress
// and re-listen reads are written. It reports whether Hardcover changed.
func syncExisting(ctx context.Context, d Deps, item audible.LibraryItem, status state.Status, existing hardcover.UserBook, prior *state.BookState, bookID int64) bool {
	if existing.StatusID == hardcover.StatusWantToRead {
		return promoteWantToRead(ctx, d, item, status, existing, bookID)
	}
	if prior != nil && prior.RelistenReadID != nil {
		return continueRelisten(ctx, d, item, status, existing, prior, bookID)
	}
	if startedAt, ok := relistenStart(item, status, existing, prior); ok {
		return startRelisten(ctx, d, item, status, existing, bookID, startedAt)
	}
	synced, failed := pushExistingProgress(ctx, d, item, status, existing, bookID)
	if !failed {
		// Recording the percent even when nothing was pushed gives the
		// next sync a baseline to detect a new listen against.
		persist(d, item, status, item.PercentComplete, bookID, nil)
	}
	return synced
}

func persist(d Deps, item audible.LibraryItem, status state.Status, percent float64, bookID int64, relistenReadID *int64) {
	err := d.Store.Upsert(state.BookState{
		ASIN:            item.ASIN,
		Title:           item.Title,
		Author:          item.Author,
		Status:          status,
		PercentComplete: percent,
		HardcoverBookID: &bookID,
		CoverURL:        item.CoverURL,
		RelistenReadID:  relistenReadID,
	})
	if err != nil {
		slog.Error("persist synced state", "asin", item.ASIN, "error", err)
	}
}

// promoteWantToRead moves a book shelved as Want to Read on Hardcover to
// match Audible once listening has started: Currently Reading with
// progress, or Read with the finish date and rating. It reports whether
// Hardcover was updated. On failure nothing is recorded, so the next sync
// retries.
func promoteWantToRead(ctx context.Context, d Deps, item audible.LibraryItem, status state.Status, existing hardcover.UserBook, bookID int64) bool {
	statusID := hardcover.StatusCurrentlyReading
	var rating *int
	var userDate *time.Time
	if status == state.StatusFinished {
		statusID, rating, userDate = hardcover.StatusRead, item.Rating, item.FinishedAt
	}
	if err := d.Hardcover.UpdateUserBookStatus(ctx, existing.ID, statusID, rating, userDate); err != nil {
		slog.Error("move hardcover book off want to read", "asin", item.ASIN, "book_id", bookID, "error", err)
		return false
	}
	slog.Info("moved hardcover book off want to read", "title", item.Title, "book_id", bookID, "status", status)

	percent := item.PercentComplete
	switch {
	case status == state.StatusFinished && userDate == nil:
		// Hardcover fills in today when a book becomes Read without a date.
		if err := d.Hardcover.ClearFinishDate(ctx, existing.ID); err != nil {
			slog.Error("clear hardcover finish date", "asin", item.ASIN, "book_id", bookID, "error", err)
		}
	case status == state.StatusInProgress:
		if secs := progressSeconds(item); secs > 0 {
			if err := d.Hardcover.SetReadProgress(ctx, existing.ID, item.PercentComplete, secs); err != nil {
				slog.Error("set hardcover progress", "asin", item.ASIN, "book_id", bookID, "error", err)
				percent = 0 // retried on the next sync, like a new book
			}
		}
	}
	persist(d, item, status, percent, bookID, nil)
	return true
}
