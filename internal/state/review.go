package state

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Decision is the user's call on whether a book syncs to Hardcover.
type Decision string

const (
	DecisionNone      Decision = "" // not reviewed yet
	DecisionSync      Decision = "sync"
	DecisionNotWanted Decision = "not_wanted"
)

// ErrUnknownASIN is returned by SetDecision for an ASIN not in the review list.
var ErrUnknownASIN = errors.New("unknown ASIN")

// ReviewBook is one Audible book awaiting, or past, the user's review.
type ReviewBook struct {
	ASIN, Title, Author, CoverURL string
	Shared                        bool // borrowed/shared title, not owned
	Status                        Status
	PercentComplete               float64
	Decision                      Decision
	Notified                      bool // user was told this book is waiting
	FirstSeen                     time.Time
}

// reviewSchema holds review decisions apart from sync_state so ResetSync
// (which clears sync_state) never forgets what the user decided.
const reviewSchema = `
CREATE TABLE IF NOT EXISTS review_books (
    asin TEXT PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    author TEXT NOT NULL DEFAULT '',
    cover_url TEXT NOT NULL DEFAULT '',
    shared INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL,
    percent_complete REAL NOT NULL DEFAULT 0,
    decision TEXT NOT NULL DEFAULT '',
    notified INTEGER NOT NULL DEFAULT 0,
    first_seen TEXT NOT NULL
);`

const reviewColumns = `asin, title, author, cover_url, shared, status, percent_complete, decision, notified, first_seen`

// tableExists reports whether the named table is already in the database.
func tableExists(db *sql.DB, name string) (bool, error) {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&n); err != nil {
		return false, fmt.Errorf("inspect table %s: %w", name, err)
	}
	return n > 0, nil
}

// backfillReviewBooks marks every book already synced to Hardcover as
// Sync and notified, so upgrading doesn't flood the user with books they
// have effectively approved. Run only on the Open that creates review_books.
func backfillReviewBooks(tx *sql.Tx) error {
	_, err := tx.Exec(`
		INSERT OR IGNORE INTO review_books
			(asin, title, author, cover_url, status, percent_complete, decision, notified, first_seen)
		SELECT asin, title, author, cover_url, status, percent_complete, ?, 1, ?
		FROM sync_state WHERE hardcover_book_id IS NOT NULL`,
		string(DecisionSync), time.Now().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("backfill review books: %w", err)
	}
	return nil
}

// initReviewBooks creates review_books and runs backfill in one transaction,
// so a failure leaves no table behind and the next Open retries the backfill.
func initReviewBooks(db *sql.DB, backfill func(*sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("init review books: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(reviewSchema); err != nil {
		return fmt.Errorf("create review_books: %w", err)
	}
	if err := backfill(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func scanReviewBook(scanner func(dest ...any) error) (ReviewBook, error) {
	var b ReviewBook
	var shared, notified int
	var firstSeen string
	if err := scanner(&b.ASIN, &b.Title, &b.Author, &b.CoverURL, &shared, &b.Status,
		&b.PercentComplete, &b.Decision, &notified, &firstSeen); err != nil {
		return b, err
	}
	b.Shared = shared != 0
	b.Notified = notified != 0
	var err error
	b.FirstSeen, err = time.Parse(time.RFC3339, firstSeen)
	if err != nil {
		return b, fmt.Errorf("parse first_seen for %q: %w", b.ASIN, err)
	}
	return b, nil
}

// SeeReviewBook inserts a book or refreshes its Audible-side fields. It
// never overwrites Decision, Notified or FirstSeen.
func (s *Store) SeeReviewBook(b ReviewBook) error {
	_, err := s.db.Exec(`
		INSERT INTO review_books
			(asin, title, author, cover_url, shared, status, percent_complete, first_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(asin) DO UPDATE SET
			title = excluded.title,
			author = excluded.author,
			cover_url = excluded.cover_url,
			shared = excluded.shared,
			status = excluded.status,
			percent_complete = excluded.percent_complete`,
		b.ASIN, b.Title, b.Author, b.CoverURL, boolToInt(b.Shared), b.Status, b.PercentComplete,
		time.Now().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("see review book %q: %w", b.ASIN, err)
	}
	return nil
}

// ReviewBook returns the book with the given ASIN, or nil when unknown.
func (s *Store) ReviewBook(asin string) (*ReviewBook, error) {
	row := s.db.QueryRow(`SELECT `+reviewColumns+` FROM review_books WHERE asin = ?`, asin)
	b, err := scanReviewBook(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get review book %q: %w", asin, err)
	}
	return &b, nil
}

// SetDecision records d for every ASIN in one transaction. An unknown ASIN
// fails the whole call and changes nothing.
func (s *Store) SetDecision(asins []string, d Decision) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("set decision: %w", err)
	}
	defer tx.Rollback()
	for _, asin := range asins {
		res, err := tx.Exec(`UPDATE review_books SET decision = ? WHERE asin = ?`, string(d), asin)
		if err != nil {
			return fmt.Errorf("set decision %q: %w", asin, err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return fmt.Errorf("set decision %q: %w", asin, err)
		} else if n == 0 {
			return fmt.Errorf("set decision: %w %q", ErrUnknownASIN, asin)
		}
	}
	return tx.Commit()
}

func (s *Store) queryReview(what, where string, args ...any) ([]ReviewBook, error) {
	rows, err := s.db.Query(`SELECT `+reviewColumns+` FROM review_books WHERE `+where+` ORDER BY first_seen DESC, asin`, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	defer rows.Close()

	var out []ReviewBook
	for rows.Next() {
		b, err := scanReviewBook(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("%s: scan row: %w", what, err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ListReview returns the books with decision d, newest FirstSeen first.
func (s *Store) ListReview(d Decision) ([]ReviewBook, error) {
	return s.queryReview("list review", `decision = ?`, string(d))
}

// UnnotifiedWaiting returns undecided books the user hasn't been told about.
func (s *Store) UnnotifiedWaiting() ([]ReviewBook, error) {
	return s.queryReview("list unnotified review books", `decision = '' AND notified = 0`)
}

// MarkReviewNotified records that the user was told about these books.
func (s *Store) MarkReviewNotified(asins []string) error {
	if len(asins) == 0 {
		return nil
	}
	args := make([]any, len(asins))
	for i, a := range asins {
		args[i] = a
	}
	q := `UPDATE review_books SET notified = 1 WHERE asin IN (?` + strings.Repeat(",?", len(asins)-1) + `)`
	if _, err := s.db.Exec(q, args...); err != nil {
		return fmt.Errorf("mark review notified: %w", err)
	}
	return nil
}
