package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kbuechl/audible-hardcover-sync/internal/state"
)

type reviewNotifier struct {
	counts []int
	err    error
}

func (r *reviewNotifier) NotifyUnmatched(context.Context, []state.BookState) error { return nil }
func (r *reviewNotifier) NotifyReview(_ context.Context, count int) error {
	if r.err != nil {
		return r.err
	}
	r.counts = append(r.counts, count)
	return nil
}

func seedWaiting(t *testing.T, store *state.Store, asins ...string) {
	t.Helper()
	for _, a := range asins {
		if err := store.SeeReviewBook(state.ReviewBook{ASIN: a, Title: a}); err != nil {
			t.Fatalf("SeeReviewBook: %v", err)
		}
	}
}

func TestNotifyWaiting(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

	t.Run("after_sync sends once per batch", func(t *testing.T) {
		store, n := newTestStore(t), &reviewNotifier{}
		seedWaiting(t, store, "A1", "A2", "A3", "A4")
		for i := 0; i < 2; i++ {
			if err := NotifyWaiting(ctx, store, n, ModeConfirmAll, "after_sync", now); err != nil {
				t.Fatal(err)
			}
		}
		if len(n.counts) != 1 || n.counts[0] != 4 {
			t.Errorf("counts = %v, want [4]", n.counts)
		}
	})

	t.Run("off never sends", func(t *testing.T) {
		store, n := newTestStore(t), &reviewNotifier{}
		seedWaiting(t, store, "A1")
		if err := NotifyWaiting(ctx, store, n, ModeConfirmAll, "off", now); err != nil || len(n.counts) != 0 {
			t.Errorf("err=%v counts=%v", err, n.counts)
		}
	})

	t.Run("daily within 24h sends nothing", func(t *testing.T) {
		store, n := newTestStore(t), &reviewNotifier{}
		seedWaiting(t, store, "A1")
		if err := store.SetReviewNotifiedAt(now.Add(-23 * time.Hour)); err != nil {
			t.Fatal(err)
		}
		if err := NotifyWaiting(ctx, store, n, ModeConfirmAll, "daily", now); err != nil || len(n.counts) != 0 {
			t.Errorf("err=%v counts=%v", err, n.counts)
		}
		if err := NotifyWaiting(ctx, store, n, ModeConfirmAll, "daily", now.Add(2*time.Hour)); err != nil || len(n.counts) != 1 {
			t.Errorf("after 24h: err=%v counts=%v", err, n.counts)
		}
	})

	t.Run("weekly after quiet days sends only the new books", func(t *testing.T) {
		store, n := newTestStore(t), &reviewNotifier{}
		seedWaiting(t, store, "A1", "A2")
		if err := NotifyWaiting(ctx, store, n, ModeConfirmAll, "weekly", now.Add(-15*24*time.Hour)); err != nil {
			t.Fatal(err)
		}
		seedWaiting(t, store, "B1", "B2", "B3")
		if err := NotifyWaiting(ctx, store, n, ModeConfirmAll, "weekly", now.Add(-24*time.Hour)); err != nil {
			t.Fatal(err)
		}
		if err := NotifyWaiting(ctx, store, n, ModeConfirmAll, "weekly", now); err != nil {
			t.Fatal(err)
		}
		if len(n.counts) != 2 || n.counts[0] != 2 || n.counts[1] != 3 {
			t.Errorf("counts = %v, want [2 3]", n.counts)
		}
	})

	t.Run("send error leaves books unnotified", func(t *testing.T) {
		store, n := newTestStore(t), &reviewNotifier{err: errors.New("boom")}
		seedWaiting(t, store, "A1")
		if err := NotifyWaiting(ctx, store, n, ModeConfirmAll, "after_sync", now); err == nil {
			t.Fatal("want error")
		}
		got, _ := store.UnnotifiedWaiting()
		if len(got) != 1 {
			t.Errorf("unnotified = %d, want 1", len(got))
		}
		if at, _ := store.ReviewNotifiedAt(); !at.IsZero() {
			t.Errorf("ReviewNotifiedAt = %v, want zero", at)
		}
	})

	t.Run("only held books are counted", func(t *testing.T) {
		store, n := newTestStore(t), &reviewNotifier{}
		seedWaiting(t, store, "A1", "A2", "A3")
		if err := store.SetDecision([]string{"A2"}, state.DecisionNotWanted); err != nil {
			t.Fatal(err)
		}
		if err := store.SeeReviewBook(state.ReviewBook{ASIN: "S1", Shared: true}); err != nil {
			t.Fatal(err)
		}
		if err := NotifyWaiting(ctx, store, n, ModeConfirmShared, "after_sync", now); err != nil {
			t.Fatal(err)
		}
		if len(n.counts) != 1 || n.counts[0] != 1 {
			t.Errorf("counts = %v, want [1] (shared only)", n.counts)
		}
	})
}
