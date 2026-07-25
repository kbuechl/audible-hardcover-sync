// internal/sync/sync_test.go
package sync

import (
	"context"
	"errors"
	"testing"

	"github.com/kbuechl/audible-hardcover-sync/internal/audible"
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
	statusID int
	rating   *float64
}

type fakeHardcover struct {
	byISBN    map[string]int64
	upserted  map[int64]upsertCall
	upsertErr error
}

func newFakeHardcover() *fakeHardcover {
	return &fakeHardcover{
		byISBN:   map[string]int64{},
		upserted: map[int64]upsertCall{},
	}
}

func (f *fakeHardcover) FindBookByISBN(ctx context.Context, isbn string) (int64, bool, error) {
	id, ok := f.byISBN[isbn]
	return id, ok, nil
}

func (f *fakeHardcover) UpsertUserBook(ctx context.Context, bookID int64, statusID int, rating *float64) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.upserted[bookID] = upsertCall{statusID, rating}
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
		name      string
		items     []audible.LibraryItem
		byISBN    map[string]int64
		upsertErr error
		seed      *state.BookState
		check     func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result)
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
				got := fh.upserted[777]
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
				if fh.upserted[777].statusID != 2 {
					t.Errorf("statusID = %d, want 2 (Currently Reading)", fh.upserted[777].statusID)
				}
			},
		},
		{
			name: "not-started book is skipped",
			items: []audible.LibraryItem{
				{ASIN: "A3", ISBN13: "9780641723445", IsFinished: false, PercentComplete: 0},
			},
			check: func(t *testing.T, store *state.Store, fh *fakeHardcover, fn *fakeNotifier, result Result) {
				if result.Synced != 0 || len(fh.upserted) != 0 {
					t.Errorf("expected no sync for a not-started book, got Synced=%d upserted=%v", result.Synced, fh.upserted)
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
				if len(fh.upserted) != 0 {
					t.Errorf("expected no Hardcover call for an unchanged book, got %v", fh.upserted)
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
				got, ok := fh.upserted[777]
				if !ok {
					t.Fatalf("upserted = %v, want an entry for book id 777", fh.upserted)
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
			// UpsertUserBook call, so a manually-resolved book actually
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
				got, ok := fh.upserted[777]
				if !ok {
					t.Fatalf("upserted = %v, want an entry for cached book id 777", fh.upserted)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fa := &fakeAudible{items: tt.items}
			fh := newFakeHardcover()
			for isbn, id := range tt.byISBN {
				fh.byISBN[isbn] = id
			}
			fh.upsertErr = tt.upsertErr
			store := newTestStore(t)
			if tt.seed != nil {
				if err := store.Upsert(*tt.seed); err != nil {
					t.Fatalf("seed store error = %v", err)
				}
			}
			fn := &fakeNotifier{}

			result, err := Run(context.Background(), Deps{Audible: fa, Hardcover: fh, Store: store, Notifier: fn})
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
