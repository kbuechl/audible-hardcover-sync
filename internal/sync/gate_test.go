package sync

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/kbuechl/audible-hardcover-sync/internal/audible"
	"github.com/kbuechl/audible-hardcover-sync/internal/hardcover"
	"github.com/kbuechl/audible-hardcover-sync/internal/state"
)

func TestAllowed(t *testing.T) {
	// want[mode] = {own, shared} for an undecided book.
	undecided := map[string][2]bool{
		"all":            {true, true},
		"skip_shared":    {true, false},
		"confirm_shared": {true, false},
		"confirm_all":    {false, false},
	}
	for mode, want := range undecided {
		for i, shared := range []bool{false, true} {
			item := audible.LibraryItem{ASIN: "A", Shared: shared}
			if got := allowed(mode, item, nil); got != want[i] {
				t.Errorf("allowed(%s, shared=%v, no row) = %v, want %v", mode, shared, got, want[i])
			}
			if got := allowed(mode, item, &state.ReviewBook{Decision: state.DecisionNone}); got != want[i] {
				t.Errorf("allowed(%s, shared=%v, none) = %v, want %v", mode, shared, got, want[i])
			}
			if !allowed(mode, item, &state.ReviewBook{Decision: state.DecisionSync}) {
				t.Errorf("allowed(%s, shared=%v, sync) = false, want true", mode, shared)
			}
			if allowed(mode, item, &state.ReviewBook{Decision: state.DecisionNotWanted}) {
				t.Errorf("allowed(%s, shared=%v, not_wanted) = true, want false", mode, shared)
			}
		}
	}
}

func TestHeld(t *testing.T) {
	tests := []struct {
		mode   string
		shared bool
		d      state.Decision
		want   bool
	}{
		{"all", true, state.DecisionNone, false},
		{"skip_shared", true, state.DecisionNone, false}, // skipped, not waiting
		{"confirm_shared", true, state.DecisionNone, true},
		{"confirm_shared", false, state.DecisionNone, false},
		{"confirm_all", false, state.DecisionNone, true},
		{"confirm_all", false, state.DecisionSync, false},
		{"confirm_all", false, state.DecisionNotWanted, false},
	}
	for _, tt := range tests {
		if got := Held(tt.mode, tt.shared, tt.d); got != tt.want {
			t.Errorf("Held(%s, %v, %q) = %v, want %v", tt.mode, tt.shared, tt.d, got, tt.want)
		}
	}
}

func runMode(t *testing.T, mode string, items []audible.LibraryItem, fh *fakeHardcover, store *state.Store, fn *fakeNotifier) Result {
	t.Helper()
	res, err := Run(context.Background(), Deps{
		Audible: &fakeAudible{items: items}, Hardcover: fh, Store: store, Notifier: fn,
		MatchConfidence: 0.95, SyncMode: mode,
	})
	if err != nil {
		t.Fatalf("Run(%s) error = %v", mode, err)
	}
	return res
}

func storedDecision(t *testing.T, store *state.Store, asin string) state.Decision {
	t.Helper()
	rb, err := store.ReviewBook(asin)
	if err != nil || rb == nil {
		t.Fatalf("ReviewBook(%s) = %v, %v", asin, rb, err)
	}
	return rb.Decision
}

func TestRun_ConfirmAllHoldsBooks(t *testing.T) {
	fh := newFakeHardcover()
	fh.byASIN = map[string]int64{"A1": 777}
	store := newTestStore(t)
	fn := &fakeNotifier{}
	items := []audible.LibraryItem{
		{ASIN: "A1", Title: "Matched", IsFinished: true, PercentComplete: 100},
		{ASIN: "A2", Title: "Unmatchable", IsFinished: true, PercentComplete: 100},
	}
	res := runMode(t, "confirm_all", items, fh, store, fn)
	if res.Waiting != 2 || res.Synced != 0 || res.Unmatched != 0 {
		t.Errorf("Result = %+v, want Waiting 2 only", res)
	}
	if len(fh.inserted) != 0 || len(fn.notified) != 0 {
		t.Errorf("inserted %v notified %v, want none", fh.inserted, fn.notified)
	}
	if bs, _ := store.Get("A2"); bs != nil {
		t.Errorf("held book got sync state %+v", bs)
	}
	if rb, _ := store.ReviewBook("A2"); rb == nil || rb.Title != "Unmatchable" {
		t.Errorf("review row = %+v, want recorded", rb)
	}
}

