// internal/sync/sync_test.go
package sync

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kbuechl/audible-hardcover-sync/internal/audible"
	"github.com/kbuechl/audible-hardcover-sync/internal/hardcover"
	"github.com/kbuechl/audible-hardcover-sync/internal/state"
)

type fakeAudible struct {
	items    []audible.LibraryItem
	fetchErr error
}

func (f *fakeAudible) FetchLibrary(ctx context.Context) ([]audible.LibraryItem, error) {
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	return f.items, nil
}

type upsertCall struct {
	statusID  int
	rating    *int
	userDate  *time.Time
	editionID *int64
}

type fakeHardcover struct {
	byISBN             map[string]int64
	editionByISBN      map[string]int64 // ISBN → edition_id returned by FindBookByISBN
	searchResults      map[string][]hardcover.SearchResult
	audioEditionByBook map[int64]int64 // bookID → edition_id returned by FindDefaultEdition
	searchErr          error
	userBooks          map[int64]hardcover.UserBook // pre-seeded cache for FetchUserBooks
	inserted           map[int64]upsertCall         // bookID → call
	upsertErr          error
}

func newFakeHardcover() *fakeHardcover {
	return &fakeHardcover{
		byISBN:             map[string]int64{},
		editionByISBN:      map[string]int64{},
		searchResults:      map[string][]hardcover.SearchResult{},
		audioEditionByBook: map[int64]int64{},
		userBooks:          map[int64]hardcover.UserBook{},
		inserted:           map[int64]upsertCall{},
	}
}

func (f *fakeHardcover) FindBookByISBN(ctx context.Context, isbn, asin string) (hardcover.BookMatch, bool, error) {
	id, ok := f.byISBN[isbn]
	if !ok {
		return hardcover.BookMatch{}, false, nil
	}
	var editionID *int64
	if ed, ok := f.editionByISBN[isbn]; ok {
		editionID = int64Ptr(ed)
	}
	return hardcover.BookMatch{BookID: id, EditionID: editionID}, true, nil
}

func (f *fakeHardcover) SearchBooks(ctx context.Context, query string, perPage int) ([]hardcover.SearchResult, error) {
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.searchResults[query], nil
}

func (f *fakeHardcover) FetchUserBooks(ctx context.Context) (map[int64]hardcover.UserBook, error) {
	out := make(map[int64]hardcover.UserBook, len(f.userBooks))
	for k, v := range f.userBooks {
		out[k] = v
	}
	return out, nil
}

func (f *fakeHardcover) FindDefaultEdition(ctx context.Context, bookID int64) (*int64, error) {
	if ed, ok := f.audioEditionByBook[bookID]; ok {
		return int64Ptr(ed), nil
	}
	return nil, nil // tests don't need edition resolution
}

func (f *fakeHardcover) InsertUserBook(ctx context.Context, bookID int64, statusID int, rating *int, userDate *time.Time, editionID *int64) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.inserted[bookID] = upsertCall{statusID: statusID, rating: rating, userDate: userDate, editionID: editionID}
	return nil
}

type fakeNotifier struct {
	notified []state.BookState
}

func (f *fakeNotifier) NotifyUnmatched(ctx context.Context, items []state.BookState) error {
	f.notified = append(f.notified, items...)
	return nil
}

