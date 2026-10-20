// cmd/sync-tool/serve.go
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/kbuechl/audible-hardcover-sync/internal/app"
	"github.com/kbuechl/audible-hardcover-sync/internal/config"
	"github.com/kbuechl/audible-hardcover-sync/internal/state"
	"github.com/kbuechl/audible-hardcover-sync/internal/webui"
)

// openApp opens the state store and builds the App around it. The caller
// closes the returned store.
func openApp(cfg config.Config) (*app.App, *state.Store, error) {
	store, err := state.Open(cfg.StateDBPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open state store: %w", err)
	}
	a, err := app.New(cfg, store)
	if err != nil {
		store.Close()
		return nil, nil, err
	}
	return a, store, nil
}

// runOnce performs a single sync pass (the "sync" subcommand).
func runOnce(ctx context.Context, cfg config.Config) (state.SyncReport, error) {
	a, store, err := openApp(cfg)
	if err != nil {
		return state.SyncReport{}, err
	}
	defer store.Close()
	return a.Sync(ctx)
}

// runServe starts the web UI and runs a sync at startup and then every
// sync interval (a web UI setting), blocking until ctx is cancelled. On cancellation it
// shuts down the HTTP server cleanly and returns ctx.Err().
func runServe(ctx context.Context, cfg config.Config) error {
	a, store, err := openApp(cfg)
	if err != nil {
		return err
	}
	defer store.Close()

	var searcher webui.HardcoverSearcher
	if hc := a.Hardcover(); hc != nil {
		searcher = hc
	}
	webServer := webui.New(store, searcher).WithService(a, ctx, app.TestNotifyURL)

	httpServer := &http.Server{Addr: ":" + cfg.WebUIPort, Handler: webServer.Handler()}
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("web ui server", "error", err)
		}
	}()

	syncNow := func() {
		_, err := a.Sync(ctx)
		switch {
		case errors.Is(err, app.ErrSyncRunning):
			slog.Info("sync already running, skipping")
		case errors.Is(err, app.ErrNotConnected):
			slog.Info("skipping sync until setup is finished in the web UI", "reason", err)
		case err != nil:
			slog.Error("scheduled sync failed", "error", err)
		}
	}

	// Sync once at startup instead of waiting a full interval.
	go syncNow()
	schedule(ctx, a.SyncInterval, a.PreferencesChanged(), func() { go syncNow() })

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	return ctx.Err()
}

// schedule calls run every interval() until ctx is cancelled. A signal on
// changed restarts the wait with the current interval, so a new interval
// takes effect without a restart.
func schedule(ctx context.Context, interval func() time.Duration, changed <-chan struct{}, run func()) {
	timer := time.NewTimer(interval())
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			run()
			timer.Reset(interval())
		case <-changed:
			timer.Reset(interval())
		}
	}
}
