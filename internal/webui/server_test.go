package webui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kbuechl/audible-hardcover-sync/internal/hardcover"
	"github.com/kbuechl/audible-hardcover-sync/internal/state"
)

// fakeSearcher implements HardcoverSearcher for tests. It also records
// every query it was called with, so tests can assert the searcher was (or
// deliberately was not) invoked — e.g. an empty search box should not fire
// a request to Hardcover.
type fakeSearcher struct {
	results []hardcover.SearchResult
	err     error
	calls   []string
}

func (f *fakeSearcher) SearchBooks(ctx context.Context, query string, perPage int) ([]hardcover.SearchResult, error) {
	f.calls = append(f.calls, query)
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
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

func TestUnmatchedList_ShowsUnmatchedBooks(t *testing.T) {
	store := newTestStore(t)
	if err := store.Upsert(state.BookState{ASIN: "A1", Status: state.StatusFinished, Unmatched: true}); err != nil {
		t.Fatalf("seed error = %v", err)
	}

	srv := New(store, &fakeSearcher{})
	req := httptest.NewRequest(http.MethodGet, "/unmatched", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "A1") {
		t.Errorf("body does not mention unmatched ASIN A1: %s", rec.Body.String())
	}
}

// TestUnmatchedList_EmptyStateMessage verifies the "nothing to resolve"
// branch of the template renders when there are no unmatched books, rather
// than an empty (and possibly confusing) list.
func TestUnmatchedList_EmptyStateMessage(t *testing.T) {
	store := newTestStore(t)

	srv := New(store, &fakeSearcher{})
	req := httptest.NewRequest(http.MethodGet, "/unmatched", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Nothing to resolve") {
		t.Errorf("body does not show empty-state message: %s", rec.Body.String())
	}
}

// TestUnmatchedList_StoreError verifies a genuine store failure (simulated
// by closing the underlying DB out from under the handler) surfaces as a
// 500 rather than a panic or a silently empty list.
func TestUnmatchedList_StoreError(t *testing.T) {
	store := newTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}

	srv := New(store, &fakeSearcher{})
	req := httptest.NewRequest(http.MethodGet, "/unmatched", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for a closed/broken store", rec.Code)
	}
}

func TestUnmatchedDetail_ShowsSearchResults(t *testing.T) {
	store := newTestStore(t)
	if err := store.Upsert(state.BookState{ASIN: "A1", Status: state.StatusFinished, Unmatched: true}); err != nil {
		t.Fatalf("seed error = %v", err)
	}
	searcher := &fakeSearcher{results: []hardcover.SearchResult{{ID: 777, Title: "Some Book", AuthorNames: []string{"Some Author"}}}}

	srv := New(store, searcher)
	req := httptest.NewRequest(http.MethodGet, "/unmatched/A1?q=some+book", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Some Book") {
		t.Errorf("body does not show search result: %s", rec.Body.String())
	}
	if len(searcher.calls) != 1 || searcher.calls[0] != "some book" {
		t.Errorf("searcher.calls = %v, want [\"some book\"]", searcher.calls)
	}
}

