package sync

import (
	"context"
	"testing"
	"time"

	"github.com/kbuechl/audible-hardcover-sync/internal/audible"
	"github.com/kbuechl/audible-hardcover-sync/internal/hardcover"
	"github.com/kbuechl/audible-hardcover-sync/internal/state"
)

const relistenISBN = "9780641723445"

func relistenItem(pct float64, finished bool, finishedAt *time.Time) audible.LibraryItem {
	return audible.LibraryItem{
		ASIN: "A1", ISBN13: relistenISBN, PercentComplete: pct, IsFinished: finished,
		FinishedAt: finishedAt, RuntimeSeconds: 36000,
	}
}

func readBook(lastRead string) hardcover.UserBook {
	return hardcover.UserBook{ID: 200, BookID: 777, StatusID: hardcover.StatusRead, AudioSeconds: 36000, LastReadDate: ptrTime(lastRead + "T00:00:00Z")}
}

func runOnce(t *testing.T, fa *fakeAudible, fh *fakeHardcover, store *state.Store) Result {
	t.Helper()
	res, err := Run(context.Background(), Deps{Audible: fa, Hardcover: fh, Store: store, Notifier: &fakeNotifier{}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	return res
}

func newRelistenFixture(t *testing.T, existing hardcover.UserBook, prior *state.BookState) (*fakeHardcover, *state.Store) {
	t.Helper()
	fh := newFakeHardcover()
	fh.byISBN[relistenISBN] = 777
	fh.userBooks[777] = existing
	store := newTestStore(t)
	if prior != nil {
		if err := store.Upsert(*prior); err != nil {
			t.Fatalf("seed state: %v", err)
		}
	}
	return fh, store
}

func TestRelisten_StartTriggers(t *testing.T) {
	restart := ptrTime("2026-10-18T05:29:00Z")
	baseline := &state.BookState{ASIN: "A1", Status: state.StatusInProgress, PercentComplete: 10, HardcoverBookID: int64Ptr(777)}

	tests := []struct {
		name      string
		item      audible.LibraryItem
		existing  hardcover.UserBook
		prior     *state.BookState
		wantReads int
	}{
		{"restart after Audible finish, fresh state", relistenItem(2, false, restart), readBook("2026-06-27"), nil, 1},
		{"restart after Audible finish, known book", relistenItem(2, false, restart),
			readBook("2026-06-27"), &state.BookState{ASIN: "A1", Status: state.StatusFinished, PercentComplete: 100, HardcoverBookID: int64Ptr(777)}, 1},
		{"jump back mid-book after sleeping past the end", relistenItem(60, false, restart), readBook("2026-06-27"), nil, 0},
		{"Audible timestamp older than Hardcover finish", relistenItem(2, false, ptrTime("2026-01-01T00:00:00Z")), readBook("2026-06-27"), nil, 0},
		{"tail replay near the end stays finished", relistenItem(99.5, false, restart), readBook("2026-06-27"), nil, 0},
		{"first listen after print read: percent went up", relistenItem(15, false, nil), readBook("2026-06-27"), baseline, 1},
		{"finished on Kobo, Audible not moving", relistenItem(10, false, nil), readBook("2026-06-27"), baseline, 0},
		{"no baseline yet on fresh state", relistenItem(15, false, nil), readBook("2026-06-27"), nil, 0},
		{"currently reading on Hardcover is never a re-listen", relistenItem(2, false, restart),
			hardcover.UserBook{ID: 200, BookID: 777, StatusID: hardcover.StatusCurrentlyReading, LastReadDate: ptrTime("2026-06-27T00:00:00Z")}, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fh, store := newRelistenFixture(t, tt.existing, tt.prior)
			runOnce(t, &fakeAudible{items: []audible.LibraryItem{tt.item}}, fh, store)
			if len(fh.reads) != tt.wantReads {
				t.Errorf("reads opened = %d, want %d", len(fh.reads), tt.wantReads)
			}
			if len(fh.inserted) != 0 {
				t.Errorf("user_book inserted for a book already on Hardcover: %v", fh.inserted)
			}
		})
	}
}

func TestRelisten_FullCycleAddsExactlyOneRead(t *testing.T) {
	restart := ptrTime("2026-10-18T05:29:00Z")
	fh, store := newRelistenFixture(t, readBook("2026-06-27"), nil)
	fa := &fakeAudible{items: []audible.LibraryItem{relistenItem(2, false, restart)}}

	runOnce(t, fa, fh, store)
	runOnce(t, fa, fh, store) // same state again: no second read
	if len(fh.reads) != 1 {
		t.Fatalf("reads after two syncs = %d, want 1", len(fh.reads))
	}
	var readID int64
	for id := range fh.reads {
		readID = id
	}
	if got := fh.reads[readID].startedAt; !got.Equal(*restart) {
		t.Errorf("startedAt = %v, want restart time %v", got, *restart)
	}

	fa.items = []audible.LibraryItem{relistenItem(50, false, restart)}
	runOnce(t, fa, fh, store)
	if got := fh.reads[readID].progressSeconds; got != 18000 {
		t.Errorf("progress = %d, want 18000 (50%% of 36000)", got)
	}

	fa.items = []audible.LibraryItem{relistenItem(40, false, restart)}
	runOnce(t, fa, fh, store)
	if got := fh.reads[readID].progressSeconds; got != 18000 {
		t.Errorf("progress after jumping back = %d, want unchanged 18000", got)
	}

	done := ptrTime("2026-11-02T20:00:00Z")
	fa.items = []audible.LibraryItem{relistenItem(100, true, done)}
	runOnce(t, fa, fh, store)
	if r := fh.reads[readID]; r.finishedAt == nil || !r.finishedAt.Equal(*done) {
		t.Errorf("finishedAt = %v, want %v", r.finishedAt, *done)
	}

	// Hardcover now reports the new finish; later syncs leave it alone.
	fh.userBooks[777] = readBook("2026-11-02")
	runOnce(t, fa, fh, store)
	if len(fh.reads) != 1 {
		t.Errorf("reads after finish = %d, want 1", len(fh.reads))
	}
	got, _ := store.Get("A1")
	if got.RelistenReadID != nil {
		t.Errorf("RelistenReadID = %v after finish, want nil", *got.RelistenReadID)
	}
}

func TestRelisten_LostStateAdoptsOpenRead(t *testing.T) {
	restart := ptrTime("2026-10-18T05:29:00Z")
	fh, store := newRelistenFixture(t, readBook("2026-06-27"), nil)
	fa := &fakeAudible{items: []audible.LibraryItem{relistenItem(2, false, restart)}}
	runOnce(t, fa, fh, store)

	// Local state wiped (reset or lost response): Hardcover still has the read.
	store2 := newTestStore(t)
	runOnce(t, fa, fh, store2)
	if len(fh.reads) != 1 {
		t.Errorf("reads = %d, want 1 (open read adopted, not duplicated)", len(fh.reads))
	}
}

func TestIsTailReplay(t *testing.T) {
	stamp := ptrTime("2026-10-18T05:28:00Z")
	tests := []struct {
		name string
		item audible.LibraryItem
		want bool
	}{
		{"99% of 10h, stamped", audible.LibraryItem{PercentComplete: 99, RuntimeSeconds: 36000, FinishedAt: stamp}, false}, // 6 min left
		{"99.5% of 10h, stamped", audible.LibraryItem{PercentComplete: 99.5, RuntimeSeconds: 36000, FinishedAt: stamp}, true},
		{"never finished", audible.LibraryItem{PercentComplete: 99.9, RuntimeSeconds: 36000}, false},
		{"unknown runtime", audible.LibraryItem{PercentComplete: 99.9, FinishedAt: stamp}, false},
		{"already finished", audible.LibraryItem{IsFinished: true, PercentComplete: 100, RuntimeSeconds: 36000, FinishedAt: stamp}, false},
	}
	for _, tt := range tests {
		if got := isTailReplay(tt.item); got != tt.want {
			t.Errorf("%s: isTailReplay = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestRelisten_ReadClosedElsewhereIsDropped(t *testing.T) {
	restart := ptrTime("2026-10-18T05:29:00Z")
	fh, store := newRelistenFixture(t, readBook("2026-06-27"), nil)
	fa := &fakeAudible{items: []audible.LibraryItem{relistenItem(2, false, restart)}}
	runOnce(t, fa, fh, store)

	// CWA finishes the shared open read from an e-reader.
	var readID int64
	for id := range fh.reads {
		readID = id
	}
	kobo := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	fh.reads[readID].finishedAt = &kobo
	fh.userBooks[777] = readBook("2026-10-25")

	fa.items = []audible.LibraryItem{relistenItem(40, false, restart)}
	runOnce(t, fa, fh, store)
	if got := fh.reads[readID]; got.progressSeconds != 720 || !got.finishedAt.Equal(kobo) {
		t.Errorf("read closed elsewhere was changed: %+v", got)
	}
	if len(fh.reads) != 1 {
		t.Errorf("reads = %d, want 1 (no new read: restart is older than the Kobo finish)", len(fh.reads))
	}
	if got, _ := store.Get("A1"); got.RelistenReadID != nil {
		t.Errorf("RelistenReadID = %v, want cleared", *got.RelistenReadID)
	}
}

func TestRun_FinishedWithoutDateClearsHardcoverDate(t *testing.T) {
	tests := []struct {
		name        string
		item        audible.LibraryItem
		wantCleared int
	}{
		{"finished without a date", audible.LibraryItem{ASIN: "A1", ISBN13: relistenISBN, IsFinished: true, PercentComplete: 17}, 1},
		{"finished with a date", audible.LibraryItem{ASIN: "A1", ISBN13: relistenISBN, IsFinished: true, PercentComplete: 100, FinishedAt: ptrTime("2026-06-27T00:00:00Z")}, 0},
		{"in progress", audible.LibraryItem{ASIN: "A1", ISBN13: relistenISBN, PercentComplete: 17}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fh := newFakeHardcover()
			fh.byISBN[relistenISBN] = 777
			runOnce(t, &fakeAudible{items: []audible.LibraryItem{tt.item}}, fh, newTestStore(t))
			if len(fh.clearedDates) != tt.wantCleared {
				t.Errorf("ClearFinishDate calls = %v, want %d", fh.clearedDates, tt.wantCleared)
			}
		})
	}
}
