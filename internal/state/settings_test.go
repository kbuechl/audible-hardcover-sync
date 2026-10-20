package state

import (
	"testing"
	"time"
)

func TestSettings_NotifyURLsRoundTrip(t *testing.T) {
	s := openTestStore(t)
	if urls, err := s.NotifyURLs(); err != nil || len(urls) != 0 {
		t.Fatalf("NotifyURLs() on empty store = %v, %v", urls, err)
	}
	want := []string{"discord://token@id", "ntfy://ntfy.sh/topic"}
	if err := s.SetNotifyURLs(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.NotifyURLs()
	if err != nil || len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("NotifyURLs() = %v, %v, want %v", got, err, want)
	}
}

func TestSettings_LastSync(t *testing.T) {
	s := openTestStore(t)
	if r, err := s.LastSync(); err != nil || r != nil {
		t.Fatalf("LastSync() before any sync = %v, %v", r, err)
	}
	want := SyncReport{FinishedAt: time.Date(2026, 10, 19, 1, 2, 3, 0, time.UTC), Synced: 3, Unmatched: 1, Error: "boom"}
	if err := s.SetLastSync(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.LastSync()
	if err != nil || got == nil || !got.FinishedAt.Equal(want.FinishedAt) || got.Synced != 3 || got.Error != "boom" {
		t.Errorf("LastSync() = %+v, %v", got, err)
	}
}

func TestResetSync_KeepsSettings(t *testing.T) {
	s := openTestStore(t)
	if err := s.Upsert(BookState{ASIN: "A1", Status: StatusFinished}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetNotifyURLs([]string{"ntfy://ntfy.sh/x"}); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetSync(); err != nil {
		t.Fatal(err)
	}
	if bs, _ := s.Get("A1"); bs != nil {
		t.Errorf("book state survived reset: %+v", bs)
	}
	if urls, _ := s.NotifyURLs(); len(urls) != 1 {
		t.Errorf("settings lost on reset: %v", urls)
	}
}

func TestSettings_PreferencesRoundTrip(t *testing.T) {
	s := openTestStore(t)
	if p, err := s.Preferences(); err != nil || p != (Preferences{}) {
		t.Fatalf("Preferences() on empty store = %+v, %v", p, err)
	}
	want := Preferences{SyncInterval: "30m", MatchConfidence: "medium", PublicURL: "http://nas:8084"}
	if err := s.SetPreferences(want); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Preferences(); err != nil || got != want {
		t.Errorf("Preferences() = %+v, %v, want %+v", got, err, want)
	}
}
