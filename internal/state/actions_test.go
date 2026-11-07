package state

import (
	"reflect"
	"testing"
	"time"
)

func TestRecordAction_RoundTrip(t *testing.T) {
	rating := 4
	tests := []struct {
		name string
		in   SyncAction
	}{
		{"all fields", SyncAction{
			ASIN: "A1", Kind: ActionPromoted, UserBookID: 11, ReadID: 22, PrevStatusID: 1,
			PriorReadIDs: []int64{5, 6}, CreatedReadIDs: []int64{7, 8}, Rating: &rating,
			At: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		}},
		{"nil rating and empty id lists", SyncAction{
			ASIN: "A2", Kind: ActionInserted, UserBookID: 12,
			At: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := openTestStore(t)
			if err := s.RecordAction(tt.in); err != nil {
				t.Fatalf("RecordAction() error = %v", err)
			}
			got, err := s.Actions(tt.in.ASIN)
			if err != nil || len(got) != 1 {
				t.Fatalf("Actions() = %+v, %v, want 1 row", got, err)
			}
			want := tt.in
			want.ID = got[0].ID
			if want.ID == 0 {
				t.Error("ID not assigned")
			}
			if !got[0].At.Equal(want.At) {
				t.Errorf("At = %v, want %v", got[0].At, want.At)
			}
			got[0].At, want.At = time.Time{}, time.Time{}
			if !reflect.DeepEqual(got[0], want) {
				t.Errorf("Actions()[0] = %+v, want %+v", got[0], want)
			}
		})
	}
}

// Actions come back in insertion order even when At is out of chronological
// order or in mixed zones, since Remove replays them in the order recorded.
func TestActions_InsertionOrder(t *testing.T) {
	s := openTestStore(t)
	plus5 := time.FixedZone("plus5", 5*3600)
	ats := []time.Time{
		time.Date(2026, 1, 1, 0, 0, 3, 0, time.UTC),
		time.Date(2026, 1, 1, 0, 0, 1, 500, time.UTC),
		time.Date(2026, 1, 1, 4, 0, 2, 0, plus5), // 23:00:02 UTC the day before
	}
	kinds := []ActionKind{ActionInserted, ActionReadStarted, ActionPromoted}
	for i, k := range kinds {
		if err := s.RecordAction(SyncAction{ASIN: "A1", Kind: k, At: ats[i]}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RecordAction(SyncAction{ASIN: "other", Kind: ActionInserted}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Actions("A1")
	if err != nil {
		t.Fatal(err)
	}
	var gotKinds []ActionKind
	for i, a := range got {
		gotKinds = append(gotKinds, a.Kind)
		if !a.At.Equal(ats[i]) {
			t.Errorf("action %d At = %v, want instant %v", i, a.At, ats[i])
		}
		if a.At.Location() != time.UTC {
			t.Errorf("action %d At zone = %v, want UTC", i, a.At.Location())
		}
	}
	if !reflect.DeepEqual(gotKinds, kinds) {
		t.Errorf("kinds = %v, want %v (insertion order, own ASIN only)", gotKinds, kinds)
	}
}

func TestClearActions(t *testing.T) {
	s := openTestStore(t)
	for _, asin := range []string{"A1", "A2"} {
		if err := s.RecordAction(SyncAction{ASIN: asin, Kind: ActionInserted}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.ClearActions("A1"); err != nil {
		t.Fatalf("ClearActions() error = %v", err)
	}
	for asin, want := range map[string]bool{"A1": false, "A2": true} {
		if got, err := s.HasActions(asin); err != nil || got != want {
			t.Errorf("HasActions(%q) = %v, %v, want %v", asin, got, err, want)
		}
	}
}

func TestResetSync_KeepsActions(t *testing.T) {
	s := openTestStore(t)
	if err := s.RecordAction(SyncAction{ASIN: "A1", Kind: ActionInserted}); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetSync(); err != nil {
		t.Fatalf("ResetSync() error = %v", err)
	}
	if got, err := s.HasActions("A1"); err != nil || !got {
		t.Errorf("HasActions() = %v, %v, want true", got, err)
	}
}
