// internal/sync/sync.go
package sync

import (
	"context"
	"log/slog"

	"github.com/kbuechl/audible-hardcover-sync/internal/audible"
	"github.com/kbuechl/audible-hardcover-sync/internal/hardcover"
	"github.com/kbuechl/audible-hardcover-sync/internal/matcher"
	"github.com/kbuechl/audible-hardcover-sync/internal/state"
)

// AudibleClient is the subset of the audible package's client needed to
// run a sync pass, narrowed so this package can be tested against a fake.
type AudibleClient interface {
	FetchLibrary(ctx context.Context) ([]audible.LibraryItem, error)
}

// HardcoverClient is the subset of the hardcover package's client needed to
// run a sync pass, narrowed so this package can be tested against a fake.
type HardcoverClient interface {
	FindBookByISBN(ctx context.Context, isbn string) (int64, bool, error)
	SearchBooks(ctx context.Context, query string, perPage int) ([]hardcover.SearchResult, error)
	UpsertUserBook(ctx context.Context, bookID int64, statusID int, rating *float64) error
}

// Notifier delivers a notification for library items that could not be
// matched to a Hardcover book via ISBN. internal/notify implements this
// for real; sync.Run only depends on the interface shape.
type Notifier interface {
	NotifyUnmatched(ctx context.Context, items []state.BookState) error
}

// Deps bundles Run's dependencies so they can be swapped for fakes in tests.
type Deps struct {
	Audible   AudibleClient
	Hardcover HardcoverClient
	Store     *state.Store
	Notifier  Notifier
	// MatchConfidence is the minimum matcher.BestMatch score (0.0-1.0)
	// required to auto-accept a fuzzy title/author match when ISBN
	// resolution fails, and to write it to the user's Hardcover library
	// unattended. Lower values (e.g. the "loose" preset, 0.55) trade
	// recall for a real, if modest, false-positive risk on unrelated
	// short/common-word titles — see internal/matcher's confidence
	// scoring docs. This is a deliberate per-user choice; the default
	// ("strict", 0.95) is nowhere near that risk zone.
	MatchConfidence float64
}

// Result summarizes the outcome of a sync pass.
type Result struct {
	Synced    int
	Unchanged int
	Unmatched int
	Errored   int
}

// Run performs one sync pass: for each Audible library item, it computes
// the desired Hardcover status, diffs it against stored state, resolves a
// Hardcover book ID (reuse of a previously resolved cached ID, ISBN-exact
// match, or a confidence-scored fuzzy fallback when ISBN matching produces
// nothing), upserts the Hardcover reading status, and persists the new
// state. Items that cannot be matched are marked unmatched and, if not
// already notified, batched into a single notification at the end of the
// pass. A per-book failure increments Result.Errored and is logged, but
// never fails the run — only a failure to fetch the Audible library at all
// returns a non-nil error.
func Run(ctx context.Context, d Deps) (Result, error) {
	var result Result

	items, err := d.Audible.FetchLibrary(ctx)
	if err != nil {
		return result, err
	}

	var newlyUnmatched []state.BookState

	for _, item := range items {
		status, statusID, ok := desiredStatus(item)
		if !ok {
			continue // not started yet — nothing to sync
		}

		prior, err := d.Store.Get(item.ASIN)
		if err != nil {
			slog.Error("read prior state", "asin", item.ASIN, "error", err)
			result.Errored++
			continue
		}

		if prior != nil && !prior.Unmatched && prior.Status == status {
			result.Unchanged++
			continue
		}

		bookID, resolvedErr := resolveBookID(ctx, d, item, prior)
		if resolvedErr != nil {
			slog.Error("resolve hardcover book id", "asin", item.ASIN, "error", resolvedErr)
			result.Errored++
			continue
		}
		if bookID == 0 {
			bs := state.BookState{
				ASIN:              item.ASIN,
				Title:             item.Title,
				Author:            item.Author,
				Status:            status,
				PercentComplete:   item.PercentComplete,
				Unmatched:         true,
				NotifiedUnmatched: prior != nil && prior.NotifiedUnmatched,
			}
			if err := d.Store.Upsert(bs); err != nil {
				slog.Error("persist unmatched state", "asin", item.ASIN, "error", err)
				result.Errored++
				continue
			}
			result.Unmatched++
			if !bs.NotifiedUnmatched {
				newlyUnmatched = append(newlyUnmatched, bs)
			}
			continue
		}

		if err := d.Hardcover.UpsertUserBook(ctx, bookID, statusID, nil); err != nil {
			slog.Error("upsert hardcover user_book", "asin", item.ASIN, "book_id", bookID, "error", err)
			result.Errored++
			continue
		}

		if err := d.Store.Upsert(state.BookState{
			ASIN:            item.ASIN,
			Status:          status,
			PercentComplete: item.PercentComplete,
			HardcoverBookID: &bookID,
		}); err != nil {
			slog.Error("persist synced state", "asin", item.ASIN, "error", err)
			result.Errored++
			continue
		}
		result.Synced++
	}

	if len(newlyUnmatched) > 0 {
		if err := d.Notifier.NotifyUnmatched(ctx, newlyUnmatched); err != nil {
			slog.Error("send unmatched notification", "error", err)
		} else {
			for _, bs := range newlyUnmatched {
				if err := d.Store.MarkNotified(bs.ASIN); err != nil {
					slog.Error("mark notified", "asin", bs.ASIN, "error", err)
				}
			}
		}
	}

	return result, nil
}

