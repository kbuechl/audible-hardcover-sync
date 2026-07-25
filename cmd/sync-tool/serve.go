// cmd/sync-tool/serve.go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	audiblesdk "github.com/mstrhakr/go-audible"
	"github.com/robfig/cron/v3"

	"github.com/kbuechl/audible-hardcover-sync/internal/audible"
	"github.com/kbuechl/audible-hardcover-sync/internal/config"
	"github.com/kbuechl/audible-hardcover-sync/internal/hardcover"
	"github.com/kbuechl/audible-hardcover-sync/internal/notify"
	"github.com/kbuechl/audible-hardcover-sync/internal/state"
	syncpkg "github.com/kbuechl/audible-hardcover-sync/internal/sync"
	"github.com/kbuechl/audible-hardcover-sync/internal/webui"
)

// runOnce performs a single sync pass, wiring together the Audible client,
// Hardcover client, state store, and notifier. It loads Audible credentials
// from cfg.CredentialsPath (written previously by the "auth" subcommand),
// opens the state store, and delegates to sync.Run.
func runOnce(ctx context.Context, cfg config.Config) (syncpkg.Result, error) {
	audibleClient := audible.New(audiblesdk.MarketplaceUS)
	if err := audibleClient.LoadCredentialsFile(cfg.CredentialsPath); err != nil {
		return syncpkg.Result{}, fmt.Errorf("load audible credentials (run 'sync-tool auth' first): %w", err)
	}

	store, err := state.Open(cfg.StateDBPath)
	if err != nil {
		return syncpkg.Result{}, fmt.Errorf("open state store: %w", err)
	}
	defer store.Close()

	hc := hardcover.New(cfg.HardcoverAPIToken)
	notifier := notify.New(cfg.NotifyURLs, cfg.PublicURL)

	result, err := syncpkg.Run(ctx, syncpkg.Deps{
		Audible:   audibleClient,
		Hardcover: hc,
		Store:     store,
		Notifier:  notifier,
	})
	if err != nil {
		return result, err
	}

	slog.Info("sync complete", "synced", result.Synced, "unchanged", result.Unchanged,
		"unmatched", result.Unmatched, "errored", result.Errored)
	return result, nil
}

// runServe starts the web UI HTTP server and schedules runOnce on a cron
// interval (cfg.SyncInterval), blocking until ctx is cancelled. On
// cancellation it shuts down the HTTP server cleanly and returns ctx.Err().
func runServe(ctx context.Context, cfg config.Config) error {
	store, err := state.Open(cfg.StateDBPath)
	if err != nil {
		return fmt.Errorf("open state store: %w", err)
	}
	defer store.Close()

	hc := hardcover.New(cfg.HardcoverAPIToken)
	webServer := webui.New(store, hc)

	httpServer := &http.Server{Addr: ":" + cfg.WebUIPort, Handler: webServer.Handler()}
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("web ui server", "error", err)
		}
	}()

	c := cron.New()
	spec := fmt.Sprintf("@every %s", cfg.SyncInterval)
	if _, err := c.AddFunc(spec, func() {
		if _, err := runOnce(ctx, cfg); err != nil {
			slog.Error("scheduled sync failed", "error", err)
		}
	}); err != nil {
		return fmt.Errorf("schedule sync: %w", err)
	}
	c.Start()
	defer c.Stop()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	return ctx.Err()
}