// TestUnmatchedDetail_EmptyQueryDoesNotCallSearcher verifies that visiting
// the detail page without a query string (the normal first load, before the
// user has typed anything) does not fire a request to Hardcover at all.
func TestUnmatchedDetail_EmptyQueryDoesNotCallSearcher(t *testing.T) {
	store := newTestStore(t)
	if err := store.Upsert(state.BookState{ASIN: "A1", Status: state.StatusFinished, Unmatched: true}); err != nil {
		t.Fatalf("seed error = %v", err)
	}
	searcher := &fakeSearcher{results: []hardcover.SearchResult{{ID: 777, Title: "Should Not Appear"}}}

	srv := New(store, searcher)
	req := httptest.NewRequest(http.MethodGet, "/unmatched/A1", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(searcher.calls) != 0 {
		t.Errorf("searcher.calls = %v, want no calls for empty query", searcher.calls)
	}
	if strings.Contains(rec.Body.String(), "Should Not Appear") {
		t.Errorf("body unexpectedly shows search results for empty query: %s", rec.Body.String())
	}
}

// TestUnmatchedDetail_SearchError verifies a Hardcover search failure
// surfaces as a 502 (bad gateway) rather than a silent empty result list or
// a 500 that implies a bug in this service rather than the upstream.
func TestUnmatchedDetail_SearchError(t *testing.T) {
	store := newTestStore(t)
	if err := store.Upsert(state.BookState{ASIN: "A1", Status: state.StatusFinished, Unmatched: true}); err != nil {
		t.Fatalf("seed error = %v", err)
	}
	searcher := &fakeSearcher{err: context.DeadlineExceeded}

	srv := New(store, searcher)
	req := httptest.NewRequest(http.MethodGet, "/unmatched/A1?q=some+book", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}

// TestUnmatchedDetail_UnknownASIN verifies the detail page still renders
// for an ASIN with no row in the store (e.g. a stale bookmarked link) —
// the handler doesn't look the ASIN up before rendering the search form, so
// this should degrade gracefully rather than error.
func TestUnmatchedDetail_UnknownASIN(t *testing.T) {
	store := newTestStore(t)

	srv := New(store, &fakeSearcher{})
	req := httptest.NewRequest(http.MethodGet, "/unmatched/NOPE", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "NOPE") {
		t.Errorf("body does not echo the requested ASIN: %s", rec.Body.String())
	}
}

func TestResolve_WritesHardcoverBookIDAndClearsUnmatched(t *testing.T) {
	store := newTestStore(t)
	if err := store.Upsert(state.BookState{ASIN: "A1", Status: state.StatusFinished, Unmatched: true, NotifiedUnmatched: true}); err != nil {
		t.Fatalf("seed error = %v", err)
	}

	srv := New(store, &fakeSearcher{})
	form := url.Values{"hardcover_book_id": {"777"}}
	req := httptest.NewRequest(http.MethodPost, "/unmatched/A1/resolve", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 redirect", rec.Code)
	}

	bs, err := store.Get("A1")
	if err != nil {
		t.Fatalf("store.Get() error = %v", err)
	}
	if bs == nil {
		t.Fatal("expected state row to still exist")
	}
	if bs.Unmatched {
		t.Error("Unmatched = true, want false after resolve")
	}
	if bs.NotifiedUnmatched {
		t.Error("NotifiedUnmatched = true, want false after resolve (so it can re-notify on a future unmatch)")
	}
	if bs.HardcoverBookID == nil || *bs.HardcoverBookID != 777 {
		t.Errorf("HardcoverBookID = %v, want 777", bs.HardcoverBookID)
	}
	if bs.Status != state.StatusFinished {
		t.Errorf("Status = %q, want %q (existing status preserved)", bs.Status, state.StatusFinished)
	}
}

// TestResolve_InvalidBookID is table-driven over the ways a submitted form
// can fail to carry a usable hardcover_book_id: missing entirely, empty,
// and non-numeric. All three must be rejected with 400 rather than being
// silently coerced to 0 and written to the store.
func TestResolve_InvalidBookID(t *testing.T) {
	tests := []struct {
		name string
		form url.Values
	}{
		{name: "missing field", form: url.Values{}},
		{name: "empty value", form: url.Values{"hardcover_book_id": {""}}},
		{name: "non-numeric value", form: url.Values{"hardcover_book_id": {"not-a-number"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestStore(t)
			if err := store.Upsert(state.BookState{ASIN: "A1", Status: state.StatusFinished, Unmatched: true}); err != nil {
				t.Fatalf("seed error = %v", err)
			}

			srv := New(store, &fakeSearcher{})
			req := httptest.NewRequest(http.MethodPost, "/unmatched/A1/resolve", strings.NewReader(tt.form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}

			bs, err := store.Get("A1")
			if err != nil {
				t.Fatalf("store.Get() error = %v", err)
			}
			if bs == nil {
				t.Fatal("expected state row to still exist")
			}
			if !bs.Unmatched {
				t.Error("Unmatched = false, want true — invalid submission must not clear it")
			}
			if bs.HardcoverBookID != nil {
				t.Errorf("HardcoverBookID = %v, want nil — invalid submission must not write it", bs.HardcoverBookID)
			}
		})
	}
}

// TestResolve_UnknownASIN verifies resolving an ASIN with no existing state
// row still succeeds (a new row is created) rather than erroring, matching
// store.Get's documented "not found" contract of (nil, nil).
func TestResolve_UnknownASIN(t *testing.T) {
	store := newTestStore(t)

	srv := New(store, &fakeSearcher{})
	form := url.Values{"hardcover_book_id": {"42"}}
	req := httptest.NewRequest(http.MethodPost, "/unmatched/NEWASIN/resolve", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 redirect", rec.Code)
	}

	bs, err := store.Get("NEWASIN")
	if err != nil {
		t.Fatalf("store.Get() error = %v", err)
	}
	if bs == nil {
		t.Fatal("expected a new state row to be created")
	}
	if bs.HardcoverBookID == nil || *bs.HardcoverBookID != 42 {
		t.Errorf("HardcoverBookID = %v, want 42", bs.HardcoverBookID)
	}
}

// TestResolve_StoreError verifies a store failure during resolve (simulated
// by closing the underlying DB) surfaces as a 500, and — since the request
// never reaches the redirect — the caller has no reason to believe the
// resolution was recorded.
func TestResolve_StoreError(t *testing.T) {
	store := newTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}

	srv := New(store, &fakeSearcher{})
	form := url.Values{"hardcover_book_id": {"777"}}
	req := httptest.NewRequest(http.MethodPost, "/unmatched/A1/resolve", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for a closed/broken store", rec.Code)
	}
}