func TestRun_NotWantedNeverSyncs(t *testing.T) {
	for _, mode := range []string{"all", "skip_shared", "confirm_shared", "confirm_all"} {
		t.Run(mode, func(t *testing.T) {
			fh := newFakeHardcover()
			fh.byASIN = map[string]int64{"A1": 777}
			// Already on Hardcover and progressing: still no writes.
			fh.userBooks[777] = hardcover.UserBook{ID: 50, StatusID: hardcover.StatusCurrentlyReading}
			store := newTestStore(t)
			if err := store.SeeReviewBook(state.ReviewBook{ASIN: "A1"}); err != nil {
				t.Fatal(err)
			}
			if err := store.SetDecision([]string{"A1"}, state.DecisionNotWanted); err != nil {
				t.Fatal(err)
			}
			items := []audible.LibraryItem{{ASIN: "A1", PercentComplete: 40, RuntimeSeconds: 3600}}
			res := runMode(t, mode, items, fh, store, &fakeNotifier{})
			if res.Skipped != 1 || res.Synced != 0 {
				t.Errorf("Result = %+v, want Skipped 1", res)
			}
			if len(fh.inserted) != 0 || len(fh.progress) != 0 {
				t.Errorf("wrote to Hardcover: inserted %v progress %v", fh.inserted, fh.progress)
			}
		})
	}
}

func TestRun_SkipSharedSkipsWithoutDecision(t *testing.T) {
	fh := newFakeHardcover()
	fh.byASIN = map[string]int64{"S1": 777}
	store := newTestStore(t)
	items := []audible.LibraryItem{{ASIN: "S1", Shared: true, IsFinished: true, PercentComplete: 100}}
	res := runMode(t, "skip_shared", items, fh, store, &fakeNotifier{})
	if res.Skipped != 1 || res.Waiting != 0 || len(fh.inserted) != 0 {
		t.Errorf("Result = %+v inserted %v, want Skipped 1", res, fh.inserted)
	}
	if d := storedDecision(t, store, "S1"); d != state.DecisionNone {
		t.Errorf("decision = %q, want none", d)
	}
}

func TestRun_ModeSwitchSyncsWaiting(t *testing.T) {
	fh := newFakeHardcover()
	fh.byASIN = map[string]int64{"A1": 777, "A2": 888}
	store := newTestStore(t)
	items := []audible.LibraryItem{
		{ASIN: "A1", IsFinished: true, PercentComplete: 100},
		{ASIN: "A2", IsFinished: true, PercentComplete: 100},
	}
	runMode(t, "confirm_all", items, fh, store, &fakeNotifier{})
	if err := store.SetDecision([]string{"A2"}, state.DecisionNotWanted); err != nil {
		t.Fatal(err)
	}
	res := runMode(t, "all", items, fh, store, &fakeNotifier{})
	if res.Synced != 1 || res.Skipped != 1 {
		t.Errorf("Result = %+v, want Synced 1 Skipped 1", res)
	}
	if _, ok := fh.inserted[777]; !ok {
		t.Error("waiting book was not inserted after switching to all")
	}
	if _, ok := fh.inserted[888]; ok {
		t.Error("Not wanted book was inserted")
	}
}

func TestRun_SyncedBookGetsSyncDecision(t *testing.T) {
	fh := newFakeHardcover()
	fh.byASIN = map[string]int64{"A1": 777, "A2": 888}
	fh.userBooks[888] = hardcover.UserBook{ID: 60, StatusID: hardcover.StatusRead}
	store := newTestStore(t)
	items := []audible.LibraryItem{
		{ASIN: "A1", IsFinished: true, PercentComplete: 100}, // insert path
		{ASIN: "A2", IsFinished: true, PercentComplete: 100}, // existing, nothing written
		{ASIN: "A3", IsFinished: true, PercentComplete: 100}, // unmatched
	}
	runMode(t, "all", items, fh, store, &fakeNotifier{})
	for asin, want := range map[string]state.Decision{"A1": state.DecisionSync, "A2": state.DecisionSync, "A3": state.DecisionNone} {
		if d := storedDecision(t, store, asin); d != want {
			t.Errorf("%s decision = %q, want %q", asin, d, want)
		}
	}
}

func TestRun_UnchangedBackfillGetsSyncDecision(t *testing.T) {
	fh := newFakeHardcover()
	store := newTestStore(t)
	if err := store.Upsert(state.BookState{ASIN: "A1", Status: state.StatusFinished, PercentComplete: 100, HardcoverBookID: int64Ptr(777)}); err != nil {
		t.Fatal(err)
	}
	items := []audible.LibraryItem{{ASIN: "A1", Title: "Kept", IsFinished: true, PercentComplete: 100}}
	res := runMode(t, "confirm_all", items, fh, store, &fakeNotifier{})
	// No review row yet (none was backfilled), so confirm_all holds it.
	if res.Waiting != 1 {
		t.Errorf("Result = %+v, want Waiting 1", res)
	}
	res = runMode(t, "all", items, fh, store, &fakeNotifier{})
	if res.Unchanged != 1 {
		t.Errorf("Result = %+v, want Unchanged 1", res)
	}
	rb, _ := store.ReviewBook("A1")
	if rb.Decision != state.DecisionSync || rb.Title != "Kept" {
		t.Errorf("review row = %+v, want Sync with title", rb)
	}
}

