// Package hardcover provides a minimal GraphQL client for the Hardcover
// API (api.hardcover.app/v1/graphql), covering authentication, ISBN-based
// book lookup, and upserting a user's reading status for a book.
package hardcover

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

const (
	StatusWantToRead       = 1
	StatusCurrentlyReading = 2
	StatusRead             = 3
)

const defaultEndpoint = "https://api.hardcover.app/v1/graphql"

// Client is not safe for concurrent use by multiple goroutines. In
// particular, userIDCached lazily populates the userID field with no
// synchronization, so sharing a single *Client across goroutines can race.
type Client struct {
	token      string
	endpoint   string
	httpClient *http.Client

	userID int64 // resolved lazily via Me(), cached for UpsertUserBook
}

func New(token string) *Client {
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 4
	retryClient.RetryWaitMin = 1 * time.Second
	retryClient.RetryWaitMax = 30 * time.Second
	retryClient.CheckRetry = retryPolicy
	retryClient.Backoff = retryablehttp.DefaultBackoff // exponential, respects Retry-After on 429

	return &Client{
		token:      token,
		endpoint:   defaultEndpoint,
		httpClient: retryClient.StandardClient(),
	}
}

// retryPolicy retries on network errors, HTTP 429 (Throttled — the
// Hardcover API's rate-limit signal), and 5xx server errors (except 501,
// which is a permanent "not implemented" and not worth retrying). It
// deliberately does NOT retry 4xx client errors like 401 (bad token) or
// 404 (not found) — those are permanent and retrying just wastes time.
//
// Mutations (marked via the X-GraphQL-Operation: mutation header set in
// do()) are NOT retried on 429 or 5xx. A 429 or 5xx response does NOT
// guarantee the server failed to process the request — a rate-limiter
// proxy may return 429 after the upstream already committed the mutation.
// Retrying a non-idempotent INSERT like insert_user_book in that case
// creates duplicate read entries on the user's Hardcover library. Network
// errors (err != nil, resp == nil) ARE still retried for mutations,
// since a transport-level failure means the request almost certainly
// never reached the server.
func retryPolicy(ctx context.Context, resp *http.Response, err error) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if err != nil {
		return true, nil // network/transport errors are retryable (request likely never reached server)
	}
	isMutation := resp.Request != nil && resp.Request.Header.Get("X-GraphQL-Operation") == "mutation"
	if isMutation {
		// Mutations are not idempotent — don't retry on HTTP status
		// codes, because the server may have already processed the
		// mutation before returning 429/5xx.
		return false, nil
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return true, nil
	}
	if resp.StatusCode >= 500 && resp.StatusCode != http.StatusNotImplemented {
		return true, nil
	}
	return false, nil
}

type gqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type gqlError struct {
	Message string `json:"message"`
}

type gqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []gqlError      `json:"errors,omitempty"`
}

