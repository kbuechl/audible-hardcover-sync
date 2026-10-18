// Package audible is a thin wrapper around the vendored
// github.com/mstrhakr/go-audible SDK, decoupling this project's data model
// from the SDK's wire format.
package audible

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	audiblesdk "github.com/mstrhakr/go-audible"
)

// LibraryItem is the subset of Audible library data this project needs,
// decoupled from the vendored SDK's wire-format Book type.
type LibraryItem struct {
	ASIN            string
	Title           string
	Author          string
	ISBN10          string
	ISBN13          string
	IsFinished      bool
	PercentComplete float64
	// FinishedAt is the timestamp when the user finished listening to
	// the book, from Audible's listening_status.finished_at_timestamp.
	// Nil if the book isn't finished or the timestamp is absent/unparseable.
	FinishedAt *time.Time
	// Rating is the user's personal star rating (1-5) from Audible's
	// provided_review.ratings.overall_rating. Nil if the user hasn't
	// rated the book on Audible.
	Rating *int
	// CoverURL is Audible's cover art (500px, falling back to 1024px).
	// Empty if Audible returned no product images.
	CoverURL string
}

// responseGroups extends the SDK's default set (WithResponseGroups replaces
// rather than merges, so the defaults must be repeated here explicitly).
var responseGroups = slices.Concat(audiblesdk.DefaultResponseGroups, []string{
	"listening_status", "percent_complete", "is_finished", "provided_review",
})

type Client struct {
	sdk *audiblesdk.Client
}

func New(marketplace audiblesdk.Marketplace) *Client {
	return &Client{sdk: audiblesdk.NewClient(marketplace)}
}

func (c *Client) IsAuthenticated() bool {
	return c.sdk.IsAuthenticated()
}

func (c *Client) GetAuthURL() (*audiblesdk.AuthURL, error) {
	return c.sdk.GetAuthURL()
}

func (c *Client) Authenticate(ctx context.Context, req audiblesdk.DeviceRegistrationRequest) error {
	return c.sdk.Authenticate(ctx, req)
}

// LoadCredentialsFile reads and applies credentials previously written by
// SaveCredentialsFile. The SDK's own LoadCredentials/SaveCredentials methods
// are unimplemented placeholders (see its README), so this project persists
// credentials itself via Marshal/UnmarshalCredentials.
func (c *Client) LoadCredentialsFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read credentials file: %w", err)
	}
	if err := c.sdk.UnmarshalCredentials(data); err != nil {
		return fmt.Errorf("unmarshal credentials: %w", err)
	}
	return nil
}

func (c *Client) SaveCredentialsFile(path string) error {
	data, err := c.sdk.MarshalCredentials()
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write credentials file: %w", err)
	}
	return nil
}

func (c *Client) FetchLibrary(ctx context.Context) ([]LibraryItem, error) {
	books, err := c.sdk.GetAllLibrary(ctx, audiblesdk.WithResponseGroups(responseGroups...))
	if err != nil {
		return nil, fmt.Errorf("fetch audible library: %w", err)
	}

	items := make([]LibraryItem, 0, len(books))
	for _, b := range books {
		items = append(items, toLibraryItem(b))
	}
	return items, nil
}

func toLibraryItem(b audiblesdk.Book) LibraryItem {
	author := ""
	if len(b.Authors) > 0 {
		author = b.Authors[0].Name
	}
	var rating *int
	if b.ProvidedReview != nil && b.ProvidedReview.Ratings != nil {
		rating = b.ProvidedReview.Ratings.OverallRating
	}
	return LibraryItem{
		ASIN:            b.ASIN,
		Title:           b.Title,
		Author:          author,
		ISBN10:          b.ISBN10,
		ISBN13:          b.ISBN13,
		IsFinished:      b.IsFinished,
		PercentComplete: b.PercentComplete,
		FinishedAt:      parseAudibleTimestamp(b.ListeningStatus.FinishedAtTimestamp),
		Rating:          rating,
		CoverURL:        cmp.Or(b.ProductImages.Image500, b.ProductImages.Image1024),
	}
}

// parseAudibleTimestamp parses an ISO 8601 timestamp from Audible's API
// (e.g. "2025-10-11T19:34:29.561Z"). Returns nil if the string is empty
// or unparseable — the caller treats nil as "no date available".
func parseAudibleTimestamp(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		// Audible timestamps may have millisecond precision with a 'Z'
		// suffix which RFC3339 handles, but try a couple of fallbacks
		// just in case.
		t, err = time.Parse("2006-01-02T15:04:05.000000Z", s)
		if err != nil {
			return nil
		}
	}
	return &t
}
