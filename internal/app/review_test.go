package app

import (
	"errors"
	"reflect"
	"testing"

	"github.com/kbuechl/audible-hardcover-sync/internal/state"
)

func seedReview(t *testing.T, a *App, mode string) {
	t.Helper()
	for _, b := range []state.ReviewBook{
		{ASIN: "A1", Title: "Dune", Status: state.StatusFinished},
		{ASIN: "A2", Title: "Dune Messiah", Shared: true, Status: state.StatusInProgress},
		{ASIN: "A3", Title: "Emma", Status: state.StatusFinished},
		{ASIN: "A4", Title: "Persuasion", Status: state.StatusFinished},
	} {
		if err := a.store.SeeReviewBook(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.store.SetDecision([]string{"A3"}, state.DecisionSync); err != nil {
		t.Fatal(err)
	}
	if err := a.store.SetDecision([]string{"A4"}, state.DecisionNotWanted); err != nil {
		t.Fatal(err)
	}
	if err := a.SavePreferences(state.Preferences{SyncInterval: "30m", MatchConfidence: "strict", SyncMode: mode}); err != nil {
		t.Fatal(err)
	}
}

func asinsOf(items []ReviewItem) []string {
	out := []string{}
	for _, it := range items {
		out = append(out, it.ASIN)
	}
	return out
}

func TestReviewBooks(t *testing.T) {
	tests := []struct {
		name         string
		mode, filter string
		q            string
		want         []string
	}{
		{"waiting confirm_all", "confirm_all", "waiting", "", []string{"A1", "A2"}},
		{"waiting confirm_shared", "confirm_shared", "waiting", "", []string{"A2"}},
		{"waiting skip_shared excludes shared", "skip_shared", "waiting", "", []string{}},
		{"waiting all", "all", "waiting", "", []string{}},
		{"synced", "confirm_all", "synced", "", []string{"A3"}},
		{"not wanted", "confirm_all", "not_wanted", "", []string{"A4"}},
		{"search is case-insensitive substring", "confirm_all", "waiting", "DUNE m", []string{"A2"}},
		{"search no match", "confirm_all", "waiting", "zzz", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestApp(t, nil)
			seedReview(t, a, tt.mode)
			items, err := a.ReviewBooks(tt.filter, tt.q)
			if err != nil {
				t.Fatal(err)
			}
			got := asinsOf(items)
			if len(got) == len(tt.want) {
				// order is newest first; compare as sets
				m := map[string]bool{}
				for _, g := range got {
					m[g] = true
				}
				for _, w := range tt.want {
					delete(m, w)
				}
				if len(m) == 0 {
					return
				}
			}
			t.Errorf("ReviewBooks(%q, %q) = %v, want %v", tt.filter, tt.q, got, tt.want)
		})
	}
}

func TestReviewBooks_UnknownFilterAndCanRemove(t *testing.T) {
	a := newTestApp(t, nil)
	seedReview(t, a, "confirm_all")
	if _, err := a.ReviewBooks("bogus", ""); err == nil {
		t.Error("unknown filter accepted")
	}
	if err := a.store.RecordAction(state.SyncAction{ASIN: "A3", Kind: state.ActionInserted}); err != nil {
		t.Fatal(err)
	}
	items, err := a.ReviewBooks("synced", "")
	if err != nil || len(items) != 1 || !items[0].CanRemove {
		t.Errorf("synced = %+v, %v; want one removable item", items, err)
	}
	items, _ = a.ReviewBooks("not_wanted", "")
	if len(items) != 1 || items[0].CanRemove {
		t.Errorf("not_wanted = %+v, want one non-removable item", items)
	}
}

func TestWaitingCountFollowsMode(t *testing.T) {
	a := newTestApp(t, nil)
	seedReview(t, a, "confirm_all")
	if n, err := a.WaitingCount(); err != nil || n != 2 {
		t.Errorf("confirm_all = %d, %v; want 2", n, err)
	}
	seedReview(t, a, "all")
	if n, _ := a.WaitingCount(); n != 0 {
		t.Errorf("all = %d, want 0", n)
	}
}

func TestSetReviewDecision(t *testing.T) {
	a := newTestApp(t, nil)
	seedReview(t, a, "confirm_all")
	if err := a.SetReviewDecision([]string{"A1"}, state.DecisionSync); err != nil {
		t.Fatal(err)
	}
	if rb, _ := a.store.ReviewBook("A1"); rb.Decision != state.DecisionSync {
		t.Errorf("decision = %q", rb.Decision)
	}
	if err := a.SetReviewDecision([]string{"nope"}, state.DecisionSync); !errors.Is(err, state.ErrUnknownASIN) {
		t.Errorf("unknown ASIN error = %v, want ErrUnknownASIN", err)
	}
}

func TestHiddenFromMatches(t *testing.T) {
	a := newTestApp(t, nil)
	seedReview(t, a, "confirm_shared")
	got, err := a.HiddenFromMatches([]string{"A1", "A2", "A3", "A4", "unseen"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"A2": true, "A4": true} // held shared, Not wanted
	if !reflect.DeepEqual(got, want) {
		t.Errorf("confirm_shared hidden = %v, want %v", got, want)
	}
	seedReview(t, a, "all")
	got, _ = a.HiddenFromMatches([]string{"A1", "A2", "A3", "A4"})
	if !reflect.DeepEqual(got, map[string]bool{"A4": true}) {
		t.Errorf("all hidden = %v, want only A4", got)
	}
}