func TestRun_RecordsActions(t *testing.T) {
	fh := newFakeHardcover()
	fh.byASIN = map[string]int64{"INS": 777, "PRO": 888, "PRO2": 889, "REL": 999}
	fh.userBooks[888] = hardcover.UserBook{ID: 60, StatusID: hardcover.StatusWantToRead}
	fh.userBooks[889] = hardcover.UserBook{ID: 61, StatusID: hardcover.StatusWantToRead}
	fh.userBooks[999] = hardcover.UserBook{ID: 70, StatusID: hardcover.StatusRead, LastReadDate: ptrTime("2025-01-01T00:00:00Z")}
	fh.existingReads = map[int64][]int64{60: {600}}
	fh.insertRead = true
	store := newTestStore(t)
	items := []audible.LibraryItem{
		{ASIN: "INS", PercentComplete: 30, RuntimeSeconds: 3600, Rating: intPtr(4)},
		{ASIN: "PRO", PercentComplete: 20, RuntimeSeconds: 3600},
		{ASIN: "PRO2", PercentComplete: 20, RuntimeSeconds: 3600},
		{ASIN: "REL", PercentComplete: 5, RuntimeSeconds: 3600, FinishedAt: ptrTime("2026-10-01T00:00:00Z")},
	}
	runMode(t, "all", items, fh, store, &fakeNotifier{})

	ins := onlyAction(t, store, "INS")
	if ins.Kind != state.ActionInserted || ins.UserBookID != 5001 || ins.Rating == nil || *ins.Rating != 4 || len(ins.CreatedReadIDs) != 1 {
		t.Errorf("insert action = %+v", ins)
	}
	pro := onlyAction(t, store, "PRO")
	if pro.Kind != state.ActionPromoted || pro.UserBookID != 60 || pro.PrevStatusID != hardcover.StatusWantToRead ||
		!slices.Equal(pro.PriorReadIDs, []int64{600}) || len(pro.CreatedReadIDs) != 0 {
		t.Errorf("promote action = %+v, want prior [600] and nothing created (progress reused read 600)", pro)
	}
	pro2 := onlyAction(t, store, "PRO2")
	if pro2.Kind != state.ActionPromoted || len(pro2.PriorReadIDs) != 0 || len(pro2.CreatedReadIDs) != 1 {
		t.Errorf("promote action = %+v, want one created read", pro2)
	}
	rel := onlyAction(t, store, "REL")
	if rel.Kind != state.ActionReadStarted || rel.UserBookID != 70 || fh.reads[rel.ReadID] == nil || fh.reads[rel.ReadID].userBookID != 70 {
		t.Errorf("relisten action = %+v", rel)
	}
}

func TestRun_AdoptedReadNotRecorded(t *testing.T) {
	fh := newFakeHardcover()
	fh.byASIN = map[string]int64{"REL": 999}
	fh.userBooks[999] = hardcover.UserBook{ID: 70, StatusID: hardcover.StatusRead, LastReadDate: ptrTime("2025-01-01T00:00:00Z")}
	fh.reads[8000] = &fakeRead{userBookID: 70, startedAt: *ptrTime("2026-10-02T00:00:00Z")}
	store := newTestStore(t)
	items := []audible.LibraryItem{{ASIN: "REL", PercentComplete: 5, RuntimeSeconds: 3600, FinishedAt: ptrTime("2026-10-01T00:00:00Z")}}
	runMode(t, "all", items, fh, store, &fakeNotifier{})
	if acts, _ := store.Actions("REL"); len(acts) != 0 {
		t.Errorf("actions = %+v, want none for an adopted read", acts)
	}
}

// Hardcover returns the read it adds for a book inserted as Read in the
// insert's response; a listing right after misses it. The sync must
// still log that read as its own.
func TestRun_InsertRecordsLateRead(t *testing.T) {
	fh := newFakeHardcover()
	fh.byASIN = map[string]int64{"FIN": 777}
	fh.insertRead = true
	store := newTestStore(t)
	items := []audible.LibraryItem{{ASIN: "FIN", IsFinished: true, PercentComplete: 100, FinishedAt: ptrTime("2026-10-01T00:00:00Z")}}
	runMode(t, "all", items, fh, store, &fakeNotifier{})

	ins := onlyAction(t, store, "FIN")
	if want := fh.existingReads[ins.UserBookID]; !slices.Equal(ins.CreatedReadIDs, want) || len(want) != 1 {
		t.Errorf("insert action CreatedReadIDs = %v, want %v", ins.CreatedReadIDs, want)
	}
}

