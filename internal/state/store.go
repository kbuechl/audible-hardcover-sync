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
	Title             string
	Author            string
	Status            Status
	PercentComplete   float64
	HardcoverBookID   *int64
	Unmatched         bool
	NotifiedUnmatched bool
	CoverURL          string // Audible cover art, shown on the manual-match page
	UpdatedAt         time.Time
}

type Store struct {
	db *sql.DB
}

// schema creates the sync_state table. NOTE: the rating column was removed
// from this schema (2026-08-02) as dead code. Pre-existing databases still
// carry the column, but no migration is run — the state DB is ephemeral and
// gets recreated on container restart.
const schema = `
CREATE TABLE IF NOT EXISTS sync_state (
    asin TEXT PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    author TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    percent_complete REAL NOT NULL DEFAULT 0,
    hardcover_book_id INTEGER,
    unmatched INTEGER NOT NULL DEFAULT 0,
    notified_unmatched INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL,
    cover_url TEXT NOT NULL DEFAULT ''
);`

// addCoverURLColumn upgrades databases created before cover_url existed.
// CREATE TABLE IF NOT EXISTS leaves an existing table untouched, so the
// column has to be added separately when missing.
func addCoverURLColumn(db *sql.DB) error {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('sync_state') WHERE name = 'cover_url'`).Scan(&n); err != nil {
		return fmt.Errorf("inspect sync_state columns: %w", err)
	}
	if n > 0 {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE sync_state ADD COLUMN cover_url TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add cover_url column: %w", err)
	}
	return nil
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	if err := addCoverURLColumn(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// scanBookState scans a single sync_state row (in the fixed column order
// shared by Get and ListUnmatched) into a BookState, converting NULL
// INTEGER columns to nil pointers and 0/1 INTEGER columns to bools.
// The scanner is row.Scan or rows.Scan, which accept the same variadic
// destination arguments.
func scanBookState(scanner func(dest ...any) error) (BookState, error) {
	var bs BookState
	var updatedAt string
	var hardcoverBookID sql.NullInt64
	var unmatched, notified int

	if err := scanner(&bs.ASIN, &bs.Title, &bs.Author, &bs.Status, &bs.PercentComplete, &hardcoverBookID,
		&unmatched, &notified, &updatedAt, &bs.CoverURL); err != nil {
		return bs, err
	}
	if hardcoverBookID.Valid {
		bs.HardcoverBookID = &hardcoverBookID.Int64
	}
	bs.Unmatched = unmatched != 0
	bs.NotifiedUnmatched = notified != 0
	var err error
	bs.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return bs, fmt.Errorf("parse updated_at for %q: %w", bs.ASIN, err)
	}
	return bs, nil
}

func (s *Store) Get(asin string) (*BookState, error) {
	row := s.db.QueryRow(`
		SELECT asin, title, author, status, percent_complete, hardcover_book_id,
		       unmatched, notified_unmatched, updated_at, cover_url
		FROM sync_state WHERE asin = ?`, asin)

	bs, err := scanBookState(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get %q: %w", asin, err)
	}
	return &bs, nil
}

func (s *Store) Upsert(bs BookState) error {
	_, err := s.db.Exec(`
		INSERT INTO sync_state
			(asin, title, author, status, percent_complete, hardcover_book_id, unmatched, notified_unmatched, updated_at, cover_url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(asin) DO UPDATE SET
			title = excluded.title,
			author = excluded.author,
			status = excluded.status,
			percent_complete = excluded.percent_complete,
			hardcover_book_id = excluded.hardcover_book_id,
			unmatched = excluded.unmatched,
			notified_unmatched = excluded.notified_unmatched,
			updated_at = excluded.updated_at,
			cover_url = excluded.cover_url`,
		bs.ASIN, bs.Title, bs.Author, bs.Status, bs.PercentComplete, bs.HardcoverBookID,
		boolToInt(bs.Unmatched), boolToInt(bs.NotifiedUnmatched), time.Now().Format(time.RFC3339), bs.CoverURL)
	if err != nil {
		return fmt.Errorf("upsert %q: %w", bs.ASIN, err)
	}
	return nil
}

func (s *Store) ListUnmatched() ([]BookState, error) {
	rows, err := s.db.Query(`
		SELECT asin, title, author, status, percent_complete, hardcover_book_id,
		       unmatched, notified_unmatched, updated_at, cover_url
		FROM sync_state WHERE unmatched = 1`)
	if err != nil {
		return nil, fmt.Errorf("list unmatched: %w", err)
	}
	defer rows.Close()

	var out []BookState
	for rows.Next() {
		bs, err := scanBookState(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan unmatched row: %w", err)
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
