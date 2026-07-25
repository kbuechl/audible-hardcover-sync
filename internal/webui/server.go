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
	hasQueryParam := r.URL.Query().Has("q")

	item, err := s.store.Get(asin)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var title, author string
	if item != nil {
		title = item.Title
		author = item.Author
	}

	// Prefill the search box's displayed value from the Audible item's title
	// (per the design spec's "prefilled from the Audible item" requirement)
	// on first load only — i.e. when the caller hasn't supplied a ?q= at
	// all. This deliberately does NOT trigger an automatic search: an empty
	// ?q= (or none) must still skip calling the searcher, exactly as before
	// — the prefill only changes what's shown in the input box.
	prefill := query
	if !hasQueryParam && title != "" {
		prefill = title
	}

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
		"Title":   title,
		"Author":  author,
		"Query":   query,
		"Prefill": prefill,
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

	// Deliberately do NOT carry forward existing.Status/PercentComplete here.
	// Status is left at its zero value ("") so that sync.Run's
	// unchanged-skip check (prior.Status == newly-computed status) can never
	// match a real state.Status constant, which forces the next sync pass to
	// treat this book as changed and actually call Hardcover.UpsertUserBook
	// using the HardcoverBookID cached here. Preserving the prior Status
	// used to make a freshly-resolved book look "unchanged" on the very next
	// run, so it silently never got synced to Hardcover.
	bs := state.BookState{ASIN: asin, HardcoverBookID: &bookID, Unmatched: false, NotifiedUnmatched: false}
	if err := s.store.Upsert(bs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/unmatched", http.StatusSeeOther)
}
