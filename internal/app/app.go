// Package app ties the pieces together for the long-running service: it
// owns the current Hardcover client, the Audible login, notification
// settings and running syncs, so the web UI and the CLI share one
// implementation.
package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	gosync "sync"
	"sync/atomic"
	"time"

	audiblesdk "github.com/mstrhakr/go-audible"

	"github.com/kbuechl/audible-hardcover-sync/internal/audible"
	"github.com/kbuechl/audible-hardcover-sync/internal/config"
	"github.com/kbuechl/audible-hardcover-sync/internal/hardcover"
	"github.com/kbuechl/audible-hardcover-sync/internal/notify"
	"github.com/kbuechl/audible-hardcover-sync/internal/state"
	syncpkg "github.com/kbuechl/audible-hardcover-sync/internal/sync"
)

// ErrSyncRunning is returned when a sync or reset is requested while a sync
// is already running.
var ErrSyncRunning = errors.New("a sync is already running")

// ErrNotConnected is returned when an action needs a service that hasn't
// been connected yet.
var ErrNotConnected = errors.New("not connected")

type App struct {
	cfg   config.Config
	store *state.Store

	mu         gosync.Mutex
	hc         *hardcover.Client // nil until a Hardcover token is set
	hcUsername string            // cached for status; "" until checked
	pending    *audiblesdk.AuthURL

	running gosync.Mutex // held for the length of a sync
	syncing atomic.Bool

	prefsChanged chan struct{}
}

// New loads the Hardcover token (from the environment, or the one saved
// from the web UI) and returns an App. A missing token is not an error.
func New(cfg config.Config, store *state.Store) (*App, error) {
	a := &App{cfg: cfg, store: store, prefsChanged: make(chan struct{}, 1)}
	token, err := a.hardcoverToken()
	if err != nil {
		return nil, err
	}
	if token != "" {
		a.hc = hardcover.New(token)
	}
	return a, nil
}

// Hardcover returns the current Hardcover client, or nil when no token is
// set. The client changes when a new token is saved.
func (a *App) Hardcover() *hardcover.Client {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.hc
}

// Syncing reports whether a sync is running.
func (a *App) Syncing() bool {
	return a.syncing.Load()
}

// LastSync returns the most recent sync's report, or nil before the first.
func (a *App) LastSync() (*state.SyncReport, error) {
	return a.store.LastSync()
}

// Sync runs one sync pass and records its outcome for the web UI. It
// returns ErrSyncRunning instead of waiting when one is already running.
func (a *App) Sync(ctx context.Context) (state.SyncReport, error) {
	if !a.running.TryLock() {
		return state.SyncReport{}, ErrSyncRunning
	}
	defer a.running.Unlock()
	// Missing setup isn't a failed sync, so it isn't recorded as one.
	deps, err := a.syncDeps()
	if err != nil {
		return state.SyncReport{}, err
	}
	a.syncing.Store(true)
	defer a.syncing.Store(false)

	result, err := syncpkg.Run(ctx, deps)
	report := state.SyncReport{
		FinishedAt: time.Now().UTC(),
		Synced:     result.Synced,
		Unchanged:  result.Unchanged,
		Unmatched:  result.Unmatched,
		Errored:    result.Errored,
	}
	if err != nil {
		report.Error = err.Error()
	}
	if saveErr := a.store.SetLastSync(report); saveErr != nil {
		slog.Error("record sync result", "error", saveErr)
	}
	if err != nil {
		return report, err
	}
	slog.Info("sync complete", "synced", result.Synced, "unchanged", result.Unchanged,
		"unmatched", result.Unmatched, "errored", result.Errored)
	return report, nil
}

// syncDeps builds a sync's dependencies, failing with ErrNotConnected when
// Hardcover or Audible isn't set up yet.
func (a *App) syncDeps() (syncpkg.Deps, error) {
	hc := a.Hardcover()
	if hc == nil {
		return syncpkg.Deps{}, fmt.Errorf("Hardcover isn't connected: add your API token in the web UI or set HARDCOVER_API_TOKEN: %w", ErrNotConnected)
	}
	audibleClient, err := a.loadAudible()
	if err != nil {
		return syncpkg.Deps{}, err
	}
	urls, err := a.store.NotifyURLs()
	if err != nil {
		return syncpkg.Deps{}, err
	}
	prefs, err := a.Preferences()
	if err != nil {
		return syncpkg.Deps{}, err
	}
	return syncpkg.Deps{
		Audible:         audibleClient,
		Hardcover:       hc,
		Store:           a.store,
		Notifier:        notify.New(urls, prefs.PublicURL),
		MatchConfidence: matchConfidences[prefs.MatchConfidence],
	}, nil
}

// ResetSync forgets every book's sync history so the next sync re-checks
// the whole library. Settings, logins and the Hardcover library are kept.
func (a *App) ResetSync() error {
	if !a.running.TryLock() {
		return ErrSyncRunning
	}
	defer a.running.Unlock()
	return a.store.ResetSync()
}

func (a *App) newAudibleClient() *audible.Client {
	c := audible.New(audiblesdk.MarketplaceUS)
	c.SetCredentialsKey(a.cfg.CredentialsKey)
	return c
}

func (a *App) loadAudible() (*audible.Client, error) {
	c := a.newAudibleClient()
	if err := c.LoadCredentialsFile(a.cfg.CredentialsPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("Audible isn't connected: connect it in the web UI or run 'sync-tool auth': %w", ErrNotConnected)
		}
		return nil, fmt.Errorf("load audible credentials: %w", err)
	}
	return c, nil
}
