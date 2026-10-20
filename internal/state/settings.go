package state

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// settingsSchema holds user settings made in the web UI and the last sync's
// outcome. It is separate from sync_state so resetting sync history keeps
// them.
const settingsSchema = `
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);`

const (
	keyNotifyURLs  = "notify_urls"
	keyLastSync    = "last_sync"
	keyPreferences = "preferences"
)

// Preferences are the general settings made in the web UI. Empty fields
// mean "use the default".
type Preferences struct {
	SyncInterval    string `json:"syncInterval"`
	MatchConfidence string `json:"matchConfidence"`
	PublicURL       string `json:"publicUrl"`
}

// SyncReport is the outcome of the most recent sync, shown in the web UI.
type SyncReport struct {
	FinishedAt time.Time `json:"finishedAt"`
	Synced     int       `json:"synced"`
	Unchanged  int       `json:"unchanged"`
	Unmatched  int       `json:"unmatched"`
	Errored    int       `json:"errored"`
	Error      string    `json:"error,omitempty"`
}

func (s *Store) getJSON(key string, v any) (bool, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get setting %q: %w", key, err)
	}
	if err := json.Unmarshal([]byte(raw), v); err != nil {
		return false, fmt.Errorf("decode setting %q: %w", key, err)
	}
	return true, nil
}

func (s *Store) setJSON(key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode setting %q: %w", key, err)
	}
	_, err = s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, string(raw))
	if err != nil {
		return fmt.Errorf("set setting %q: %w", key, err)
	}
	return nil
}

// NotifyURLs returns the notification URLs saved in the web UI.
func (s *Store) NotifyURLs() ([]string, error) {
	var urls []string
	if _, err := s.getJSON(keyNotifyURLs, &urls); err != nil {
		return nil, err
	}
	return urls, nil
}

// SetNotifyURLs replaces the notification URLs saved in the web UI.
func (s *Store) SetNotifyURLs(urls []string) error {
	return s.setJSON(keyNotifyURLs, urls)
}

// Preferences returns the saved general settings; fields never saved are
// empty.
func (s *Store) Preferences() (Preferences, error) {
	var p Preferences
	_, err := s.getJSON(keyPreferences, &p)
	return p, err
}

// SetPreferences replaces the saved general settings.
func (s *Store) SetPreferences(p Preferences) error {
	return s.setJSON(keyPreferences, p)
}

// LastSync returns the most recent sync's report, or nil before the first.
func (s *Store) LastSync() (*SyncReport, error) {
	var r SyncReport
	found, err := s.getJSON(keyLastSync, &r)
	if err != nil || !found {
		return nil, err
	}
	return &r, nil
}

// SetLastSync records the most recent sync's report.
func (s *Store) SetLastSync(r SyncReport) error {
	return s.setJSON(keyLastSync, r)
}

// ResetSync forgets every book's sync history, so the next sync re-checks
// the whole library. Settings and the Hardcover library are untouched.
func (s *Store) ResetSync() error {
	if _, err := s.db.Exec(`DELETE FROM sync_state`); err != nil {
		return fmt.Errorf("reset sync state: %w", err)
	}
	return nil
}
