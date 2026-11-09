package app

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/kbuechl/audible-hardcover-sync/internal/state"
)

// SyncIntervals are the choices offered in the web UI.
var SyncIntervals = []time.Duration{
	15 * time.Minute, 30 * time.Minute, time.Hour, 6 * time.Hour, 12 * time.Hour, 24 * time.Hour,
}

const (
	defaultSyncInterval    = 6 * time.Hour
	defaultMatchConfidence = "strict"
	defaultSyncMode        = "confirm_shared"
	defaultReviewNotify    = "after_sync"
)

// SyncModes are the choices for which Audible books sync to Hardcover.
var SyncModes = []string{"all", "skip_shared", "confirm_shared", "confirm_all"}

// ReviewNotifyOptions are the choices for how often the "books waiting for
// review" message is sent.
var ReviewNotifyOptions = []string{"after_sync", "daily", "weekly", "off"}

// matchConfidences maps each preset to the minimum fuzzy-match score.
var matchConfidences = map[string]float64{"strict": 0.95, "medium": 0.75, "loose": 0.55}

// Preferences returns the general settings, with defaults filled in.
func (a *App) Preferences() (state.Preferences, error) {
	p, err := a.store.Preferences()
	if err != nil {
		return state.Preferences{}, err
	}
	if _, err := time.ParseDuration(p.SyncInterval); err != nil {
		p.SyncInterval = defaultSyncInterval.String()
	}
	if _, ok := matchConfidences[p.MatchConfidence]; !ok {
		p.MatchConfidence = defaultMatchConfidence
	}
	if !slices.Contains(SyncModes, p.SyncMode) {
		p.SyncMode = defaultSyncMode
	}
	if !slices.Contains(ReviewNotifyOptions, p.ReviewNotify) {
		p.ReviewNotify = defaultReviewNotify
	}
	return p, nil
}

// SavePreferences validates and saves the general settings. A new sync
// interval takes effect right away.
func (a *App) SavePreferences(p state.Preferences) error {
	d, err := time.ParseDuration(p.SyncInterval)
	if err != nil || !slices.Contains(SyncIntervals, d) {
		return fmt.Errorf("sync interval %q isn't one of the offered choices", p.SyncInterval)
	}
	if _, ok := matchConfidences[p.MatchConfidence]; !ok {
		return fmt.Errorf("match confidence must be strict, medium or loose, not %q", p.MatchConfidence)
	}
	// Empty means the default, so older clients that omit these still work.
	syncMode, reviewNotify := p.SyncMode, p.ReviewNotify
	if syncMode == "" {
		syncMode = defaultSyncMode
	}
	if reviewNotify == "" {
		reviewNotify = defaultReviewNotify
	}
	if !slices.Contains(SyncModes, syncMode) {
		return fmt.Errorf("sync mode %q isn't one of the offered choices", p.SyncMode)
	}
	if !slices.Contains(ReviewNotifyOptions, reviewNotify) {
		return fmt.Errorf("review message frequency %q isn't one of the offered choices", p.ReviewNotify)
	}
	publicURL, err := cleanPublicURL(p.PublicURL)
	if err != nil {
		return err
	}
	saved := state.Preferences{
		SyncInterval: d.String(), MatchConfidence: p.MatchConfidence, PublicURL: publicURL,
		SyncMode: syncMode, ReviewNotify: reviewNotify,
	}
	if err := a.store.SetPreferences(saved); err != nil {
		return err
	}
	// Wake the scheduler; it already has a pending wake-up if this is full.
	select {
	case a.prefsChanged <- struct{}{}:
	default:
	}
	return nil
}

// PreferencesChanged signals after preferences are saved.
func (a *App) PreferencesChanged() <-chan struct{} {
	return a.prefsChanged
}

// SyncInterval is the time between scheduled syncs.
func (a *App) SyncInterval() time.Duration {
	p, err := a.Preferences()
	if err != nil {
		return defaultSyncInterval
	}
	d, err := time.ParseDuration(p.SyncInterval)
	if err != nil {
		return defaultSyncInterval
	}
	return d
}

func cleanPublicURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("web UI address must look like http://192.168.1.10:8080, not %q", raw)
	}
	return raw, nil
}
