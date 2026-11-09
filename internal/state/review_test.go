package state

import (
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func seeBook(t *testing.T, s *Store, asin, title string) {
	t.Helper()
	if err := s.SeeReviewBook(ReviewBook{ASIN: asin, Title: title, Status: StatusInProgress}); err != nil {
		t.Fatalf("SeeReviewBook(%q) error = %v", asin, err)
	}
}

func mustReview(t *testing.T, s *Store, asin string) *ReviewBook {
	t.Helper()
	b, err := s.ReviewBook(asin)
	if err != nil {
		t.Fatalf("ReviewBook(%q) error = %v", asin, err)
	}
	if b == nil {
		t.Fatalf("ReviewBook(%q) = nil, want a row", asin)
	}
	return b
}

func TestSeeReviewBook_KeepsDecision(t *testing.T) {
	s := openTestStore(t)
	seeBook(t, s, "A1", "Old Title")
	if err := s.SetDecision([]string{"A1"}, DecisionNotWanted); err != nil {
		t.Fatalf("SetDecision() error = %v", err)
	}
	first := mustReview(t, s, "A1")

	err := s.SeeReviewBook(ReviewBook{
		ASIN: "A1", Title: "New Title", Author: "Au", CoverURL: "http://c", Shared: true,
		Status: StatusFinished, PercentComplete: 100,
	})
	if err != nil {
		t.Fatalf("SeeReviewBook() error = %v", err)
	}
	got, err := s.ReviewBook("A1")
	if err != nil || got == nil {
		t.Fatalf("ReviewBook() = %v, %v", got, err)
	}
	if got.Title != "New Title" || got.Author != "Au" || got.CoverURL != "http://c" || !got.Shared ||
		got.Status != StatusFinished || got.PercentComplete != 100 {
		t.Errorf("fields not refreshed: %+v", got)
	}
	if got.Decision != DecisionNotWanted {
		t.Errorf("Decision = %q, want %q", got.Decision, DecisionNotWanted)
	}
	if !got.FirstSeen.Equal(first.FirstSeen) {
		t.Errorf("FirstSeen changed: %v -> %v", first.FirstSeen, got.FirstSeen)
	}
}

func TestReviewBook_Missing(t *testing.T) {
	s := openTestStore(t)
	got, err := s.ReviewBook("nope")
	if err != nil || got != nil {
		t.Errorf("ReviewBook() = %v, %v, want nil, nil", got, err)
	}
}

func TestSetDecision_Bulk(t *testing.T) {
	s := openTestStore(t)
	seeBook(t, s, "A1", "One")
	seeBook(t, s, "A2", "Two")
	seeBook(t, s, "A3", "Three")
	if err := s.SetDecision([]string{"A1", "A2"}, DecisionSync); err != nil {
		t.Fatalf("SetDecision() error = %v", err)
	}
	for asin, want := range map[string]Decision{"A1": DecisionSync, "A2": DecisionSync, "A3": DecisionNone} {
		got := mustReview(t, s, asin)
		if got.Decision != want {
			t.Errorf("%s Decision = %q, want %q", asin, got.Decision, want)
		}
	}
}

func TestSetDecision_UnknownASIN(t *testing.T) {
	s := openTestStore(t)
	seeBook(t, s, "A1", "One")
	if err := s.SetDecision([]string{"A1", "ghost"}, DecisionSync); err == nil {
		t.Fatal("SetDecision() error = nil, want error for unknown ASIN")
	}
	got := mustReview(t, s, "A1")
	if got.Decision != DecisionNone {
		t.Errorf("A1 Decision = %q, want unchanged after rolled-back transaction", got.Decision)
	}
}

func TestListReview_ByDecision(t *testing.T) {
	s := openTestStore(t)
	for _, asin := range []string{"A1", "A2", "A3"} {
		seeBook(t, s, asin, asin)
	}
	// Force distinct FirstSeen values so ordering is deterministic.
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, asin := range []string{"A1", "A2", "A3"} {
		if _, err := s.db.Exec(`UPDATE review_books SET first_seen = ? WHERE asin = ?`,
			base.Add(time.Duration(i)*time.Hour).Format(time.RFC3339), asin); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetDecision([]string{"A1", "A3"}, DecisionSync); err != nil {
		t.Fatal(err)
	}

	asins := func(d Decision) []string {
		list, err := s.ListReview(d)
		if err != nil {
			t.Fatalf("ListReview(%q) error = %v", d, err)
		}
		var out []string
		for _, b := range list {
			out = append(out, b.ASIN)
		}
		return out
	}
	if got, want := asins(DecisionSync), []string{"A3", "A1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("sync list = %v, want %v", got, want)
	}
	if got, want := asins(DecisionNone), []string{"A2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("none list = %v, want %v", got, want)
	}
	if got := asins(DecisionNotWanted); len(got) != 0 {
		t.Errorf("not_wanted list = %v, want empty", got)
	}
}

func TestUnnotifiedWaiting(t *testing.T) {
	s := openTestStore(t)
	for _, asin := range []string{"A1", "A2", "A3", "A4"} {
		seeBook(t, s, asin, asin)
	}
	if err := s.SetDecision([]string{"A3"}, DecisionSync); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDecision([]string{"A4"}, DecisionNotWanted); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkReviewNotified([]string{"A2"}); err != nil {
		t.Fatalf("MarkReviewNotified() error = %v", err)
	}

	list, err := s.UnnotifiedWaiting()
	if err != nil {
		t.Fatalf("UnnotifiedWaiting() error = %v", err)
	}
	if len(list) != 1 || list[0].ASIN != "A1" {
		t.Errorf("UnnotifiedWaiting() = %+v, want only A1", list)
	}
	got := mustReview(t, s, "A2")
	if !got.Notified {
		t.Error("A2 Notified = false, want true")
	}
}

func TestOpen_BackfillsSyncedBooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	id := int64(42)
	if err := s.Upsert(BookState{ASIN: "MATCHED", Title: "M", Author: "Au", Status: StatusFinished, PercentComplete: 100, HardcoverBookID: &id, CoverURL: "u"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Upsert(BookState{ASIN: "UNMATCHED", Title: "U", Status: StatusInProgress, Unmatched: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DROP TABLE review_books`); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got := mustReview(t, s, "MATCHED")
	if got.Decision != DecisionSync || !got.Notified || got.Title != "M" || got.Status != StatusFinished {
		t.Errorf("MATCHED = %+v, want backfilled Sync+Notified", got)
	}
	if u, err := s.ReviewBook("UNMATCHED"); err != nil || u != nil {
		t.Errorf("UNMATCHED = %+v (err %v), want no review row", u, err)
	}

	// A later Open must not backfill again once the user changed decisions.
	if err := s.SetDecision([]string{"MATCHED"}, DecisionNotWanted); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := mustReview(t, s, "MATCHED"); got.Decision != DecisionNotWanted {
		t.Errorf("Decision after reopen = %q, want not_wanted", got.Decision)
	}
}

func TestResetSync_KeepsReviewBooks(t *testing.T) {
	s := openTestStore(t)
	seeBook(t, s, "A1", "One")
	if err := s.SetDecision([]string{"A1"}, DecisionSync); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetSync(); err != nil {
		t.Fatalf("ResetSync() error = %v", err)
	}
	got, err := s.ReviewBook("A1")
	if err != nil || got == nil || got.Decision != DecisionSync {
		t.Errorf("ReviewBook() = %+v, %v, want decision kept", got, err)
	}
}

func TestInitReviewBooks_FailedBackfillRetries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	id := int64(1)
	if err := s.Upsert(BookState{ASIN: "M", Title: "M", Status: StatusFinished, HardcoverBookID: &id}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DROP TABLE review_books`); err != nil {
		t.Fatal(err)
	}

	if err := initReviewBooks(s.db, func(*sql.Tx) error { return errors.New("boom") }); err == nil {
		t.Fatal("initReviewBooks() error = nil, want backfill error")
	}
	if exists, err := tableExists(s.db, "review_books"); err != nil || exists {
		t.Fatalf("tableExists = %v, %v, want false after failed backfill", exists, err)
	}
	s.Close()

	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := mustReview(t, s, "M"); got.Decision != DecisionSync {
		t.Errorf("Decision = %q, want backfilled sync on retry", got.Decision)
	}
}
