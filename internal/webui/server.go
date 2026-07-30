// Package webui provides a JSON API for a human to resolve books the
// automated sync could not match deterministically (via
// hardcover.Client.FindBookByISBN) to a Hardcover book_id. It is the only
// part of this codebase that performs fuzzy title/author matching, via
// hardcover.Client.SearchBooks, and it is human-supervised: a person
// reviews the search results and picks the match, rather than the sync
// path applying a fuzzy match automatically. The actual UI is a React SPA
// (see frontend/) that consumes this API; static-file serving of the
// built SPA is added in a later task once frontend/dist exists to embed.
package webui

import (
	"context"
	"encoding/json"
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
	return mux
}

type unmatchedBookDTO struct {
	ASIN            string  `json:"asin"`
	Title           string  `json:"title"`
	Author          string  `json:"author"`
	Status          string  `json:"status"`
	PercentComplete float64 `json:"percentComplete"`
}

func toUnmatchedBookDTO(bs state.BookState) unmatchedBookDTO {
	return unmatchedBookDTO{
		ASIN:            bs.ASIN,
		Title:           bs.Title,
		Author:          bs.Author,
		Status:          string(bs.Status),
		PercentComplete: bs.PercentComplete,
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

	results, err := s.hc.SearchBooks(r.Context(), query, 10)
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
	covers, err := s.hc.FetchCoverURLs(r.Context(), ids)
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
	// Hardcover.UpsertUserBook using the HardcoverBookID cached here.
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
