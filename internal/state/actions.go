package state

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ActionKind is the kind of Hardcover write the sync made for a book.
type ActionKind string

const (
	ActionInserted    ActionKind = "inserted"     // added the book to the library
	ActionPromoted    ActionKind = "promoted"     // moved an existing book to a new status
	ActionReadStarted ActionKind = "read_started" // started a re-listen read
)

// SyncAction is one Hardcover write, logged so "Remove from Hardcover" can
// undo exactly what the app added and nothing the user did themselves.
type SyncAction struct {
	ID             int64
	ASIN           string
	Kind           ActionKind
	UserBookID     int64
	ReadID         int64 // read_started: the read opened; promoted: the open read finished in place
	PrevStatusID   int
	PriorReadIDs   []int64 // read ids present before the write
	CreatedReadIDs []int64 // read ids the write created
	Rating         *int    // rating the app sent (inserted/promoted only)
	At             time.Time
}

// actionsSchema lives apart from sync_state so ResetSync keeps the log.
const actionsSchema = `
CREATE TABLE IF NOT EXISTS sync_actions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    asin TEXT NOT NULL,
    kind TEXT NOT NULL,
    user_book_id INTEGER NOT NULL DEFAULT 0,
    read_id INTEGER NOT NULL DEFAULT 0,
    prev_status_id INTEGER NOT NULL DEFAULT 0,
    prior_read_ids TEXT NOT NULL DEFAULT 'null',
    created_read_ids TEXT NOT NULL DEFAULT 'null',
    rating INTEGER,
    at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sync_actions_asin ON sync_actions (asin);`

// RecordAction appends a to the log. A zero At is stamped with the current time.
func (s *Store) RecordAction(a SyncAction) error {
	prior, err := json.Marshal(a.PriorReadIDs)
	if err != nil {
		return fmt.Errorf("encode prior read ids: %w", err)
	}
	created, err := json.Marshal(a.CreatedReadIDs)
	if err != nil {
		return fmt.Errorf("encode created read ids: %w", err)
	}
	var rating sql.NullInt64
	if a.Rating != nil {
		rating = sql.NullInt64{Int64: int64(*a.Rating), Valid: true}
	}
	at := a.At
	if at.IsZero() {
		at = time.Now()
	}
	_, err = s.db.Exec(`
		INSERT INTO sync_actions
			(asin, kind, user_book_id, read_id, prev_status_id, prior_read_ids, created_read_ids, rating, at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ASIN, string(a.Kind), a.UserBookID, a.ReadID, a.PrevStatusID, string(prior), string(created),
		rating, at.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("record action for %q: %w", a.ASIN, err)
	}
	return nil
}

// Actions returns the logged writes for asin in the order they were recorded,
// which is the replay order Remove needs (At is informational only).
func (s *Store) Actions(asin string) ([]SyncAction, error) {
	rows, err := s.db.Query(`
		SELECT id, asin, kind, user_book_id, read_id, prev_status_id, prior_read_ids, created_read_ids, rating, at
		FROM sync_actions WHERE asin = ? ORDER BY id`, asin)
	if err != nil {
		return nil, fmt.Errorf("list actions for %q: %w", asin, err)
	}
	defer rows.Close()

	var out []SyncAction
	for rows.Next() {
		var a SyncAction
		var prior, created, at string
		var rating sql.NullInt64
		if err := rows.Scan(&a.ID, &a.ASIN, &a.Kind, &a.UserBookID, &a.ReadID, &a.PrevStatusID,
			&prior, &created, &rating, &at); err != nil {
			return nil, fmt.Errorf("list actions for %q: scan row: %w", asin, err)
		}
		if err := json.Unmarshal([]byte(prior), &a.PriorReadIDs); err != nil {
			return nil, fmt.Errorf("decode prior read ids for action %d: %w", a.ID, err)
		}
		if err := json.Unmarshal([]byte(created), &a.CreatedReadIDs); err != nil {
			return nil, fmt.Errorf("decode created read ids for action %d: %w", a.ID, err)
		}
		if rating.Valid {
			r := int(rating.Int64)
			a.Rating = &r
		}
		if a.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return nil, fmt.Errorf("parse at for action %d: %w", a.ID, err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ClearActions forgets the logged writes for asin.
func (s *Store) ClearActions(asin string) error {
	if _, err := s.db.Exec(`DELETE FROM sync_actions WHERE asin = ?`, asin); err != nil {
		return fmt.Errorf("clear actions for %q: %w", asin, err)
	}
	return nil
}

// HasActions reports whether the sync has logged any write for asin.
func (s *Store) HasActions(asin string) (bool, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sync_actions WHERE asin = ?`, asin).Scan(&n); err != nil {
		return false, fmt.Errorf("count actions for %q: %w", asin, err)
	}
	return n > 0, nil
}
