package sync

import (
	"context"
	"fmt"
	"time"

	"github.com/kbuechl/audible-hardcover-sync/internal/state"
)

// Review message frequencies (Preferences: "Review messages").
const (
	notifyOff    = "off"
	notifyDaily  = "daily"
	notifyWeekly = "weekly"
)

// NotifyWaiting sends one message for the books newly waiting for review,
// then marks them told so each book is messaged about once ever. daily and
// weekly stay quiet until that long after the last message. A send error
// leaves the books unnotified so the next sync retries.
func NotifyWaiting(ctx context.Context, store *state.Store, n Notifier, mode, freq string, now time.Time) error {
	var gap time.Duration
	switch freq {
	case notifyOff:
		return nil
	case notifyDaily:
		gap = 24 * time.Hour
	case notifyWeekly:
		gap = 7 * 24 * time.Hour
	}
	if gap > 0 {
		last, err := store.ReviewNotifiedAt()
		if err != nil {
			return fmt.Errorf("read last review message time: %w", err)
		}
		if now.Sub(last) < gap {
			return nil
		}
	}
	books, err := store.UnnotifiedWaiting()
	if err != nil {
		return fmt.Errorf("list books waiting for review: %w", err)
	}
	var asins []string
	for _, b := range books {
		if Held(mode, b.Shared, b.Decision) {
			asins = append(asins, b.ASIN)
		}
	}
	if len(asins) == 0 {
		return nil
	}
	if err := n.NotifyReview(ctx, len(asins)); err != nil {
		return fmt.Errorf("send review message: %w", err)
	}
	if err := store.MarkReviewNotified(asins); err != nil {
		return fmt.Errorf("mark books notified: %w", err)
	}
	if err := store.SetReviewNotifiedAt(now); err != nil {
		return fmt.Errorf("record review message time: %w", err)
	}
	return nil
}
