package app

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/kbuechl/audible-hardcover-sync/internal/hardcover"
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

func TestHiddenFromMatches_UnseenBook(t *testing.T) {
	a := newTestApp(t, nil)
	seedReview(t, a, "confirm_all")
	got, err := a.HiddenFromMatches([]string{"unseen"})
	if err != nil || !got["unseen"] {
		t.Errorf("confirm_all unseen hidden = %v, %v; want hidden", got, err)
	}
	seedReview(t, a, "confirm_shared")
	if got, _ := a.HiddenFromMatches([]string{"unseen"}); got["unseen"] {
		t.Errorf("confirm_shared unseen hidden = %v; want shown", got)
	}
}

// fakeRemover records Remove's Hardcover calls in order.
type fakeRemover struct {
	details map[int64]*hardcover.UserBookDetails
	failOn  string
	calls   []string
}

func (f *fakeRemover) step(s string) error {
	f.calls = append(f.calls, s)
	if f.failOn == s {
		return errors.New("hardcover down")
	}
	return nil
}
func (f *fakeRemover) UserBookDetails(_ context.Context, id int64) (*hardcover.UserBookDetails, error) {
	return f.details[id], nil
}
func (f *fakeRemover) DeleteUserBook(_ context.Context, id int64) error {
	return f.step("delete entry " + itoa(id))
}
func (f *fakeRemover) DeleteRead(_ context.Context, id int64) error {
	return f.step("delete read " + itoa(id))
}
func (f *fakeRemover) UpdateUserBookStatus(_ context.Context, id int64, statusID int, _ *int, _ *time.Time) error {
	return f.step("status " + itoa(id) + " " + itoa(int64(statusID)))
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func intp(n int) *int { return &n }

func seedActions(t *testing.T, a *App, acts ...state.SyncAction) {
	t.Helper()
	if err := a.store.SeeReviewBook(state.ReviewBook{ASIN: "B1", Title: "Dune", Status: state.StatusFinished}); err != nil {
		t.Fatal(err)
	}
	if err := a.store.SetDecision([]string{"B1"}, state.DecisionSync); err != nil {
		t.Fatal(err)
	}
	for _, act := range acts {
		act.ASIN = "B1"
		if err := a.store.RecordAction(act); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPreviewRemove(t *testing.T) {
	inserted := state.SyncAction{Kind: state.ActionInserted, UserBookID: 200, Rating: intp(4), CreatedReadIDs: []int64{11}}
	relisten := state.SyncAction{Kind: state.ActionReadStarted, UserBookID: 200, ReadID: 12}
	promoted := state.SyncAction{Kind: state.ActionPromoted, UserBookID: 200, PrevStatusID: hardcover.StatusWantToRead, PriorReadIDs: []int64{5}, CreatedReadIDs: []int64{11}}
	tests := []struct {
		name    string
		acts    []state.SyncAction
		details *hardcover.UserBookDetails
		want    RemovePreview
	}{
		{"inserted, untouched", []state.SyncAction{inserted, relisten},
			&hardcover.UserBookDetails{Rating: ptrFloat(4), ReadIDs: []int64{11, 12}},
			RemovePreview{Deletes: []string{"the re-listen read the sync started", "the book from your Hardcover library"}, AlsoDeletes: []string{}}},
		{"user rating, review, notes and reads", []state.SyncAction{inserted},
			&hardcover.UserBookDetails{Rating: ptrFloat(3.5), HasReview: true, HasNotes: true, ReadIDs: []int64{11, 30, 31}},
			RemovePreview{Deletes: []string{"the book from your Hardcover library"},
				AlsoDeletes: []string{"your 3.5-star rating", "your review", "your private notes", "2 reads you added"}}},
		{"rating when the app sent none", []state.SyncAction{{Kind: state.ActionInserted, UserBookID: 200}},
			&hardcover.UserBookDetails{Rating: ptrFloat(4), ReadIDs: []int64{30}},
			RemovePreview{Deletes: []string{"the book from your Hardcover library"}, AlsoDeletes: []string{"your 4-star rating", "1 read you added"}}},
		{"entry already gone", []state.SyncAction{inserted, relisten}, nil,
			RemovePreview{Deletes: []string{"the re-listen read the sync started"}, AlsoDeletes: []string{}}},
		{"promoted keeps the entry, so nothing else goes", []state.SyncAction{promoted},
			&hardcover.UserBookDetails{Rating: ptrFloat(2), HasReview: true, ReadIDs: []int64{5, 11}},
			RemovePreview{Deletes: []string{"the reads the sync added, and moves the book back to Want to Read"}, AlsoDeletes: []string{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestApp(t, nil)
			a.removeHC = &fakeRemover{details: map[int64]*hardcover.UserBookDetails{200: tt.details}}
			seedActions(t, a, tt.acts...)
			got, err := a.PreviewRemove(context.Background(), "B1")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PreviewRemove() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestRemove(t *testing.T) {
	tests := []struct {
		name      string
		acts      []state.SyncAction
		details   *hardcover.UserBookDetails
		failOn    string
		wantCalls []string
		wantErr   bool
	}{
		{"inserted then re-listen: read, then entry",
			[]state.SyncAction{{Kind: state.ActionInserted, UserBookID: 200, CreatedReadIDs: []int64{11}}, {Kind: state.ActionReadStarted, UserBookID: 200, ReadID: 12}},
			&hardcover.UserBookDetails{}, "",
			[]string{"delete read 12", "delete entry 200"}, false},
		{"promoted: only created reads, back to Want to Read",
			[]state.SyncAction{{Kind: state.ActionPromoted, UserBookID: 200, PrevStatusID: hardcover.StatusWantToRead, PriorReadIDs: []int64{5}, CreatedReadIDs: []int64{11}}},
			&hardcover.UserBookDetails{ReadIDs: []int64{5, 11, 30}}, "",
			[]string{"delete read 11", "status 200 1"}, false},
		{"promoted entry already gone: success",
			[]state.SyncAction{{Kind: state.ActionPromoted, UserBookID: 200, PrevStatusID: hardcover.StatusWantToRead, CreatedReadIDs: []int64{11}}},
			nil, "",
			[]string{"delete read 11"}, false},
		{"inserted entry already gone: success",
			[]state.SyncAction{{Kind: state.ActionInserted, UserBookID: 200}},
			nil, "",
			[]string{"delete entry 200"}, false},
		{"hardcover error aborts",
			[]state.SyncAction{{Kind: state.ActionInserted, UserBookID: 200}, {Kind: state.ActionReadStarted, UserBookID: 200, ReadID: 12}},
			&hardcover.UserBookDetails{}, "delete entry 200",
			[]string{"delete read 12", "delete entry 200"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestApp(t, nil)
			f := &fakeRemover{details: map[int64]*hardcover.UserBookDetails{200: tt.details}, failOn: tt.failOn}
			a.removeHC = f
			seedActions(t, a, tt.acts...)
			err := a.Remove(context.Background(), "B1")
			if (err != nil) != tt.wantErr {
				t.Fatalf("Remove() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(f.calls, tt.wantCalls) {
				t.Errorf("calls = %v, want %v", f.calls, tt.wantCalls)
			}
			has, _ := a.store.HasActions("B1")
			rb, _ := a.store.ReviewBook("B1")
			if tt.wantErr {
				if !has || rb.Decision != state.DecisionSync {
					t.Errorf("after failure: actions kept=%v decision=%q, want kept and sync", has, rb.Decision)
				}
				return
			}
			if has || rb.Decision != state.DecisionNotWanted {
				t.Errorf("after remove: actions kept=%v decision=%q, want cleared and not_wanted", has, rb.Decision)
			}
		})
	}
}

func TestRemove_NoActions(t *testing.T) {
	a := newTestApp(t, nil)
	a.removeHC = &fakeRemover{}
	if _, err := a.PreviewRemove(context.Background(), "B1"); !errors.Is(err, ErrNothingToRemove) {
		t.Errorf("PreviewRemove() error = %v, want ErrNothingToRemove", err)
	}
	if err := a.Remove(context.Background(), "B1"); !errors.Is(err, ErrNothingToRemove) {
		t.Errorf("Remove() error = %v, want ErrNothingToRemove", err)
	}
}

func ptrFloat(f float64) *float64 { return &f }