func (c *Client) do(ctx context.Context, query string, variables map[string]any, out any) error {
	body, err := json.Marshal(gqlRequest{Query: query, Variables: variables})
	if err != nil {
		return fmt.Errorf("marshal graphql request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build graphql request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("authorization", c.token)
	req.Header.Set("user-agent", "audible-hardcover-sync (github.com/kbuechl/audible-hardcover-sync)")
	// Mark mutations so retryPolicy can avoid retrying non-idempotent
	// operations on 429/5xx (the server may have already committed the
	// mutation before returning an error status).
	if strings.HasPrefix(strings.TrimSpace(query), "mutation") {
		req.Header.Set("X-GraphQL-Operation", "mutation")
	} else {
		req.Header.Set("X-GraphQL-Operation", "query")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("graphql request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read graphql response body (status %d): %w", resp.StatusCode, err)
	}

	// The GraphQL-spec "errors" array is not the only failure shape Hardcover
	// uses: e.g. an auth failure is documented to return HTTP 401 with a
	// bare {"error": "..."} body (no "errors" array). Checking the HTTP
	// status here, before relying on gr.Errors or a successful unmarshal
	// into out, ensures a non-2xx response is never mistaken for success —
	// which matters most for callers (like UpsertUserBook's mutations) that
	// pass out == nil and would otherwise have no other failure signal.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("graphql request failed: status %d: %s", resp.StatusCode, string(respBody))
	}

	var gr gqlResponse
	if err := json.Unmarshal(respBody, &gr); err != nil {
		return fmt.Errorf("decode graphql response (status %d): %w", resp.StatusCode, err)
	}
	if len(gr.Errors) > 0 {
		return fmt.Errorf("graphql error: %s", gr.Errors[0].Message)
	}
	if out != nil {
		if err := json.Unmarshal(gr.Data, out); err != nil {
			return fmt.Errorf("unmarshal graphql data: %w", err)
		}
	}
	return nil
}

func (c *Client) Me(ctx context.Context) (userID int64, username string, err error) {
	var out struct {
		Me []struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"me"`
	}
	if err := c.do(ctx, `query { me { id username } }`, nil, &out); err != nil {
		return 0, "", err
	}
	if len(out.Me) == 0 {
		return 0, "", fmt.Errorf("hardcover: me query returned no user")
	}
	return out.Me[0].ID, out.Me[0].Username, nil
}

func (c *Client) userIDCached(ctx context.Context) (int64, error) {
	if c.userID != 0 {
		return c.userID, nil
	}
	id, _, err := c.Me(ctx)
	if err != nil {
		return 0, err
	}
	c.userID = id
	return id, nil
}

// FindBookByISBN resolves a Hardcover book_id from an exact ISBN-10 or
// ISBN-13 match against the editions table (not the fuzzy search endpoint —
// this is the deterministic path used by the automated sync).
func (c *Client) FindBookByISBN(ctx context.Context, isbn string) (bookID int64, found bool, err error) {
	var out struct {
		Editions []struct {
			BookID int64 `json:"book_id"`
		} `json:"editions"`
	}
	query := `query FindByISBN($isbn: String!) {
		editions(where: {_or: [{isbn_13: {_eq: $isbn}}, {isbn_10: {_eq: $isbn}}]}, limit: 1) {
			book_id
		}
	}`
	if err := c.do(ctx, query, map[string]any{"isbn": isbn}, &out); err != nil {
		return 0, false, err
	}
	if len(out.Editions) == 0 {
		return 0, false, nil
	}
	return out.Editions[0].BookID, true, nil
}

type SearchResult struct {
	ID          int64
	Title       string
	AuthorNames []string
	ISBNs       []string
	ReleaseYear int
	Slug        string
}

// SearchBooks does a fuzzy Typesense-backed search. It backs both the
// manual-match web UI and, as a confidence-scored fallback, the automated
// sync path (internal/sync.resolveByFuzzyMatch) when the deterministic
// FindBookByISBN produces no match.
func (c *Client) SearchBooks(ctx context.Context, query string, perPage int) ([]SearchResult, error) {
	var out struct {
		Search struct {
			Results struct {
				Hits []struct {
					Document struct {
						ID          json.Number `json:"id"`
						Title       string      `json:"title"`
						AuthorNames []string    `json:"author_names"`
						ISBNs       []string    `json:"isbns"`
						ReleaseYear int         `json:"release_year"`
						Slug        string      `json:"slug"`
					} `json:"document"`
				} `json:"hits"`
			} `json:"results"`
		} `json:"search"`
	}

	gqlQuery := `query SearchBooks($query: String!, $perPage: Int!) {
		search(query: $query, query_type: "Book", per_page: $perPage) {
			results
		}
	}`
	if err := c.do(ctx, gqlQuery, map[string]any{"query": query, "perPage": perPage}, &out); err != nil {
		return nil, fmt.Errorf("search books: %w", err)
	}

	results := make([]SearchResult, 0, len(out.Search.Results.Hits))
	for _, hit := range out.Search.Results.Hits {
		id, err := hit.Document.ID.Int64()
		if err != nil {
			continue // skip malformed entries rather than failing the whole search
		}
		results = append(results, SearchResult{
			ID:          id,
			Title:       hit.Document.Title,
			AuthorNames: hit.Document.AuthorNames,
			ISBNs:       hit.Document.ISBNs,
			ReleaseYear: hit.Document.ReleaseYear,
			Slug:        hit.Document.Slug,
		})
	}
	return results, nil
}

// FetchCoverURLs looks up cover image URLs for a batch of Hardcover book
// IDs. The search endpoint doesn't return an image — this is the documented
// follow-up query (books.image.url) needed to get one. Books with no cover
// image are simply absent from the returned map, never a zero-value entry.
func (c *Client) FetchCoverURLs(ctx context.Context, bookIDs []int64) (map[int64]string, error) {
	if len(bookIDs) == 0 {
		return map[int64]string{}, nil
	}

	var out struct {
		Books []struct {
			ID    int64 `json:"id"`
			Image *struct {
				URL string `json:"url"`
			} `json:"image"`
		} `json:"books"`
	}

	gqlQuery := `query FetchCovers($ids: [Int!]!) {
		books(where: {id: {_in: $ids}}) {
			id
			image { url }
		}
	}`
	if err := c.do(ctx, gqlQuery, map[string]any{"ids": bookIDs}, &out); err != nil {
		return nil, fmt.Errorf("fetch cover urls: %w", err)
	}

	urls := make(map[int64]string, len(out.Books))
	for _, b := range out.Books {
		if b.Image != nil && b.Image.URL != "" {
			urls[b.ID] = b.Image.URL
		}
	}
	return urls, nil
}

// UserBook is a single entry in the user's Hardcover library, as
// returned by FetchUserBooks. ID is the user_books row ID (needed for
// updates); BookID is the books table ID (used as the cache key).
type UserBook struct {
	ID       int64
	BookID   int64
	StatusID int
	Rating   *float64
}

// FetchUserBooks fetches all of the authenticated user's user_books rows
// in a single query and returns them as a map keyed by book_id. This is
// called once at the start of a sync pass so the orchestrator can check
// existing status in memory — avoiding a per-book FindUserBook query
// (and its rate-limit cost) for every item in the library.
func (c *Client) FetchUserBooks(ctx context.Context) (map[int64]UserBook, error) {
	userID, err := c.userIDCached(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve hardcover user id: %w", err)
	}

	var out struct {
		UserBooks []struct {
			ID       int64        `json:"id"`
			BookID   int64        `json:"book_id"`
			StatusID int          `json:"status_id"`
			Rating   *float64     `json:"rating"`
		} `json:"user_books"`
	}
	gqlQuery := `query FetchUserBooks($userID: Int!) {
		user_books(where: {user_id: {_eq: $userID}}) {
			id
			book_id
			status_id
			rating
		}
	}`
	if err := c.do(ctx, gqlQuery, map[string]any{"userID": userID}, &out); err != nil {
		return nil, fmt.Errorf("fetch user books: %w", err)
	}

	books := make(map[int64]UserBook, len(out.UserBooks))
	for _, ub := range out.UserBooks {
		books[ub.BookID] = UserBook{
			ID:       ub.ID,
			BookID:   ub.BookID,
			StatusID: ub.StatusID,
			Rating:   ub.Rating,
		}
	}
	return books, nil
}

// InsertUserBook adds a new book to the user's Hardcover library with
// the given status (and optional rating). It does NOT check for an
// existing row first — the caller is responsible for using
// FetchUserBooks to decide whether to insert or update.
//
// The mutation shape (argument types) was confirmed via live GraphQL
// introspection against Hardcover's real API on 2026-08-01.
func (c *Client) InsertUserBook(ctx context.Context, bookID int64, statusID int, rating *float64) error {
	insertQuery := `mutation InsertUserBook($object: UserBookCreateInput!) {
		insert_user_book(object: $object) { id }
	}`
	object := map[string]any{"book_id": bookID, "status_id": statusID}
	if rating != nil {
		object["rating"] = *rating
	}
	if err := c.do(ctx, insertQuery, map[string]any{"object": object}, nil); err != nil {
		return fmt.Errorf("insert user_book: %w", err)
	}
	return nil
}

// UpdateUserBook updates an existing user_books row (identified by its
// row ID from FetchUserBooks) with a new status (and optional rating).
// It does NOT check for an existing row first — the caller supplies the
// row ID from the FetchUserBooks cache.
//
// The mutation shape (argument types and the update mutation's name/args)
// was confirmed via live GraphQL introspection against Hardcover's real
// API on 2026-08-01.
func (c *Client) UpdateUserBook(ctx context.Context, rowID int64, statusID int, rating *float64) error {
	updateQuery := `mutation UpdateUserBook($id: Int!, $object: UserBookUpdateInput!) {
		update_user_book(id: $id, object: $object) { id }
	}`
	object := map[string]any{"status_id": statusID}
	if rating != nil {
		object["rating"] = *rating
	}
	if err := c.do(ctx, updateQuery, map[string]any{"id": rowID, "object": object}, nil); err != nil {
		return fmt.Errorf("update user_book %d: %w", rowID, err)
	}
	return nil
}