func desiredStatus(item audible.LibraryItem) (status state.Status, statusID int, ok bool) {
	switch {
	case item.IsFinished:
		return state.StatusFinished, hardcover.StatusRead, true
	case item.PercentComplete > 0:
		return state.StatusInProgress, hardcover.StatusCurrentlyReading, true
	default:
		return state.StatusNotStarted, 0, false
	}
}

// resolveBookID returns 0 (with no error) when the item cannot be matched
// and should be routed to the unmatched flow — never an error in that case.
func resolveBookID(ctx context.Context, d Deps, item audible.LibraryItem, prior *state.BookState) (int64, error) {
	if prior != nil && prior.HardcoverBookID != nil {
		return *prior.HardcoverBookID, nil
	}

	isbn, ok := matcher.BestISBN(matcher.AudibleItem{ASIN: item.ASIN, ISBN13: item.ISBN13, ISBN10: item.ISBN10})
	if ok {
		id, found, err := d.Hardcover.FindBookByISBN(ctx, isbn)
		if err != nil {
			return 0, err
		}
		if found {
			return id, nil
		}
	}

	// No ISBN, or the ISBN wasn't found in Hardcover's catalog (common for
	// audiobooks, which are frequently catalogued by ASIN with no linked
	// print/ebook ISBN) — try a confidence-scored fuzzy fallback before
	// giving up to the manual-match path.
	return resolveByFuzzyMatch(ctx, d, item)
}

// resolveByFuzzyMatch searches Hardcover by title and, if the best-scoring
// candidate clears d.MatchConfidence, returns its book ID. Returns 0 (with
// no error) whenever there's no confident match, routing the item to the
// unmatched/manual-match flow exactly like a failed ISBN lookup.
func resolveByFuzzyMatch(ctx context.Context, d Deps, item audible.LibraryItem) (int64, error) {
	results, err := d.Hardcover.SearchBooks(ctx, item.Title, 10)
	if err != nil {
		return 0, err
	}
	if len(results) == 0 {
		return 0, nil
	}

	candidates := make([]matcher.CandidateInfo, len(results))
	for i, r := range results {
		author := ""
		if len(r.AuthorNames) > 0 {
			author = r.AuthorNames[0]
		}
		candidates[i] = matcher.CandidateInfo{Title: r.Title, Author: author}
	}

	idx, score, ok := matcher.BestMatch(item.Title, item.Author, candidates)
	if !ok || score < d.MatchConfidence {
		return 0, nil
	}
	return results[idx].ID, nil
}
