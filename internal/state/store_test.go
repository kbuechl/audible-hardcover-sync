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

func TestStore_UpsertAndGet(t *testing.T) {
	s := openTestStore(t)
	bookID := int64(12345)
	rating := 4.5

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

	got, err := s.Get("B00WRHY15Y")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil {
		t.Fatal("Get() = nil, want a row")
	}
	if got.Status != StatusFinished {
		t.Errorf("Status = %q, want %q", got.Status, StatusFinished)
	}
	if got.HardcoverBookID == nil || *got.HardcoverBookID != bookID {
		t.Errorf("HardcoverBookID = %v, want %d", got.HardcoverBookID, bookID)
	}
	if got.Rating == nil || *got.Rating != 4.5 {
		t.Errorf("Rating = %v, want 4.5", got.Rating)
	}

	// Upsert again with a changed status should overwrite, not duplicate.
	in.Status = StatusInProgress
	in.PercentComplete = 40
	if err := s.Upsert(in); err != nil {
		t.Fatalf("second Upsert() error = %v", err)
	}
	got, err = s.Get("B00WRHY15Y")
	if err != nil {
		t.Fatalf("Get() after update error = %v", err)
	}
	if got.Status != StatusInProgress {
		t.Errorf("Status after update = %q, want %q", got.Status, StatusInProgress)
	}
}

func TestStore_ListUnmatchedAndMarkNotified(t *testing.T) {
	s := openTestStore(t)

	if err := s.Upsert(BookState{ASIN: "A1", Status: StatusInProgress, Unmatched: true}); err != nil {
		t.Fatalf("Upsert(A1) error = %v", err)
	}
	if err := s.Upsert(BookState{ASIN: "A2", Status: StatusFinished, Unmatched: false}); err != nil {
		t.Fatalf("Upsert(A2) error = %v", err)
	}

	unmatched, err := s.ListUnmatched()
	if err != nil {
		t.Fatalf("ListUnmatched() error = %v", err)
	}
	if len(unmatched) != 1 || unmatched[0].ASIN != "A1" {
		t.Fatalf("ListUnmatched() = %+v, want only A1", unmatched)
	}
	if unmatched[0].NotifiedUnmatched {
		t.Error("NotifiedUnmatched = true before MarkNotified was called")
	}

	if err := s.MarkNotified("A1"); err != nil {
		t.Fatalf("MarkNotified() error = %v", err)
	}
	got, err := s.Get("A1")
	if err != nil {
		t.Fatalf("Get(A1) error = %v", err)
	}
	if !got.NotifiedUnmatched {
		t.Error("NotifiedUnmatched = false after MarkNotified")
	}
}