func newTestStore(t *testing.T) *state.Store {
	t.Helper()
	s, err := state.Open(":memory:")
	if err != nil {
		t.Fatalf("state.Open() error = %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func int64Ptr(v int64) *int64 { return &v }

func intPtr(v int) *int { return &v }

func ptrTime(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(fmt.Sprintf("ptrTime: bad timestamp %q: %v", s, err))
	}
	return &t
}

// TestRun covers Run's orchestration logic end to end against fakes for the
// Audible/Hardcover/Notifier dependencies and a real (in-memory) state
// store. Each case's setup (seeded prior state, ISBN lookup table, forced
// Hardcover error) and assertions (Result counters, persisted BookState,
// notifications sent, upserted Hardcover status) differ enough that they're
// captured via a per-case check closure rather than a fixed set of "want"
// fields — the table still eliminates the boilerplate of building
// Deps/running Run/checking the top-level error in every scenario.
func TestRun(t *testing.T) {
	tests := []struct {
		name               string
		items              []audible.LibraryItem
		byISBN             map[string]int64
		editionByISBN      map[string]int64 // ISBN → edition_id FindBookByISBN should return
		searchResults      map[string][]hardcover.SearchResult
		audioEditionByBook map[int64]int64 // bookID → edition_id FindDefaultEdition should return
		searchErr          error
		userBooks          map[int64]hardcover.UserBook // pre-seeded Hardcover library cache
		matchConfidence    float64
		upsertErr          error
		seed               *state.BookState
		check              func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result)
	}{
		{
			name: "finished book syncs as read",
			items: []audible.LibraryItem{
				{ASIN: "A1", ISBN13: "9780641723445", IsFinished: true, PercentComplete: 100},
			},
			byISBN: map[string]int64{"9780641723445": 777},
			check: func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result) {
				if result.Synced != 1 {
					t.Errorf("Synced = %d, want 1", result.Synced)
				}
				got := fh.inserted[777]
				if got.statusID != 3 {
					t.Errorf("statusID = %d, want 3 (Read)", got.statusID)
				}
				bs, err := store.Get("A1")
				if err != nil {
					t.Fatalf("store.Get() error = %v", err)
				}
				if bs == nil || bs.Status != state.StatusFinished {
					t.Errorf("stored state = %+v, want status finished", bs)
				}
			},
		},
		{
			name: "in-progress book syncs as currently reading",
			items: []audible.LibraryItem{
				{ASIN: "A2", ISBN13: "9780641723445", IsFinished: false, PercentComplete: 40},
			},
			byISBN: map[string]int64{"9780641723445": 777},
			check: func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result) {
				if result.Synced != 1 {
					t.Errorf("Synced = %d, want 1", result.Synced)
				}
				if fh.inserted[777].statusID != 2 {
					t.Errorf("statusID = %d, want 2 (Currently Reading)", fh.inserted[777].statusID)
				}
			},
		},
		{
			name: "not-started book is skipped",
			items: []audible.LibraryItem{
				{ASIN: "A3", ISBN13: "9780641723445", IsFinished: false, PercentComplete: 0},
			},
			check: func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result) {
				if result.Synced != 0 || len(fh.inserted) != 0 {
					t.Errorf("expected no sync for a not-started book, got Synced=%d inserted=%v", result.Synced, fh.inserted)
				}
			},
		},
		{
			name: "unchanged book skips hardcover call",
			items: []audible.LibraryItem{
				{ASIN: "A4", ISBN13: "9780641723445", IsFinished: true, PercentComplete: 100},
			},
			byISBN: map[string]int64{"9780641723445": 777},
			seed:   &state.BookState{ASIN: "A4", Status: state.StatusFinished, HardcoverBookID: int64Ptr(777)},
			check: func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result) {
				if result.Unchanged != 1 {
					t.Errorf("Unchanged = %d, want 1", result.Unchanged)
				}
				if len(fh.inserted) != 0 {
					t.Errorf("expected no Hardcover call for an unchanged book, got %v", fh.inserted)
				}
			},
		},
		{
			name: "no ISBN marks unmatched and notifies",
			items: []audible.LibraryItem{
				{ASIN: "A5", Title: "Some Title", Author: "Some Author", IsFinished: true, PercentComplete: 100}, // no ISBN13/ISBN10
			},
			check: func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result) {
				if result.Unmatched != 1 {
					t.Errorf("Unmatched = %d, want 1", result.Unmatched)
				}
				bs, err := store.Get("A5")
				if err != nil {
					t.Fatalf("store.Get() error = %v", err)
				}
				if bs == nil || !bs.Unmatched {
					t.Errorf("stored state = %+v, want Unmatched=true", bs)
				}
				if bs != nil && (bs.Title != "Some Title" || bs.Author != "Some Author") {
					t.Errorf("stored state Title/Author = %q/%q, want %q/%q (persisted for the manual-match web UI)", bs.Title, bs.Author, "Some Title", "Some Author")
				}
				if len(fn.notified) != 1 || fn.notified[0].ASIN != "A5" {
					t.Errorf("notified = %+v, want one entry for A5", fn.notified)
				}
				after, err := store.Get("A5")
				if err != nil {
					t.Fatalf("store.Get() error = %v", err)
				}
				if after == nil {
					t.Fatalf("store.Get(\"A5\") = nil, want persisted state")
				}
				if !after.NotifiedUnmatched {
					t.Errorf("NotifiedUnmatched = false after successful notify, want true (MarkNotified should have flipped it)")
				}
			},
		},
		{
			name: "already-notified unmatched book is not re-notified",
			items: []audible.LibraryItem{
				{ASIN: "A6", IsFinished: true, PercentComplete: 100},
			},
			seed: &state.BookState{ASIN: "A6", Status: state.StatusFinished, Unmatched: true, NotifiedUnmatched: true},
			check: func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result) {
				if len(fn.notified) != 0 {
					t.Errorf("notified = %+v, want no re-notification", fn.notified)
				}
			},
		},
		{
			name: "prior hardcover book id is reused without re-resolving by isbn",
			items: []audible.LibraryItem{
				{ASIN: "A8", ISBN13: "9780641723445", IsFinished: true, PercentComplete: 100},
			},
			// byISBN intentionally left empty: if resolveBookID incorrectly
			// re-resolved via FindBookByISBN instead of reusing the cached
			// HardcoverBookID, this item would fail to match.
			seed: &state.BookState{ASIN: "A8", Status: state.StatusInProgress, HardcoverBookID: int64Ptr(777)},
			check: func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result) {
				if result.Synced != 1 {
					t.Errorf("Synced = %d, want 1", result.Synced)
				}
				got, ok := fh.inserted[777]
				if !ok {
					t.Fatalf("inserted = %v, want an entry for book id 777", fh.inserted)
				}
				if got.statusID != 3 {
					t.Errorf("statusID = %d, want 3 (Read)", got.statusID)
				}
			},
		},
		{
			// Regression test for the Critical finding: webui.handleResolve
			// writes a state row with a cached HardcoverBookID but Status
			// left at its zero value ("") — see internal/webui/server.go's
			// handleResolve. This case seeds exactly that shape and proves
			// the next sync pass does NOT treat it as unchanged (since ""
			// can never equal a real state.Status constant) and instead
			// reuses the cached HardcoverBookID to make a real
			// InsertUserBook call, so a manually-resolved book actually
			// gets synced to Hardcover.
			name: "webui-resolved state (empty status) is synced, not skipped as unchanged",
			items: []audible.LibraryItem{
				{ASIN: "A9", ISBN13: "9780641723445", IsFinished: true, PercentComplete: 100},
			},
			// byISBN intentionally left empty: if resolveBookID incorrectly
			// re-resolved via FindBookByISBN instead of reusing the cached
			// HardcoverBookID, this item would fail to match and end up
			// unmatched instead of synced.
			seed: &state.BookState{ASIN: "A9", Status: "", HardcoverBookID: int64Ptr(777), Unmatched: false},
			check: func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result) {
				if result.Synced != 1 {
					t.Errorf("Synced = %d, want 1 (must not be skipped as unchanged)", result.Synced)
				}
				got, ok := fh.inserted[777]
				if !ok {
					t.Fatalf("inserted = %v, want an entry for cached book id 777", fh.inserted)
				}
				if got.statusID != 3 {
					t.Errorf("statusID = %d, want 3 (Read)", got.statusID)
				}
				bs, err := store.Get("A9")
				if err != nil {
					t.Fatalf("store.Get() error = %v", err)
				}
				if bs == nil || bs.Status != state.StatusFinished {
					t.Errorf("stored state = %+v, want status finished after sync", bs)
				}
			},
		},
		{
			name: "no isbn but high-confidence fuzzy match auto-syncs",
			items: []audible.LibraryItem{
				{ASIN: "A10", Title: "System Collapse (Dramatized Adaptation)", Author: "Martha Wells", IsFinished: true, PercentComplete: 100},
			},
			// no ISBN13/ISBN10 set — forces the fuzzy fallback path
			searchResults: map[string][]hardcover.SearchResult{
				"System Collapse (Dramatized Adaptation)": {{ID: 888, Title: "System Collapse", AuthorNames: []string{"Martha Wells"}}},
			},
			matchConfidence: 0.75, // medium
			check: func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result) {
				if result.Synced != 1 {
					t.Errorf("Synced = %d, want 1", result.Synced)
				}
				got, ok := fh.inserted[888]
				if !ok {
					t.Fatalf("inserted = %v, want an entry for book id 888", fh.inserted)
				}
				if got.statusID != 3 {
					t.Errorf("statusID = %d, want 3 (Read)", got.statusID)
				}
			},
		},
		{
			name: "no isbn and low-confidence fuzzy match falls through to unmatched",
			items: []audible.LibraryItem{
				{ASIN: "A11", Title: "Dune", Author: "Frank Herbert", IsFinished: true, PercentComplete: 100},
			},
			searchResults: map[string][]hardcover.SearchResult{
				"Dune": {{ID: 999, Title: "The Great Gatsby", AuthorNames: []string{"F. Scott Fitzgerald"}}},
			},
			matchConfidence: 0.95, // strict — this unrelated candidate must not clear it
			check: func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result) {
				if result.Unmatched != 1 {
					t.Errorf("Unmatched = %d, want 1", result.Unmatched)
				}
				if len(fh.inserted) != 0 {
					t.Errorf("expected no Hardcover upsert for a low-confidence match, got %v", fh.inserted)
				}
			},
		},
		{
			name: "book already read on hardcover is not downgraded to in-progress",
			items: []audible.LibraryItem{
				{ASIN: "A13", ISBN13: "9780641723445", IsFinished: false, PercentComplete: 40},
			},
			byISBN: map[string]int64{"9780641723445": 777},
			// Book 777 is already marked Read (3) on Hardcover — maybe
			// the user read it in print. Audible says in-progress (2).
			// The sync must NOT downgrade it to Currently Reading.
			userBooks: map[int64]hardcover.UserBook{
				777: {ID: 500, BookID: 777, StatusID: 3},
			},
			check: func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result) {
				if result.Unchanged != 1 {
					t.Errorf("Unchanged = %d, want 1 (already-read book must not be downgraded)", result.Unchanged)
				}
				if len(fh.inserted) != 0 {
					t.Errorf("expected no insert call for an existing book, got %v", fh.inserted)
				}
			},
		},
		{
			name: "book currently reading on hardcover is skipped, not upgraded",
			items: []audible.LibraryItem{
				{ASIN: "A14", ISBN13: "9780641723445", IsFinished: true, PercentComplete: 100},
			},
			byISBN: map[string]int64{"9780641723445": 777},
			// Book 777 is Currently Reading (2) on Hardcover. Audible
			// says finished → Read (3). The sync skips it entirely —
			// existing entries are never modified, even for upgrades.
			userBooks: map[int64]hardcover.UserBook{
				777: {ID: 500, BookID: 777, StatusID: 2},
			},
			check: func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result) {
				if result.Unchanged != 1 {
					t.Errorf("Unchanged = %d, want 1 (existing book must be skipped)", result.Unchanged)
				}
				if len(fh.inserted) != 0 {
					t.Errorf("expected no insert call for an existing book, got %v", fh.inserted)
				}
			},
		},
		{
			name: "book already read on hardcover stays read, no api call",
			items: []audible.LibraryItem{
				{ASIN: "A15", ISBN13: "9780641723445", IsFinished: true, PercentComplete: 100},
			},
			byISBN: map[string]int64{"9780641723445": 777},
			// Book 777 is already Read (3) on Hardcover. Audible also
			// says finished → Read (3). Status is equal — no API call
			// needed at all.
			userBooks: map[int64]hardcover.UserBook{
				777: {ID: 500, BookID: 777, StatusID: 3},
			},
			check: func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result) {
				if result.Unchanged != 1 {
					t.Errorf("Unchanged = %d, want 1 (already-read, no API call needed)", result.Unchanged)
				}
				if len(fh.inserted) != 0 {
					t.Errorf("expected no API calls for equal status, got %v", fh.inserted)
				}
			},
		},
		{
			name: "finished book passes finished date to hardcover insert",
			items: []audible.LibraryItem{
				{ASIN: "A16", ISBN13: "9780641723445", IsFinished: true, PercentComplete: 100, FinishedAt: ptrTime("2025-10-11T19:34:29.561Z")},
			},
			byISBN: map[string]int64{"9780641723445": 777},
			check: func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result) {
				if result.Synced != 1 {
					t.Fatalf("Synced = %d, want 1", result.Synced)
				}
				got, ok := fh.inserted[777]
				if !ok {
					t.Fatalf("inserted = %v, want an entry for book id 777", fh.inserted)
				}
				if got.userDate == nil {
					t.Error("userDate = nil, want the FinishedAt timestamp passed to the insert")
				}
			},
		},
		{
			name: "in-progress book does not pass a finished date",
			items: []audible.LibraryItem{
				{ASIN: "A17", ISBN13: "9780641723445", IsFinished: false, PercentComplete: 40, FinishedAt: nil},
			},
			byISBN: map[string]int64{"9780641723445": 777},
			check: func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result) {
				if result.Synced != 1 {
					t.Fatalf("Synced = %d, want 1", result.Synced)
				}
				got, ok := fh.inserted[777]
				if !ok {
					t.Fatalf("inserted = %v, want an entry for book id 777", fh.inserted)
				}
				if got.userDate != nil {
					t.Errorf("userDate = %v, want nil for an in-progress book", got.userDate)
				}
			},
		},
		{
			name: "isbn-matched book passes default audio edition to insert",
			items: []audible.LibraryItem{
				{ASIN: "A18", ISBN13: "9780641723445", IsFinished: true, PercentComplete: 100},
			},
			byISBN:        map[string]int64{"9780641723445": 777},
			editionByISBN: map[string]int64{"9780641723445": 999},
			check: func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result) {
				if result.Synced != 1 {
					t.Fatalf("Synced = %d, want 1", result.Synced)
				}
				got, ok := fh.inserted[777]
				if !ok {
					t.Fatalf("inserted = %v, want an entry for book id 777", fh.inserted)
				}
				if got.editionID == nil || *got.editionID != 999 {
					t.Errorf("editionID = %v, want 999 (FindBookByISBN's book.default_audio_edition_id)", got.editionID)
				}
			},
		},
		{
			name: "fuzzy-matched book passes audio edition from FindDefaultEdition to insert",
			items: []audible.LibraryItem{
				{ASIN: "A19", Title: "System Collapse", Author: "Martha Wells", IsFinished: true, PercentComplete: 100},
			},
			// no ISBN — forces the fuzzy fallback path, which resolves the
			// edition via a separate FindDefaultEdition call
			searchResults: map[string][]hardcover.SearchResult{
				"System Collapse": {{ID: 888, Title: "System Collapse", AuthorNames: []string{"Martha Wells"}}},
			},
			audioEditionByBook: map[int64]int64{888: 8889},
			matchConfidence:    0.75,
			check: func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result) {
				if result.Synced != 1 {
					t.Fatalf("Synced = %d, want 1", result.Synced)
				}
				got, ok := fh.inserted[888]
				if !ok {
					t.Fatalf("inserted = %v, want an entry for book id 888", fh.inserted)
				}
				if got.editionID == nil || *got.editionID != 8889 {
					t.Errorf("editionID = %v, want 8889 (from FindDefaultEdition)", got.editionID)
				}
			},
		},
		{
			name: "rated book passes rating to insert",
			items: []audible.LibraryItem{
				{ASIN: "A20", ISBN13: "9780641723445", IsFinished: true, PercentComplete: 100, Rating: intPtr(4)},
			},
			byISBN: map[string]int64{"9780641723445": 777},
			check: func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result) {
				if result.Synced != 1 {
					t.Fatalf("Synced = %d, want 1", result.Synced)
				}
				got, ok := fh.inserted[777]
				if !ok {
					t.Fatalf("inserted = %v, want an entry for book id 777", fh.inserted)
				}
				if got.rating == nil || *got.rating != 4 {
					t.Errorf("rating = %v, want 4", got.rating)
				}
			},
		},
		{
			name: "hardcover error counts as errored, not fatal",
			items: []audible.LibraryItem{
				{ASIN: "A7", ISBN13: "9780641723445", IsFinished: true, PercentComplete: 100},
			},
			byISBN:    map[string]int64{"9780641723445": 777},
			upsertErr: errors.New("hardcover down"),
			check: func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result) {
				if result.Errored != 1 {
					t.Errorf("Errored = %d, want 1", result.Errored)
				}
			},
		},
		{
			name: "fuzzy search api failure is a per-book error not a run abort",
			items: []audible.LibraryItem{
				{ASIN: "A12", Title: "Dune", Author: "Frank Herbert", IsFinished: true, PercentComplete: 100},
			},
			// no ISBN — forces the fuzzy fallback path, where the search
			// itself fails. This must be a per-book error (Errored++), not
			// a fatal run abort, matching how ISBN-search failures are
			// already treated.
			searchErr:       errors.New("search down"),
			matchConfidence: 0.75,
			check: func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result) {
				if result.Errored != 1 {
					t.Errorf("Errored = %d, want 1", result.Errored)
				}
				if len(fh.inserted) != 0 {
					t.Errorf("expected no upsert on search failure, got %v", fh.inserted)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fa := &fakeAudible{items: tt.items}
			fh := newFakeHardcover()
			for isbn, id := range tt.byISBN {
				fh.byISBN[isbn] = id
			}
			for isbn, ed := range tt.editionByISBN {
				fh.editionByISBN[isbn] = ed
			}
			for query, results := range tt.searchResults {
				fh.searchResults[query] = results
			}
			for bookID, ed := range tt.audioEditionByBook {
				fh.audioEditionByBook[bookID] = ed
			}
			fh.upsertErr = tt.upsertErr
			fh.searchErr = tt.searchErr
			for bookID, ub := range tt.userBooks {
				fh.userBooks[bookID] = ub
			}
			store := newTestStore(t)
			if tt.seed != nil {
				if err := store.Upsert(*tt.seed); err != nil {
					t.Fatalf("seed store error = %v", err)
				}
			}
			fn := &fakeNotifier{}

			result, err := Run(context.Background(), Deps{Audible: fa, Hardcover: fh, Store: store, Notifier: fn, MatchConfidence: tt.matchConfidence})
			if err != nil {
				t.Fatalf("Run() error = %v, want nil (per-book errors must not fail the run)", err)
			}

			tt.check(t, store, fh, fn, result)
		})
	}
}

