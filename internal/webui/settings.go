package webui

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/kbuechl/audible-hardcover-sync/internal/app"
	"github.com/kbuechl/audible-hardcover-sync/internal/hardcover"
	"github.com/kbuechl/audible-hardcover-sync/internal/state"
)

// Service is the part of *app.App behind the setup, sync and notification
// endpoints. Responses never include the Audible login or the Hardcover
// token, only whether they work.
type Service interface {
	Hardcover() *hardcover.Client
	AudibleStatus() app.AudibleStatus
	HardcoverStatus(ctx context.Context) app.HardcoverStatus
	StartAudibleLogin() (string, error)
	FinishAudibleLogin(ctx context.Context, redirectURL string) error
	LogoutAudible(ctx context.Context) error
	SetHardcoverToken(ctx context.Context, token string) (string, error)
	Sync(ctx context.Context) (state.SyncReport, error)
	Syncing() bool
	LastSync() (*state.SyncReport, error)
	ResetSync() error
	NotifyTargets() ([]app.NotifyTarget, error)
	AddNotifyURL(rawURL string) error
	RemoveNotifyURL(id int) error
	TestNotifyTarget(id int) error
	Preferences() (state.Preferences, error)
	SavePreferences(p state.Preferences) error
}

// WithService adds the setup, sync and notification endpoints, and makes
// search use the service's current Hardcover client.
func (s *Server) WithService(svc Service, syncCtx context.Context, testURL func(string) error) *Server {
	s.svc, s.syncCtx, s.testURL = svc, syncCtx, testURL
	return s
}

func (s *Server) registerServiceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("POST /api/audible/login", s.handleAudibleLoginStart)
	mux.HandleFunc("POST /api/audible/login/finish", s.handleAudibleLoginFinish)
	mux.HandleFunc("POST /api/audible/logout", s.handleAudibleLogout)
	mux.HandleFunc("PUT /api/hardcover/token", s.handleHardcoverToken)
	mux.HandleFunc("POST /api/sync", s.handleSyncNow)
	mux.HandleFunc("POST /api/sync/reset", s.handleSyncReset)
	mux.HandleFunc("GET /api/notifications", s.handleNotifyList)
	mux.HandleFunc("POST /api/notifications", s.handleNotifyAdd)
	mux.HandleFunc("POST /api/notifications/test", s.handleNotifyTestURL)
	mux.HandleFunc("DELETE /api/notifications/{id}", s.handleNotifyRemove)
	mux.HandleFunc("POST /api/notifications/{id}/test", s.handleNotifyTestTarget)
	mux.HandleFunc("GET /api/preferences", s.handlePreferencesGet)
	mux.HandleFunc("PUT /api/preferences", s.handlePreferencesPut)
}

type statusDTO struct {
	Audible   app.AudibleStatus   `json:"audible"`
	Hardcover app.HardcoverStatus `json:"hardcover"`
	Sync      syncStatusDTO       `json:"sync"`
	Version   string              `json:"version"`
}

type syncStatusDTO struct {
	Running bool              `json:"running"`
	Last    *state.SyncReport `json:"last,omitempty"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	last, err := s.svc.LastSync()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, statusDTO{
		Audible:   s.svc.AudibleStatus(),
		Hardcover: s.svc.HardcoverStatus(r.Context()),
		Sync:      syncStatusDTO{Running: s.svc.Syncing(), Last: last},
		Version:   s.version,
	})
}

func (s *Server) handleAudibleLoginStart(w http.ResponseWriter, r *http.Request) {
	url, err := s.svc.StartAudibleLogin()
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

func (s *Server) handleAudibleLoginFinish(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RedirectURL string `json:"redirectUrl"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := s.svc.FinishAudibleLogin(r.Context(), req.RedirectURL); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAudibleLogout(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.LogoutAudible(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleHardcoverToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &req) {
		return
	}
	username, err := s.svc.SetHardcoverToken(r.Context(), req.Token)
	if errors.Is(err, app.ErrTokenFromEnv) {
		writeError(w, http.StatusConflict, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"username": username})
}

// handleSyncNow starts a sync in the background and returns at once; the
// UI follows it through /api/status.
func (s *Server) handleSyncNow(w http.ResponseWriter, r *http.Request) {
	if s.svc.Syncing() {
		writeError(w, http.StatusConflict, app.ErrSyncRunning)
		return
	}
	go func() {
		if _, err := s.svc.Sync(s.syncCtx); err != nil && !errors.Is(err, app.ErrSyncRunning) {
			slog.Error("sync from web ui failed", "error", err)
		}
	}()
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleSyncReset(w http.ResponseWriter, r *http.Request) {
	err := s.svc.ResetSync()
	if errors.Is(err, app.ErrSyncRunning) {
		writeError(w, http.StatusConflict, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleNotifyList(w http.ResponseWriter, r *http.Request) {
	targets, err := s.svc.NotifyTargets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, targets)
}

type notifyURLRequest struct {
	URL string `json:"url"`
}

func (s *Server) handleNotifyAdd(w http.ResponseWriter, r *http.Request) {
	var req notifyURLRequest
	if !decode(w, r, &req) {
		return
	}
	if err := s.svc.AddNotifyURL(req.URL); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleNotifyTestURL(w http.ResponseWriter, r *http.Request) {
	var req notifyURLRequest
	if !decode(w, r, &req) {
		return
	}
	if err := s.testURL(req.URL); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleNotifyRemove(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.svc.RemoveNotifyURL(id); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleNotifyTestTarget(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.svc.TestNotifyTarget(id); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func pathID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid id"))
		return 0, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
		return false
	}
	return true
}

// writeError replies with {"error": "..."} so the UI can show the message.
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func (s *Server) handlePreferencesGet(w http.ResponseWriter, r *http.Request) {
	p, err := s.svc.Preferences()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handlePreferencesPut(w http.ResponseWriter, r *http.Request) {
	var p state.Preferences
	if !decode(w, r, &p) {
		return
	}
	if err := s.svc.SavePreferences(p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.handlePreferencesGet(w, r)
}
