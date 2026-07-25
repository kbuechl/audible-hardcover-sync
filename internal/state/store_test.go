package state

import (
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
	rating := 4.5
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
				Status:          StatusFinished,
				PercentComplete: 100,
				Rating:          &rating,
				HardcoverBookID: &bookID,
			},
			check: func(t *testing.T, got *BookState) {
				if got.Status != StatusFinished {
					t.Errorf("Status = %q, want %q", got.Status, StatusFinished)
				}
				if got.HardcoverBookID == nil || *got.HardcoverBookID != bookID {
					t.Errorf("HardcoverBookID = %v, want %d", got.HardcoverBookID, bookID)
				}
				if got.Rating == nil || *got.Rating != 4.5 {
					t.Errorf("Rating = %v, want 4.5", got.Rating)
				}
			},
		},
		{
			name: "nil fields round-trip as nil, not zero values",
			in: BookState{
				ASIN:            "B00NILFIELDS",
				Status:          StatusInProgress,
				PercentComplete: 40,
				Rating:          nil,
				HardcoverBookID: nil,
			},
			check: func(t *testing.T, got *BookState) {
				if got.Rating != nil {
					t.Errorf("Rating = %v, want nil", got.Rating)
				}
				if got.HardcoverBookID != nil {
					t.Errorf("HardcoverBookID = %v, want nil", got.HardcoverBookID)
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
	rating := 4.5
	bookID := int64(12345)

	in := BookState{
		ASIN:            "B00WRHY15Y",
		Status:          StatusFinished,
		PercentComplete: 100,
		Rating:          &rating,
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