// TestRun_FetchLibraryError covers the one case where Run returns a non-nil
// top-level error: a failure to fetch the Audible library at all. Unlike
// per-book failures (Hardcover errors, store errors), this must short-circuit
// the whole pass with nothing synced and no notification sent.
func TestRun_FetchLibraryError(t *testing.T) {
	wantErr := errors.New("audible unavailable")
	fa := &fakeAudible{fetchErr: wantErr}
	fh := newFakeHardcover()
	store := newTestStore(t)
	fn := &fakeNotifier{}

	result, err := Run(context.Background(), Deps{Audible: fa, Hardcover: fh, Store: store, Notifier: fn})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want it to wrap %v", err, wantErr)
	}
	if result != (Result{}) {
		t.Errorf("Run() result = %+v, want zero value on fetch failure", result)
	}
	if len(fn.notified) != 0 {
		t.Errorf("notified = %v, want no notifications sent on fetch failure", fn.notified)
	}
}

func TestRun_StopsWhenDailyQuotaExhausted(t *testing.T) {
	fa := &fakeAudible{items: []audible.LibraryItem{
		{ASIN: "A1", ISBN13: "9780641723445", IsFinished: true, PercentComplete: 100},
		{ASIN: "A2", ISBN13: "9780641723452", IsFinished: true, PercentComplete: 100},
	}}
	fh := newFakeHardcover()
	fh.byISBN["9780641723445"] = 777
	fh.byISBN["9780641723452"] = 888
	fh.upsertErr = fmt.Errorf("insert user_book: %w", hardcover.ErrDailyQuotaExhausted)
	store := newTestStore(t)

	result, err := Run(context.Background(), Deps{Audible: fa, Hardcover: fh, Store: store, Notifier: &fakeNotifier{}})
	if !errors.Is(err, hardcover.ErrDailyQuotaExhausted) {
		t.Fatalf("Run() error = %v, want ErrDailyQuotaExhausted", err)
	}
	if result.Errored != 1 {
		t.Errorf("Errored = %d, want 1 (run stops after the first quota failure)", result.Errored)
	}
}
