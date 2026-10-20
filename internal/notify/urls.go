package notify

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/containrrr/shoutrrr"
)

const testMessage = "Test notification from audible-hardcover-sync. Notifications are working."

// Validate reports whether rawURL is a notification URL shoutrrr can send to.
func Validate(rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return errors.New("URL is empty")
	}
	if _, err := shoutrrr.CreateSender(rawURL); err != nil {
		return fmt.Errorf("not a valid notification URL: %w", err)
	}
	return nil
}

// SendTest sends a test message to rawURL and returns the service's error.
func SendTest(rawURL string) error {
	sender, err := shoutrrr.CreateSender(rawURL)
	if err != nil {
		return fmt.Errorf("not a valid notification URL: %w", err)
	}
	for _, sendErr := range sender.Send(testMessage, nil) {
		if sendErr != nil {
			return sendErr
		}
	}
	return nil
}

// Mask hides the secret parts of a notification URL (credentials, path
// segments and query values) for display, keeping enough to recognise it,
// e.g. "discord://****@1234567890" or "ntfy://ntfy.sh/bo****".
func Mask(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" {
		return "****"
	}
	var b strings.Builder
	b.WriteString(u.Scheme + "://")
	if u.User != nil {
		b.WriteString("****@")
	}
	b.WriteString(u.Host)
	for _, seg := range strings.Split(strings.Trim(u.Path, "/"), "/") {
		if seg != "" {
			b.WriteString("/" + maskSegment(seg))
		}
	}
	if q := u.Query(); len(q) > 0 {
		keys := make([]string, 0, len(q))
		for k := range q {
			keys = append(keys, k+"=****")
		}
		b.WriteString("?" + strings.Join(keys, "&"))
	}
	return b.String()
}

func maskSegment(seg string) string {
	if len(seg) <= 4 {
		return "****"
	}
	return seg[:2] + "****"
}
