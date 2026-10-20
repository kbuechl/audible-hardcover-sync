// Package webui provides a JSON API for a human to resolve books the
// automated sync could not match deterministically (via
// hardcover.Client.FindBookByISBN) to a Hardcover book_id. It is the
// human-supervised fuzzy match path: a person reviews the
// hardcover.Client.SearchBooks results and picks the match, in contrast to
// internal/sync's resolveByFuzzyMatch, which applies a confidence-scored
// fuzzy title/author match automatically. The actual UI is a React SPA
// (see frontend/) that consumes this API; the built SPA (frontend/dist) is
// embedded and served for any request that doesn't match /api/*, with a
// client-side-routing fallback to index.html (see static.go).
package webui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kbuechl/audible-hardcover-sync/internal/hardcover"
	"github.com/kbuechl/audible-hardcover-sync/internal/state"
)

// HardcoverSearcher is the subset of *hardcover.Client this package needs,
// so tests can substitute a fake instead of hitting the real API.
type HardcoverSearcher interface {
	SearchBooks(ctx context.Context, query string, perPage int) ([]hardcover.SearchResult, error)
	FetchCoverURLs(ctx context.Context, bookIDs []int64) (map[int64]string, error)
}

type Server struct {
	store *state.Store
	hc    HardcoverSearcher

	// Set by WithService; nil when only the match endpoints are served.
	svc     Service
	syncCtx context.Context
	testURL func(string) error
}

func New(store *state.Store, hc HardcoverSearcher) *Server {
	return &Server{store: store, hc: hc}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/unmatched", s.handleAPIUnmatchedList)
	mux.HandleFunc("GET /api/unmatched/{asin}", s.handleAPIUnmatchedDetail)
	mux.HandleFunc("GET /api/search", s.handleAPISearch)
	mux.HandleFunc("POST /api/resolve", s.handleAPIResolve)
	if s.svc != nil {
		s.registerServiceRoutes(mux)
	}
	mux.Handle("/", spaHandler())
	return mux
}

type unmatchedBookDTO struct {
	ASIN            string  `json:"asin"`
	Title           string  `json:"title"`
	Author          string  `json:"author"`
	Status          string  `json:"status"`
	PercentComplete float64 `json:"percentComplete"`
	CoverURL        string  `json:"coverUrl,omitempty"`
}

func toUnmatchedBookDTO(bs state.BookState) unmatchedBookDTO {
	return unmatchedBookDTO{
		ASIN:            bs.ASIN,
		Title:           bs.Title,
		Author:          bs.Author,
		Status:          string(bs.Status),
		PercentComplete: bs.PercentComplete,
		CoverURL:        bs.CoverURL,
	}
}

func (s *Server) handleAPIUnmatchedList(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListUnmatched()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	dtos := make([]unmatchedBookDTO, 0, len(items))
	for _, bs := range items {
		dtos = append(dtos, toUnmatchedBookDTO(bs))
	}
	writeJSON(w, http.StatusOK, dtos)
}

func (s *Server) handleAPIUnmatchedDetail(w http.ResponseWriter, r *http.Request) {
	asin := r.PathValue("asin")
	bs, err := s.store.Get(asin)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if bs == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, toUnmatchedBookDTO(*bs))
}

type searchResultDTO struct {
	ID          int64    `json:"id"`
	Title       string   `json:"title"`
	AuthorNames []string `json:"authorNames"`
	ReleaseYear int      `json:"releaseYear,omitempty"`
	Slug        string   `json:"slug"`
	CoverURL    string   `json:"coverUrl,omitempty"`
}

func (s *Server) handleAPISearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeJSON(w, http.StatusOK, []searchResultDTO{})
		return
	}

	searcher := s.hc
	if s.svc != nil {
		hc := s.svc.Hardcover()
		if hc == nil {
			writeError(w, http.StatusServiceUnavailable, errors.New("Hardcover isn't connected: add your API token in Settings"))
			return
		}
		searcher = hc
	}

	results, err := searcher.SearchBooks(r.Context(), query, 10)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	ids := make([]int64, len(results))
	for i, r := range results {
		ids[i] = r.ID
	}
	// A cover-lookup failure is not fatal to the search response — the
	// candidate matches themselves are still useful without cover art, and
	// the frontend renders a placeholder tile for any book with no
	// (or unresolved) cover URL.
	covers, err := searcher.FetchCoverURLs(r.Context(), ids)
	if err != nil {
		covers = map[int64]string{}
	}

	dtos := make([]searchResultDTO, len(results))
	for i, res := range results {
		dtos[i] = searchResultDTO{
			ID:          res.ID,
			Title:       res.Title,
			AuthorNames: res.AuthorNames,
			ReleaseYear: res.ReleaseYear,
			Slug:        res.Slug,
			CoverURL:    covers[res.ID],
		}
	}
	writeJSON(w, http.StatusOK, dtos)
}

type resolveRequest struct {
	ASIN            string `json:"asin"`
	HardcoverBookID *int64 `json:"hardcoverBookId"`
}

func (s *Server) handleAPIResolve(w http.ResponseWriter, r *http.Request) {
	var req resolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.ASIN == "" || req.HardcoverBookID == nil {
		http.Error(w, "asin and hardcoverBookId are required", http.StatusBadRequest)
		return
	}

	// Deliberately do NOT carry forward existing Status/PercentComplete
	// here. Status is left at its zero value ("") so that sync.Run's
	// unchanged-skip check (prior.Status == newly-computed status) can
	// never match a real state.Status constant, which forces the next sync
	// pass to treat this book as changed and actually call
	// Hardcover.InsertUserBook using the HardcoverBookID cached here.
	// Preserving the prior Status used to make a freshly-resolved book look
	// "unchanged" on the very next run, so it silently never got synced to
	// Hardcover — see the final whole-branch review finding this fixed.
	bs := state.BookState{
		ASIN:              req.ASIN,
		HardcoverBookID:   req.HardcoverBookID,
		Unmatched:         false,
		NotifiedUnmatched: false,
	}
	if err := s.store.Upsert(bs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
