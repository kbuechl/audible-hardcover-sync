package state

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Status string

const (
	StatusNotStarted Status = "not_started"
	StatusInProgress Status = "in_progress"
	StatusFinished   Status = "finished"
)

type BookState struct {
	ASIN              string
	Status            Status
	PercentComplete   float64
	Rating            *float64
	HardcoverBookID   *int64
	Unmatched         bool
	NotifiedUnmatched bool
	UpdatedAt         time.Time
}

type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS sync_state (
    asin TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    percent_complete REAL NOT NULL DEFAULT 0,
    rating REAL,
    hardcover_book_id INTEGER,
    unmatched INTEGER NOT NULL DEFAULT 0,
    notified_unmatched INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL
);`

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Get(asin string) (*BookState, error) {
	row := s.db.QueryRow(`
		SELECT asin, status, percent_complete, rating, hardcover_book_id,
		       unmatched, notified_unmatched, updated_at
		FROM sync_state WHERE asin = ?`, asin)

	var bs BookState
	var updatedAt string
	var rating sql.NullFloat64
	var hardcoverBookID sql.NullInt64
	var unmatched, notified int

	err := row.Scan(&bs.ASIN, &bs.Status, &bs.PercentComplete, &rating, &hardcoverBookID,
		&unmatched, &notified, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get %q: %w", asin, err)
	}

	if rating.Valid {
		bs.Rating = &rating.Float64
	}
	if hardcoverBookID.Valid {
		bs.HardcoverBookID = &hardcoverBookID.Int64
	}
	bs.Unmatched = unmatched != 0
	bs.NotifiedUnmatched = notified != 0
	bs.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at for %q: %w", asin, err)
	}
	return &bs, nil
}

func (s *Store) Upsert(bs BookState) error {
	_, err := s.db.Exec(`
		INSERT INTO sync_state
			(asin, status, percent_complete, rating, hardcover_book_id, unmatched, notified_unmatched, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(asin) DO UPDATE SET
			status = excluded.status,
			percent_complete = excluded.percent_complete,
			rating = excluded.rating,
			hardcover_book_id = excluded.hardcover_book_id,
			unmatched = excluded.unmatched,
			notified_unmatched = excluded.notified_unmatched,
			updated_at = excluded.updated_at`,
		bs.ASIN, bs.Status, bs.PercentComplete, bs.Rating, bs.HardcoverBookID,
		boolToInt(bs.Unmatched), boolToInt(bs.NotifiedUnmatched), time.Now().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("upsert %q: %w", bs.ASIN, err)
	}
	return nil
}

func (s *Store) ListUnmatched() ([]BookState, error) {
	rows, err := s.db.Query(`
		SELECT asin, status, percent_complete, rating, hardcover_book_id,
		       unmatched, notified_unmatched, updated_at
		FROM sync_state WHERE unmatched = 1`)
	if err != nil {
		return nil, fmt.Errorf("list unmatched: %w", err)
	}
	defer rows.Close()

	var out []BookState
	for rows.Next() {
		var bs BookState
		var updatedAt string
		var rating sql.NullFloat64
		var hardcoverBookID sql.NullInt64
		var unmatched, notified int

		if err := rows.Scan(&bs.ASIN, &bs.Status, &bs.PercentComplete, &rating, &hardcoverBookID,
			&unmatched, &notified, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan unmatched row: %w", err)
		}
		if rating.Valid {
			bs.Rating = &rating.Float64
		}
		if hardcoverBookID.Valid {
			bs.HardcoverBookID = &hardcoverBookID.Int64
		}
		bs.Unmatched = unmatched != 0
		bs.NotifiedUnmatched = notified != 0
		bs.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse updated_at for %q: %w", bs.ASIN, err)
		}
		out = append(out, bs)
	}
	return out, rows.Err()
}

func (s *Store) MarkNotified(asin string) error {
	_, err := s.db.Exec(`UPDATE sync_state SET notified_unmatched = 1 WHERE asin = ?`, asin)
	if err != nil {
		return fmt.Errorf("mark notified %q: %w", asin, err)
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