func onlyAction(t *testing.T, store *state.Store, asin string) state.SyncAction {
	t.Helper()
	acts, err := store.Actions(asin)
	if err != nil || len(acts) != 1 {
		t.Fatalf("Actions(%s) = %+v, %v; want one", asin, acts, err)
	}
	return acts[0]
}

// Promoting to Read makes Hardcover add a read that the status change's
// response holds but a listing right after misses. The sync must still
// log that read as its own, and never the user's earlier one.
func TestRun_PromoteRecordsLateRead(t *testing.T) {
	fh := newFakeHardcover()
	fh.byASIN = map[string]int64{"PRO": 888}
	fh.userBooks[888] = hardcover.UserBook{ID: 60, StatusID: hardcover.StatusWantToRead}
	fh.existingReads = map[int64][]int64{60: {600}}
	fh.insertRead = true
	store := newTestStore(t)
	items := []audible.LibraryItem{{ASIN: "PRO", IsFinished: true, PercentComplete: 100, FinishedAt: ptrTime("2026-10-01T00:00:00Z")}}
	runMode(t, "all", items, fh, store, &fakeNotifier{})

	pro := onlyAction(t, store, "PRO")
	if !slices.Equal(pro.PriorReadIDs, []int64{600}) || len(pro.CreatedReadIDs) != 1 || pro.CreatedReadIDs[0] == 600 {
		t.Errorf("promote action = %+v, want prior [600] and the late read created", pro)
	}
}

// After Remove forgets a book's sync state, setting it back to Sync must
// add it to Hardcover again.
func TestRunReinsertsAfterRemoveAndResync(t *testing.T) {
	fh := newFakeHardcover()
	fh.byASIN = map[string]int64{"A1": 777}
	store := newTestStore(t)
	if err := store.SeeReviewBook(state.ReviewBook{ASIN: "A1"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetDecision([]string{"A1"}, state.DecisionSync); err != nil {
		t.Fatal(err)
	}
	items := []audible.LibraryItem{{ASIN: "A1", IsFinished: true, PercentComplete: 100}}
	res := runMode(t, "confirm_all", items, fh, store, &fakeNotifier{})
	if res.Synced != 1 || len(fh.inserted) != 1 {
		t.Errorf("Result = %+v inserted %v, want one insert", res, fh.inserted)
	}
}

func TestRun_ConfirmSharedMixed(t *testing.T) {
	fh := newFakeHardcover()
	fh.byASIN = map[string]int64{"SH": 777, "OWN": 778}
	store := newTestStore(t)
	items := []audible.LibraryItem{
		{ASIN: "SH", Shared: true, IsFinished: true, PercentComplete: 100},
		{ASIN: "OWN", IsFinished: true, PercentComplete: 100},
	}
	res := runMode(t, "confirm_shared", items, fh, store, &fakeNotifier{})
	if res.Waiting != 1 || res.Synced != 1 {
		t.Errorf("Result = %+v, want Waiting 1 Synced 1", res)
	}
	if _, ok := fh.inserted[777]; ok {
		t.Errorf("shared book written to Hardcover")
	}
	if _, ok := fh.inserted[778]; !ok {
		t.Errorf("owned book not inserted")
	}
	if acts, _ := store.Actions("SH"); len(acts) != 0 {
		t.Errorf("shared actions = %+v, want none", acts)
	}
}

// When the reads before a promote cannot be listed, which reads are new
// is unknown, so none are claimed and Remove cannot delete a user's read.
func TestRun_PromoteClaimsNothingWithoutPriorReads(t *testing.T) {
	fh := newFakeHardcover()
	fh.byASIN = map[string]int64{"PRO": 888}
	fh.userBooks[888] = hardcover.UserBook{ID: 60, StatusID: hardcover.StatusWantToRead}
	fh.existingReads = map[int64][]int64{60: {600}}
	fh.insertRead, fh.readIDsErr = true, errors.New("hardcover down")
	store := newTestStore(t)
	items := []audible.LibraryItem{{ASIN: "PRO", IsFinished: true, PercentComplete: 100, FinishedAt: ptrTime("2026-10-01T00:00:00Z")}}
	runMode(t, "all", items, fh, store, &fakeNotifier{})

	pro := onlyAction(t, store, "PRO")
	if pro.Kind != state.ActionPromoted || len(pro.CreatedReadIDs) != 0 {
		t.Errorf("promote action = %+v, want nothing created", pro)
	}
}
