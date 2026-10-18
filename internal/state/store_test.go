package state

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "state.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStore_GetMissing(t *testing.T) {
	s := openTestStore(t)
	bs, err := s.Get("B00WRHY15Y")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if bs != nil {
		t.Errorf("Get() = %+v, want nil for missing ASIN", bs)
	}
}

func TestStore_Open_InvalidPath(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nonexistent-subdir", "state.db")
	s, err := Open(dbPath)
	if err == nil {
		s.Close()
		t.Fatal("Open() error = nil, want error for path under nonexistent directory")
	}
}

func TestStore_UpsertAndGet(t *testing.T) {
	bookID := int64(12345)

	tests := []struct {
		name  string
		in    BookState
		check func(t *testing.T, got *BookState)
	}{
		{
			name: "populated fields round-trip",
			in: BookState{
				ASIN:            "B00WRHY15Y",
				Title:           "The Hobbit",
				Author:          "J.R.R. Tolkien",
				Status:          StatusFinished,
				PercentComplete: 100,
				HardcoverBookID: &bookID,
			},
			check: func(t *testing.T, got *BookState) {
				if got.Status != StatusFinished {
					t.Errorf("Status = %q, want %q", got.Status, StatusFinished)
				}
				if got.Title != "The Hobbit" {
					t.Errorf("Title = %q, want %q", got.Title, "The Hobbit")
				}
				if got.Author != "J.R.R. Tolkien" {
					t.Errorf("Author = %q, want %q", got.Author, "J.R.R. Tolkien")
				}
				if got.HardcoverBookID == nil || *got.HardcoverBookID != bookID {
					t.Errorf("HardcoverBookID = %v, want %d", got.HardcoverBookID, bookID)
				}
			},
		},
		{
			name: "nil fields round-trip as nil, not zero values",
			in: BookState{
				ASIN:            "B00NILFIELDS",
				Status:          StatusInProgress,
				PercentComplete: 40,
				HardcoverBookID: nil,
			},
			check: func(t *testing.T, got *BookState) {
				if got.HardcoverBookID != nil {
					t.Errorf("HardcoverBookID = %v, want nil", got.HardcoverBookID)
				}
				if got.Title != "" {
					t.Errorf("Title = %q, want empty string when not set", got.Title)
				}
				if got.Author != "" {
					t.Errorf("Author = %q, want empty string when not set", got.Author)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := openTestStore(t)
			if err := s.Upsert(tt.in); err != nil {
				t.Fatalf("Upsert() error = %v", err)
			}
			got, err := s.Get(tt.in.ASIN)
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if got == nil {
				t.Fatal("Get() = nil, want a row")
			}
			tt.check(t, got)
		})
	}
}

func TestStore_Upsert_Overwrites(t *testing.T) {
	s := openTestStore(t)
	bookID := int64(12345)

	in := BookState{
		ASIN:            "B00WRHY15Y",
		Status:          StatusFinished,
		PercentComplete: 100,
		HardcoverBookID: &bookID,
	}
	if err := s.Upsert(in); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	// Upsert again with a changed status should overwrite, not duplicate.
	in.Status = StatusInProgress
	in.PercentComplete = 40
	if err := s.Upsert(in); err != nil {
		t.Fatalf("second Upsert() error = %v", err)
	}

	got, err := s.Get("B00WRHY15Y")
	if err != nil {
		t.Fatalf("Get() after update error = %v", err)
	}
	if got == nil {
		t.Fatal("Get() = nil, want a row")
	}
	if got.Status != StatusInProgress {
		t.Errorf("Status after update = %q, want %q", got.Status, StatusInProgress)
	}
	if got.PercentComplete != 40 {
		t.Errorf("PercentComplete after update = %v, want 40", got.PercentComplete)
	}
}

func TestStore_ListUnmatched(t *testing.T) {
	tests := []struct {
		name      string
		in        []BookState
		wantASINs []string
	}{
		{
			name: "returns only unmatched rows",
			in: []BookState{
				{ASIN: "A1", Status: StatusInProgress, Unmatched: true},
				{ASIN: "A2", Status: StatusFinished, Unmatched: false},
			},
			wantASINs: []string{"A1"},
		},
		{
			name: "returns none when nothing is unmatched",
			in: []BookState{
				{ASIN: "B1", Status: StatusInProgress, Unmatched: false},
				{ASIN: "B2", Status: StatusFinished, Unmatched: false},
			},
			wantASINs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := openTestStore(t)
			for _, bs := range tt.in {
				if err := s.Upsert(bs); err != nil {
					t.Fatalf("Upsert(%q) error = %v", bs.ASIN, err)
				}
			}

			unmatched, err := s.ListUnmatched()
			if err != nil {
				t.Fatalf("ListUnmatched() error = %v", err)
			}
			if len(unmatched) != len(tt.wantASINs) {
				t.Fatalf("ListUnmatched() = %+v, want ASINs %v", unmatched, tt.wantASINs)
			}
			for i, want := range tt.wantASINs {
				if unmatched[i].ASIN != want {
					t.Errorf("ListUnmatched()[%d].ASIN = %q, want %q", i, unmatched[i].ASIN, want)
				}
				if unmatched[i].NotifiedUnmatched {
					t.Errorf("ListUnmatched()[%d].NotifiedUnmatched = true before MarkNotified was called", i)
				}
			}
		})
	}
}

