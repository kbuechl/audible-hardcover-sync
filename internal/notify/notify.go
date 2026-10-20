// Package notify sends unmatched-book alerts to external notification
// services (Slack, Discord, email, etc.) via shoutrrr.
package notify

import (
	"context"
	"fmt"
	"strings"

	"github.com/containrrr/shoutrrr"

	"github.com/kbuechl/audible-hardcover-sync/internal/state"
)

// Notifier delivers unmatched-book notifications via shoutrrr-configured
// service URLs. It implements sync.Notifier.
type Notifier struct {
	urls      []string
	publicURL string
}

// New returns a Notifier that sends to the given shoutrrr service URLs and
// links back to publicURL in messages.
func New(urls []string, publicURL string) *Notifier {
	return &Notifier{urls: urls, publicURL: publicURL}
}

// NotifyUnmatched sends a single summary notification for items that could
// not be matched to a Hardcover book. It is a no-op when no notification
// URLs are configured or when there are no items to report.
func (n *Notifier) NotifyUnmatched(ctx context.Context, items []state.BookState) error {
	if len(n.urls) == 0 || len(items) == 0 {
		return nil
	}

	sender, err := shoutrrr.CreateSender(n.urls...)
	if err != nil {
		return fmt.Errorf("create notification sender: %w", err)
	}

	msg := buildMessage(items, n.publicURL)
	for _, sendErr := range sender.Send(msg, nil) {
		if sendErr != nil {
			return fmt.Errorf("send notification: %w", sendErr)
		}
	}
	return nil
}

func buildMessage(items []state.BookState, publicURL string) string {
	msg := fmt.Sprintf("%d book(s) need manual matching on Hardcover", len(items))
	if publicURL == "" {
		return msg + ". Open the web UI to match them."
	}
	// The match page is the web UI's root.
	return msg + ": " + strings.TrimRight(publicURL, "/") + "/"
}
