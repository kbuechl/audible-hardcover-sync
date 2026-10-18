package webui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kbuechl/audible-hardcover-sync/internal/hardcover"
	"github.com/kbuechl/audible-hardcover-sync/internal/state"
)

type fakeSearcher struct {
	results   []hardcover.SearchResult
	covers    map[int64]string
	searchErr error
	coversErr error
}

func (f *fakeSearcher) SearchBooks(ctx context.Context, query string, perPage int) ([]hardcover.SearchResult, error) {
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.results, nil
}

func (f *fakeSearcher) FetchCoverURLs(ctx context.Context, bookIDs []int64) (map[int64]string, error) {
	if f.coversErr != nil {
		return nil, f.coversErr
	}
	if f.covers == nil {
		return map[int64]string{}, nil
	}
	return f.covers, nil
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

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode response body %q: %v", rec.Body.String(), err)
	}
	return v
}

func TestAPIUnmatchedList(t *testing.T) {
	tests := []struct {
		name  string
		seed  []state.BookState
		check func(t *testing.T, got []unmatchedBookDTO)
	}{
		{
			name: "returns unmatched books",
			seed: []state.BookState{
				{ASIN: "A1", Status: state.StatusFinished, Title: "Book One", Author: "Author One", Unmatched: true, PercentComplete: 100},
				{ASIN: "A2", Status: state.StatusFinished, Unmatched: false},
			},
			check: func(t *testing.T, got []unmatchedBookDTO) {
				if len(got) != 1 {
					t.Fatalf("got %d items, want 1 (only unmatched)", len(got))
				}
				if got[0].ASIN != "A1" || got[0].Title != "Book One" || got[0].Author != "Author One" {
					t.Errorf("got %+v", got[0])
				}
				if got[0].PercentComplete != 100 {
					t.Errorf("PercentComplete = %v, want 100", got[0].PercentComplete)
				}
			},
		},
		{
			name: "empty when nothing unmatched",
			seed: nil,
			check: func(t *testing.T, got []unmatchedBookDTO) {
				if len(got) != 0 {
					t.Errorf("got %v, want empty", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestStore(t)
			for _, bs := range tt.seed {
				if err := store.Upsert(bs); err != nil {
					t.Fatalf("seed Upsert() error = %v", err)
				}
			}
			srv := New(store, &fakeSearcher{})
			req := httptest.NewRequest(http.MethodGet, "/api/unmatched", nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
			}
			tt.check(t, decodeJSON[[]unmatchedBookDTO](t, rec))
		})
	}
}

func TestAPIUnmatchedDetail(t *testing.T) {
	store := newTestStore(t)
	if err := store.Upsert(state.BookState{ASIN: "A1", Title: "Book One", Author: "Author One", Unmatched: true}); err != nil {
		t.Fatalf("seed error = %v", err)
	}
	srv := New(store, &fakeSearcher{})

	t.Run("known asin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/unmatched/A1", nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		got := decodeJSON[unmatchedBookDTO](t, rec)
		if got.Title != "Book One" {
			t.Errorf("Title = %q, want %q", got.Title, "Book One")
		}
	})

	t.Run("unknown asin returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/unmatched/DOES-NOT-EXIST", nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
}

func TestAPISearch(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		searcher  *fakeSearcher
		wantCode  int
		checkBody func(t *testing.T, got []searchResultDTO)
	}{
		{
			name:  "merges cover urls by id",
			query: "thorns",
			searcher: &fakeSearcher{
				results: []hardcover.SearchResult{
					{ID: 111, Title: "A Court of Thorns and Roses", AuthorNames: []string{"Sarah J. Maas"}, ReleaseYear: 2015, Slug: "acotar"},
					{ID: 222, Title: "No Cover Book", AuthorNames: []string{"Someone"}},
				},
				covers: map[int64]string{111: "https://assets.hardcover.app/111.jpg"},
			},
			wantCode: http.StatusOK,
			checkBody: func(t *testing.T, got []searchResultDTO) {
				if len(got) != 2 {
					t.Fatalf("got %d results, want 2", len(got))
				}
				if got[0].CoverURL != "https://assets.hardcover.app/111.jpg" {
					t.Errorf("CoverURL = %q, want the cover for id 111", got[0].CoverURL)
				}
				if got[1].CoverURL != "" {
					t.Errorf("CoverURL = %q, want empty for a book with no cover", got[1].CoverURL)
				}
				if got[0].Slug != "acotar" {
					t.Errorf("Slug = %q, want %q", got[0].Slug, "acotar")
				}
			},
		},
		{
			name:     "empty query returns empty results without calling the searcher",
			query:    "",
			searcher: &fakeSearcher{searchErr: fmt.Errorf("should never be called")},
			wantCode: http.StatusOK,
			checkBody: func(t *testing.T, got []searchResultDTO) {
				if len(got) != 0 {
					t.Errorf("got %v, want empty", got)
				}
			},
		},
		{
			name:     "search error becomes 502",
			query:    "thorns",
			searcher: &fakeSearcher{searchErr: fmt.Errorf("hardcover down")},
			wantCode: http.StatusBadGateway,
		},
		{
			name:  "cover lookup error still returns search results without covers",
			query: "thorns",
			searcher: &fakeSearcher{
				results:   []hardcover.SearchResult{{ID: 111, Title: "A Book"}},
				coversErr: fmt.Errorf("cover lookup down"),
			},
			wantCode: http.StatusOK,
			checkBody: func(t *testing.T, got []searchResultDTO) {
				if len(got) != 1 || got[0].CoverURL != "" {
					t.Errorf("got %+v, want 1 result with no cover url (cover errors are non-fatal)", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestStore(t)
			srv := New(store, tt.searcher)
			req := httptest.NewRequest(http.MethodGet, "/api/search?asin=A1&q="+tt.query, nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.wantCode, rec.Body.String())
			}
			if tt.checkBody != nil {
				tt.checkBody(t, decodeJSON[[]searchResultDTO](t, rec))
			}
		})
	}
}

func TestHandler_ServesStaticFrontend(t *testing.T) {
	store := newTestStore(t)
	srv := New(store, &fakeSearcher{})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("<!doctype html")) && !bytes.Contains(rec.Body.Bytes(), []byte("<!DOCTYPE html")) {
		t.Errorf("body doesn't look like the built SPA's index.html: %s", rec.Body.String())
	}
}

func TestHandler_SPAFallbackForClientSideRoutes(t *testing.T) {
	store := newTestStore(t)
	srv := New(store, &fakeSearcher{})

	// A path with no matching static asset and no /api prefix should still
	// serve index.html, so client-side routing (if any is added later) works
	// on a hard refresh instead of 404ing.
	req := httptest.NewRequest(http.MethodGet, "/some/deep/client/route", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (SPA fallback)", rec.Code)
	}
}

func TestHandler_ServesRealStaticAssets(t *testing.T) {
	store := newTestStore(t)
	srv := New(store, &fakeSearcher{})

	// A request for a real static asset should serve the asset directly,
	// not the SPA fallback. This covers the branch in spaHandler that
	// calls fileServer.ServeHTTP for existing files.
	req := httptest.NewRequest(http.MethodGet, "/favicon.svg", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// Verify we're NOT getting the index.html fallback by ensuring
	// the response doesn't contain the HTML doctype that would indicate
	// we fell back to the SPA's root index.html.
	if bytes.Contains(rec.Body.Bytes(), []byte("<!doctype html")) || bytes.Contains(rec.Body.Bytes(), []byte("<!DOCTYPE html")) {
		t.Errorf("body contains HTML doctype; should be serving favicon.svg, not index.html fallback")
	}
}

func TestAPIResolve(t *testing.T) {
	tests := []struct {
		name     string
		seed     *state.BookState
		body     string
		wantCode int
	}{
		{
			name:     "valid resolve clears status and unmatched",
			seed:     &state.BookState{ASIN: "A1", Status: state.StatusFinished, Unmatched: true, NotifiedUnmatched: true},
			body:     `{"asin":"A1","hardcoverBookId":777}`,
			wantCode: http.StatusOK,
		},
		{
			name:     "missing hardcoverBookId is a bad request",
			seed:     &state.BookState{ASIN: "A1", Unmatched: true},
			body:     `{"asin":"A1"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "malformed json is a bad request",
			seed:     &state.BookState{ASIN: "A1", Unmatched: true},
			body:     `not json`,
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestStore(t)
			if tt.seed != nil {
				if err := store.Upsert(*tt.seed); err != nil {
					t.Fatalf("seed error = %v", err)
				}
			}
			srv := New(store, &fakeSearcher{})
			req := httptest.NewRequest(http.MethodPost, "/api/resolve", bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.wantCode, rec.Body.String())
			}
			if tt.wantCode != http.StatusOK {
				return
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
				t.Error("NotifiedUnmatched = true, want false after resolve")
			}
			// Deliberately cleared, not preserved — see the Global Constraints
			// note on why a resolved book's Status must not survive resolve.
			if bs.Status != "" {
				t.Errorf("Status = %q, want empty (cleared so the next sync detects a change)", bs.Status)
			}
			if bs.HardcoverBookID == nil || *bs.HardcoverBookID != 777 {
				t.Errorf("HardcoverBookID = %v, want 777", bs.HardcoverBookID)
			}
		})
	}
}

func TestAPIUnmatched_IncludesCoverURL(t *testing.T) {
	store := newTestStore(t)
	if err := store.Upsert(state.BookState{ASIN: "A1", Status: state.StatusFinished, Unmatched: true, CoverURL: "https://m.media-amazon.com/500.jpg"}); err != nil {
		t.Fatalf("seed error = %v", err)
	}
	srv := New(store, &fakeSearcher{})

	for _, path := range []string{"/api/unmatched", "/api/unmatched/A1"} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if !bytes.Contains(rec.Body.Bytes(), []byte(`"coverUrl":"https://m.media-amazon.com/500.jpg"`)) {
			t.Errorf("%s body = %s, want coverUrl", path, rec.Body.String())
		}
	}
}