func TestStore_MarkNotified(t *testing.T) {
	t.Run("marks an existing row as notified", func(t *testing.T) {
		s := openTestStore(t)
		if err := s.Upsert(BookState{ASIN: "A1", Status: StatusInProgress, Unmatched: true}); err != nil {
			t.Fatalf("Upsert(A1) error = %v", err)
		}

		if err := s.MarkNotified("A1"); err != nil {
			t.Fatalf("MarkNotified() error = %v", err)
		}

		got, err := s.Get("A1")
		if err != nil {
			t.Fatalf("Get(A1) error = %v", err)
		}
		if got == nil {
			t.Fatal("Get(A1) = nil, want a row")
		}
		if !got.NotifiedUnmatched {
			t.Error("NotifiedUnmatched = false after MarkNotified")
		}
	})

	t.Run("is a silent no-op for a nonexistent ASIN", func(t *testing.T) {
		s := openTestStore(t)

		if err := s.MarkNotified("nonexistent-asin"); err != nil {
			t.Fatalf("MarkNotified() error = %v, want nil", err)
		}

		got, err := s.Get("nonexistent-asin")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got != nil {
			t.Errorf("Get() = %+v, want nil (no row should have been created)", got)
		}
	})
}

func TestStore_CoverURLRoundTrip(t *testing.T) {
	s := openTestStore(t)
	in := BookState{ASIN: "B1", Status: StatusFinished, Unmatched: true, CoverURL: "https://m.media-amazon.com/500.jpg"}
	if err := s.Upsert(in); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	got, err := s.Get("B1")
	if err != nil || got == nil {
		t.Fatalf("Get() = %v, %v", got, err)
	}
	if got.CoverURL != in.CoverURL {
		t.Errorf("Get().CoverURL = %q, want %q", got.CoverURL, in.CoverURL)
	}
	list, err := s.ListUnmatched()
	if err != nil || len(list) != 1 {
		t.Fatalf("ListUnmatched() = %v, %v", list, err)
	}
	if list[0].CoverURL != in.CoverURL {
		t.Errorf("ListUnmatched()[0].CoverURL = %q, want %q", list[0].CoverURL, in.CoverURL)
	}
}

func TestStore_Open_AddsCoverURLToExistingDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	// Schema as it existed before cover_url was added.
	if _, err := db.Exec(`CREATE TABLE sync_state (
		asin TEXT PRIMARY KEY, title TEXT NOT NULL DEFAULT '', author TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL, percent_complete REAL NOT NULL DEFAULT 0, hardcover_book_id INTEGER,
		unmatched INTEGER NOT NULL DEFAULT 0, notified_unmatched INTEGER NOT NULL DEFAULT 0,
		updated_at TEXT NOT NULL);
		INSERT INTO sync_state (asin, status, unmatched, updated_at) VALUES ('OLD', 'finished', 1, '2026-01-01T00:00:00Z');`); err != nil {
		t.Fatalf("seed old schema: %v", err)
	}
	db.Close()

	for i := 0; i < 2; i++ { // second open must not fail on the existing column
		s, err := Open(path)
		if err != nil {
			t.Fatalf("Open() #%d error = %v", i+1, err)
		}
		got, err := s.Get("OLD")
		s.Close()
		if err != nil || got == nil || got.CoverURL != "" {
			t.Fatalf("Get(OLD) = %+v, %v; want existing row with empty CoverURL", got, err)
		}
	}
}
