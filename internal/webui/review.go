package webui

import (
	"errors"
	"net/http"

	"github.com/kbuechl/audible-hardcover-sync/internal/app"
	"github.com/kbuechl/audible-hardcover-sync/internal/state"
)

type reviewBookDTO struct {
	ASIN            string  `json:"asin"`
	Title           string  `json:"title"`
	Author          string  `json:"author"`
	CoverURL        string  `json:"coverUrl,omitempty"`
	Shared          bool    `json:"shared"`
	Status          string  `json:"status"`
	PercentComplete float64 `json:"percentComplete"`
	Decision        string  `json:"decision"`
	CanRemove       bool    `json:"canRemove"`
}

func (s *Server) handleReviewList(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("filter")
	switch filter {
	case app.ReviewWaiting, app.ReviewSynced, app.ReviewNotWanted:
	default:
		writeError(w, http.StatusBadRequest, errors.New("filter must be waiting, synced or not_wanted"))
		return
	}
	items, err := s.svc.ReviewBooks(filter, r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	dtos := make([]reviewBookDTO, 0, len(items))
	for _, it := range items {
		dtos = append(dtos, reviewBookDTO{
			ASIN: it.ASIN, Title: it.Title, Author: it.Author, CoverURL: it.CoverURL,
			Shared: it.Shared, Status: string(it.Status), PercentComplete: it.PercentComplete,
			Decision: string(it.Decision), CanRemove: it.CanRemove,
		})
	}
	writeJSON(w, http.StatusOK, dtos)
}

func (s *Server) handleReviewDecision(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ASINs    []string `json:"asins"`
		Decision string   `json:"decision"`
	}
	if !decode(w, r, &req) {
		return
	}
	d := state.Decision(req.Decision)
	switch d {
	case state.DecisionSync, state.DecisionNotWanted, state.DecisionNone:
	default:
		writeError(w, http.StatusBadRequest, errors.New(`decision must be "sync", "not_wanted" or ""`))
		return
	}
	if len(req.ASINs) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("choose at least one book"))
		return
	}
	err := s.svc.SetReviewDecision(req.ASINs, d)
	if errors.Is(err, state.ErrUnknownASIN) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
