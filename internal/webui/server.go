// Package webui provides HTTP handlers and html/template templates for a
// human to resolve books the automated sync could not match deterministically
// (via hardcover.Client.FindBookByISBN) to a Hardcover book_id. It is the
// only part of this codebase that performs fuzzy title/author matching,
// via hardcover.Client.SearchBooks, and it is human-supervised: a person
// reviews the search results and picks the match, rather than the sync
// path applying a fuzzy match automatically.
package webui

import (
	"context"
	"embed"
	"html/template"
	"net/http"
	"strconv"

	"github.com/kbuechl/audible-hardcover-sync/internal/hardcover"
	"github.com/kbuechl/audible-hardcover-sync/internal/state"
)

//go:embed templates/*.html
var templateFS embed.FS

var templates = template.Must(template.ParseFS(templateFS, "templates/*.html"))

// HardcoverSearcher is the subset of *hardcover.Client this package needs,
// so tests can substitute a fake instead of hitting the real API.
type HardcoverSearcher interface {
	SearchBooks(ctx context.Context, query string, perPage int) ([]hardcover.SearchResult, error)
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
	mux.HandleFunc("GET /unmatched", s.handleList)
	mux.HandleFunc("GET /unmatched/{asin}", s.handleDetail)
	mux.HandleFunc("POST /unmatched/{asin}/resolve", s.handleResolve)
	return mux
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListUnmatched()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := templates.ExecuteTemplate(w, "unmatched_list.html", map[string]any{"Items": items}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleDetail(w http.ResponseWriter, r *http.Request) {
	asin := r.PathValue("asin")
	query := r.URL.Query().Get("q")

	var results []hardcover.SearchResult
	if query != "" {
		found, err := s.hc.SearchBooks(r.Context(), query, 10)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		results = found
	}

	if err := templates.ExecuteTemplate(w, "unmatched_detail.html", map[string]any{
		"ASIN":    asin,
		"Query":   query,
		"Results": results,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleResolve(w http.ResponseWriter, r *http.Request) {
	asin := r.PathValue("asin")
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	bookID, err := strconv.ParseInt(r.PostFormValue("hardcover_book_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid hardcover_book_id", http.StatusBadRequest)
		return
	}

	existing, err := s.store.Get(asin)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	bs := state.BookState{ASIN: asin, HardcoverBookID: &bookID, Unmatched: false, NotifiedUnmatched: false}
	if existing != nil {
		bs.Status = existing.Status
		bs.PercentComplete = existing.PercentComplete
	}
	if err := s.store.Upsert(bs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/unmatched", http.StatusSeeOther)
}
