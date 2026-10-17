package hardcover

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

func newTestServer(t *testing.T, handler func(query string, vars map[string]any) (any, error)) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("authorization") != "test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{"error": "Unable to verify token"})
			return
		}
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		data, err := handler(req.Query, req.Variables)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]any{"errors": []map[string]any{{"message": err.Error()}}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(srv.Close)

	c := New("test-token")
	c.endpoint = srv.URL
	c.httpClient = srv.Client()
	c.limiter = newLimiter(0)
	return srv, c
}

func TestClient_Me(t *testing.T) {
	_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
		return map[string]any{"me": []map[string]any{{"id": 42, "username": "reader"}}}, nil
	})

	id, username, err := c.Me(context.Background())
	if err != nil {
		t.Fatalf("Me() error = %v", err)
	}
	if id != 42 || username != "reader" {
		t.Errorf("Me() = (%d, %q), want (42, %q)", id, username, "reader")
	}
}

func TestClient_FindBookByISBN_Found(t *testing.T) {
	_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
		return map[string]any{"editions": []map[string]any{
			{"book_id": 777, "book": map[string]any{"default_audio_edition_id": 999}},
		}}, nil
	})

	match, found, err := c.FindBookByISBN(context.Background(), "9780641723445", "")
	if err != nil {
		t.Fatalf("FindBookByISBN() error = %v", err)
	}
	if !found || match.BookID != 777 {
		t.Errorf("FindBookByISBN() = (%+v, %v), want BookID 777", match, found)
	}
	if match.EditionID == nil || *match.EditionID != 999 {
		t.Errorf("FindBookByISBN() EditionID = %v, want 999 (book.default_audio_edition_id)", match.EditionID)
	}
}

func TestClient_FindBookByISBN_NoDefaultEdition(t *testing.T) {
	// A matched book whose nested book object has no default editions set
	// must yield a nil EditionID — the caller then falls back to no edition.
	_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
		return map[string]any{"editions": []map[string]any{
			{"book_id": 777, "book": map[string]any{
				"default_audio_edition_id":    nil,
				"default_ebook_edition_id":    nil,
				"default_physical_edition_id": nil,
			}},
		}}, nil
	})

	match, found, err := c.FindBookByISBN(context.Background(), "9780641723445", "")
	if err != nil {
		t.Fatalf("FindBookByISBN() error = %v", err)
	}
	if !found || match.BookID != 777 {
		t.Errorf("FindBookByISBN() = (%+v, %v), want BookID 777", match, found)
	}
	if match.EditionID != nil {
		t.Errorf("FindBookByISBN() EditionID = %v, want nil when no default editions are set", match.EditionID)
	}
}

func TestClient_FindBookByISBN_NotFound(t *testing.T) {
	_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
		return map[string]any{"editions": []map[string]any{}}, nil
	})

	_, found, err := c.FindBookByISBN(context.Background(), "0000000000", "")
	if err != nil {
		t.Fatalf("FindBookByISBN() error = %v", err)
	}
	if found {
		t.Error("found = true, want false for unknown ISBN")
	}
}

func TestClient_FetchUserBooks(t *testing.T) {
	_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
		switch {
		case strings.Contains(query, "me"):
			return map[string]any{"me": []map[string]any{{"id": 42, "username": "reader"}}}, nil
		case strings.Contains(query, "user_books("):
			return map[string]any{"user_books": []map[string]any{
				{"id": 100, "book_id": float64(777), "status_id": float64(StatusRead)},
				{"id": 200, "book_id": float64(888), "status_id": float64(StatusCurrentlyReading)},
			}}, nil
		default:
			t.Fatalf("unexpected query: %s", query)
			return nil, nil
		}
	})

	books, err := c.FetchUserBooks(context.Background())
	if err != nil {
		t.Fatalf("FetchUserBooks() error = %v", err)
	}
	if len(books) != 2 {
		t.Fatalf("len(books) = %d, want 2", len(books))
	}
	ub777, ok := books[777]
	if !ok {
		t.Fatalf("books[777] not found")
	}
	if ub777.ID != 100 || ub777.StatusID != StatusRead {
		t.Errorf("books[777] = %+v, want ID=100 StatusID=%d", ub777, StatusRead)
	}
	ub888, ok := books[888]
	if !ok {
		t.Fatalf("books[888] not found")
	}
	if ub888.ID != 200 || ub888.StatusID != StatusCurrentlyReading {
		t.Errorf("books[888] = %+v, want ID=200 StatusID=%d", ub888, StatusCurrentlyReading)
	}
}

func TestClient_InsertUserBook(t *testing.T) {
	var insertCalled bool
	_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
		switch {
		case strings.Contains(query, "insert_user_book"):
			insertCalled = true
			object, ok := vars["object"].(map[string]any)
			if !ok {
				t.Fatalf("insert called with object = %v, want a map", vars["object"])
			}
			if object["book_id"] != float64(777) {
				t.Errorf("insert object[book_id] = %v, want 777", object["book_id"])
			}
			if object["status_id"] != float64(StatusRead) {
				t.Errorf("insert object[status_id] = %v, want %d", object["status_id"], StatusRead)
			}
			return map[string]any{"insert_user_book": map[string]any{"id": 1}}, nil
		default:
			t.Fatalf("unexpected query: %s", query)
			return nil, nil
		}
	})

	if err := c.InsertUserBook(context.Background(), 777, StatusRead, nil, nil, nil); err != nil {
		t.Fatalf("InsertUserBook() error = %v", err)
	}
	if !insertCalled {
		t.Error("expected insert_user_book to be called")
	}
}

// TestClient_Me_Table covers success and failure/edge cases for Me() beyond
// the baseline TestClient_Me above: a GraphQL-level error, and a
// well-formed-but-empty "me" array (an authenticated request that
// unexpectedly resolves to no user).
func TestClient_Me_Table(t *testing.T) {
	tests := []struct {
		name    string
		reply   func() (any, error)
		wantID  int64
		wantErr bool
	}{
		{
			name: "success",
			reply: func() (any, error) {
				return map[string]any{"me": []map[string]any{{"id": 7, "username": "reader"}}}, nil
			},
			wantID: 7,
		},
		{
			name: "graphql error",
			reply: func() (any, error) {
				return nil, errors.New("Unable to verify token")
			},
			wantErr: true,
		},
		{
			name: "empty me array",
			reply: func() (any, error) {
				return map[string]any{"me": []map[string]any{}}, nil
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
				return tt.reply()
			})

			id, _, err := c.Me(context.Background())
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Me() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Me() error = %v, want nil", err)
			}
			if id != tt.wantID {
				t.Errorf("Me() id = %d, want %d", id, tt.wantID)
			}
		})
	}
}

// TestClient_FindBookByISBN_Error verifies a GraphQL-level error from the
// editions query is propagated to the caller (the failure case alongside
// the found/not-found edge cases above).
func TestClient_FindBookByISBN_Error(t *testing.T) {
	_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
		return nil, errors.New("editions query failed")
	})

	_, found, err := c.FindBookByISBN(context.Background(), "9780641723445", "")
	if err == nil {
		t.Fatal("FindBookByISBN() error = nil, want error")
	}
	if found {
		t.Error("found = true, want false on error")
	}
}

// TestClient_FindBookByISBN_EbookFallback verifies the edition-selection
// fallback chain inside FindBookByISBN: when the book has no
// default_audio_edition_id, default_ebook_edition_id is used instead.
func TestClient_FindBookByISBN_EbookFallback(t *testing.T) {
	_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
		return map[string]any{"editions": []map[string]any{
			{"book_id": 777, "book": map[string]any{
				"default_audio_edition_id":    nil,
				"default_ebook_edition_id":    555,
				"default_physical_edition_id": 444,
			}},
		}}, nil
	})

	match, found, err := c.FindBookByISBN(context.Background(), "9780641723445", "")
	if err != nil {
		t.Fatalf("FindBookByISBN() error = %v", err)
	}
	if !found || match.BookID != 777 {
		t.Errorf("FindBookByISBN() = (%+v, %v), want BookID 777", match, found)
	}
	if match.EditionID == nil || *match.EditionID != 555 {
		t.Errorf("FindBookByISBN() EditionID = %v, want 555 (default_ebook_edition_id fallback)", match.EditionID)
	}
}

// TestPickEditionID verifies pickEditionID's preference order directly:
// audio → ebook → physical → nil.
func TestPickEditionID(t *testing.T) {
	audio := int64(1)
	ebook := int64(2)
	physical := int64(3)
	tests := []struct {
		name     string
		audio    *int64
		ebook    *int64
		physical *int64
		want     *int64
	}{
		{"prefers audio over ebook and physical", &audio, &ebook, &physical, &audio},
		{"falls back to ebook when no audio", nil, &ebook, &physical, &ebook},
		{"falls back to physical when only physical is set", nil, nil, &physical, &physical},
		{"nil when no default editions set", nil, nil, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pickEditionID(tt.audio, tt.ebook, tt.physical)
			if tt.want == nil {
				if got != nil {
					t.Errorf("pickEditionID() = %v, want nil", got)
				}
				return
			}
			if got == nil || *got != *tt.want {
				t.Errorf("pickEditionID() = %v, want %d", got, *tt.want)
			}
		})
	}
}

// TestClient_FetchUserBooks_Error verifies that a failure resolving the
// user ID (via Me) propagates as an error from FetchUserBooks.
func TestClient_InsertUserBook_WithUserDate(t *testing.T) {
	var capturedObject map[string]any
	_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
		if strings.Contains(query, "insert_user_book") {
			capturedObject = vars["object"].(map[string]any)
			return map[string]any{"insert_user_book": map[string]any{"id": 1}}, nil
		}
		t.Fatalf("unexpected query: %s", query)
		return nil, nil
	})

	finishedAt := time.Date(2025, 10, 11, 19, 34, 29, 0, time.UTC)
	if err := c.InsertUserBook(context.Background(), 777, StatusRead, nil, &finishedAt, nil); err != nil {
		t.Fatalf("InsertUserBook() error = %v", err)
	}
	if got := capturedObject["user_date"]; got != "2025-10-11" {
		t.Errorf("object[user_date] = %v, want %q", got, "2025-10-11")
	}
}

func TestClient_InsertUserBook_WithoutUserDate(t *testing.T) {
	var capturedObject map[string]any
	_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
		if strings.Contains(query, "insert_user_book") {
			capturedObject = vars["object"].(map[string]any)
			return map[string]any{"insert_user_book": map[string]any{"id": 1}}, nil
		}
		t.Fatalf("unexpected query: %s", query)
		return nil, nil
	})

	if err := c.InsertUserBook(context.Background(), 777, StatusCurrentlyReading, nil, nil, nil); err != nil {
		t.Fatalf("InsertUserBook() error = %v", err)
	}
	if _, hasDate := capturedObject["user_date"]; hasDate {
		t.Errorf("object[user_date] = %v, want absent for nil userDate", capturedObject["user_date"])
	}
}

func TestClient_FetchUserBooks_Error(t *testing.T) {
	_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
		if strings.Contains(query, "me") {
			return nil, errors.New("Unable to verify token")
		}
		t.Fatalf("unexpected query after me failure: %s", query)
		return nil, nil
	})

	_, err := c.FetchUserBooks(context.Background())
	if err == nil || !strings.Contains(err.Error(), "resolve hardcover user id") {
		t.Fatalf("FetchUserBooks() error = %v, want containing %q", err, "resolve hardcover user id")
	}
}

// TestClient_do_Unauthorized verifies that a request signed with the wrong
// token (server returns a non-GraphQL-shaped error body) surfaces as an
// error rather than a silent success.
func TestClient_do_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{"error": "Unable to verify token"})
	}))
	t.Cleanup(srv.Close)

	c := New("wrong-token")
	c.endpoint = srv.URL
	c.httpClient = srv.Client()

	if _, _, err := c.Me(context.Background()); err == nil {
		t.Fatal("Me() error = nil, want error for unauthorized response")
	}
}

func TestClient_SearchBooks(t *testing.T) {
	_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
		if !strings.Contains(query, "search(") {
			t.Fatalf("expected a search() query, got: %s", query)
		}
		return map[string]any{
			"search": map[string]any{
				"results": map[string]any{
					"found": 1,
					"hits": []map[string]any{
						{
							"document": map[string]any{
								"id":           "192946",
								"title":        "A Court of Thorns and Roses",
								"author_names": []string{"Sarah J. Maas"},
								"isbns":        []string{"9781635575569"},
							},
						},
					},
				},
			},
		}, nil
	})

	results, err := c.SearchBooks(context.Background(), "court of thorns and roses", 5)
	if err != nil {
		t.Fatalf("SearchBooks() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %v, want 1", results)
	}
	if results[0].ID != 192946 {
		t.Errorf("ID = %d, want 192946", results[0].ID)
	}
	if results[0].Title != "A Court of Thorns and Roses" {
		t.Errorf("Title = %q", results[0].Title)
	}
	if len(results[0].AuthorNames) != 1 || results[0].AuthorNames[0] != "Sarah J. Maas" {
		t.Errorf("AuthorNames = %v", results[0].AuthorNames)
	}
}

// TestClient_SearchBooks_Table covers the success case above plus edge
// cases a reviewer would expect: a GraphQL-level error propagating, a
// malformed hit (non-numeric id) being skipped rather than failing the
// whole search, and an empty hit list producing an empty (non-nil) slice.
func TestClient_SearchBooks_Table(t *testing.T) {
	tests := []struct {
		name      string
		reply     func() (any, error)
		wantLen   int
		wantErr   bool
		wantTitle string
	}{
		{
			name: "success",
			reply: func() (any, error) {
				return map[string]any{
					"search": map[string]any{
						"results": map[string]any{
							"found": 1,
							"hits": []map[string]any{
								{"document": map[string]any{
									"id": "1", "title": "Book One",
									"author_names": []string{"Author One"},
									"isbns":        []string{"111"},
								}},
							},
						},
					},
				}, nil
			},
			wantLen:   1,
			wantTitle: "Book One",
		},
		{
			name: "graphql error",
			reply: func() (any, error) {
				return nil, errors.New("search backend unavailable")
			},
			wantErr: true,
		},
		{
			// "1.5" is a syntactically valid JSON number (so the outer
			// json.Unmarshal into json.Number succeeds), but it is not a
			// whole number, so Document.ID.Int64() fails on it — this
			// exercises the per-hit skip in SearchBooks, as opposed to a
			// document whose "id" isn't valid JSON number syntax at all
			// (that would fail the whole json.Unmarshal, not per-hit).
			name: "malformed id is skipped, not fatal",
			reply: func() (any, error) {
				return map[string]any{
					"search": map[string]any{
						"results": map[string]any{
							"found": 2,
							"hits": []map[string]any{
								{"document": map[string]any{
									"id": "1.5", "title": "Bad Hit",
									"author_names": []string{}, "isbns": []string{},
								}},
								{"document": map[string]any{
									"id": "2", "title": "Good Hit",
									"author_names": []string{}, "isbns": []string{},
								}},
							},
						},
					},
				}, nil
			},
			wantLen:   1,
			wantTitle: "Good Hit",
		},
		{
			name: "no hits returns empty slice",
			reply: func() (any, error) {
				return map[string]any{
					"search": map[string]any{
						"results": map[string]any{
							"found": 0,
							"hits":  []map[string]any{},
						},
					},
				}, nil
			},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
				return tt.reply()
			})

			results, err := c.SearchBooks(context.Background(), "some query", 5)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("SearchBooks() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("SearchBooks() error = %v, want nil", err)
			}
			if results == nil {
				t.Error("results = nil, want non-nil (possibly empty) slice")
			}
			if len(results) != tt.wantLen {
				t.Fatalf("len(results) = %d, want %d", len(results), tt.wantLen)
			}
			if tt.wantLen > 0 && results[0].Title != tt.wantTitle {
				t.Errorf("results[0].Title = %q, want %q", results[0].Title, tt.wantTitle)
			}
		})
	}
}

// TestClient_do_NonGraphQLErrorStatusWithNilOut reproduces the real
// Hardcover auth-failure shape ({"error": "..."} — no "errors" array, HTTP
// 401) via a call path where out == nil, exactly matching how UpsertUserBook
// invokes do() for insert_user_book and update_user_book. Before the
// do() status-code fix this returned nil (a silent success): do() only
// detected failure via a populated GraphQL "errors" array or a failed
// unmarshal into out, and it skips the unmarshal entirely when out is nil,
// so a non-2xx response with a body shaped like {"error": "..."} was
// invisible. This is the scenario that matters for UpsertUserBook, whose
// mutation calls always pass out == nil.
func TestClient_do_NonGraphQLErrorStatusWithNilOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{"error": "Unable to verify token"})
	}))
	t.Cleanup(srv.Close)

	c := New("wrong-token")
	c.endpoint = srv.URL
	c.httpClient = srv.Client()

	err := c.do(context.Background(), `mutation { insert_user_book(object: {}) { id } }`, nil, nil)
	if err == nil {
		t.Fatal("do() error = nil, want error for HTTP 401 response with no GraphQL errors array (out == nil call path, as used by UpsertUserBook's mutations)")
	}
}

func TestClient_SearchBooks_IncludesReleaseYearAndSlug(t *testing.T) {
	_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
		return map[string]any{
			"search": map[string]any{
				"results": map[string]any{
					"hits": []map[string]any{
						{
							"document": map[string]any{
								"id":           "192946",
								"title":        "A Court of Thorns and Roses",
								"author_names": []string{"Sarah J. Maas"},
								"isbns":        []string{"9781635575569"},
								"release_year": 2015,
								"slug":         "a-court-of-thorns-and-roses",
							},
						},
					},
				},
			},
		}, nil
	})

	results, err := c.SearchBooks(context.Background(), "court of thorns", 5)
	if err != nil {
		t.Fatalf("SearchBooks() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %v, want 1", results)
	}
	if results[0].ReleaseYear != 2015 {
		t.Errorf("ReleaseYear = %d, want 2015", results[0].ReleaseYear)
	}
	if results[0].Slug != "a-court-of-thorns-and-roses" {
		t.Errorf("Slug = %q, want %q", results[0].Slug, "a-court-of-thorns-and-roses")
	}
}

func TestClient_FetchCoverURLs(t *testing.T) {
	_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
		if !strings.Contains(query, "books(") {
			t.Fatalf("expected a books() query, got: %s", query)
		}
		return map[string]any{
			"books": []map[string]any{
				{"id": 111, "image": map[string]any{"url": "https://assets.hardcover.app/111.jpg"}},
				{"id": 222, "image": nil},
			},
		}, nil
	})

	urls, err := c.FetchCoverURLs(context.Background(), []int64{111, 222, 333})
	if err != nil {
		t.Fatalf("FetchCoverURLs() error = %v", err)
	}
	if urls[111] != "https://assets.hardcover.app/111.jpg" {
		t.Errorf("urls[111] = %q, want the cover URL", urls[111])
	}
	if _, ok := urls[222]; ok {
		t.Errorf("urls[222] = %q, want no entry for a book with a nil image", urls[222])
	}
	if _, ok := urls[333]; ok {
		t.Errorf("urls[333] should not exist (333 was never in the mocked response)")
	}
}

func TestClient_FetchCoverURLs_EmptyInput(t *testing.T) {
	c := New("test-token")
	urls, err := c.FetchCoverURLs(context.Background(), nil)
	if err != nil {
		t.Fatalf("FetchCoverURLs(nil) error = %v, want nil (no request should be made)", err)
	}
	if len(urls) != 0 {
		t.Errorf("urls = %v, want empty", urls)
	}
}

func TestClient_FetchCoverURLs_Error(t *testing.T) {
	_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
		return nil, fmt.Errorf("hardcover down")
	})

	_, err := c.FetchCoverURLs(context.Background(), []int64{111})
	if err == nil {
		t.Fatal("FetchCoverURLs() error = nil, want error propagated from do()")
	}
}

// newRetryTestServer creates a test server and a Client whose httpClient
// is a retryablehttp transport wrapping the server's client, with fast
// backoff settings suitable for tests. The returned client has retries
// enabled (unlike newTestServer, which uses a plain client with no
// retries).
func newRetryTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	rc := retryablehttp.NewClient()
	rc.RetryMax = 4
	rc.RetryWaitMin = 1 * time.Millisecond
	rc.RetryWaitMax = 10 * time.Millisecond
	rc.CheckRetry = retryPolicy
	rc.Backoff = retryablehttp.DefaultBackoff
	rc.HTTPClient = srv.Client()
	rc.Logger = nil // suppress chatty retry logs in tests

	c := New("test-token")
	c.endpoint = srv.URL
	c.httpClient = rc.StandardClient()
	return srv, c
}

// TestClient_RetriesOn429 verifies that a 429 Throttled response is
// retried and eventually succeeds when the server recovers. This is the
// real production scenario: Hardcover rate-limits, the client backs off
// (respecting Retry-After if present), and retries until the request
// goes through.
func TestClient_RetriesOn429(t *testing.T) {
	var attempts atomic.Int32
	_, c := newRetryTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 2 {
			w.Header().Set("Retry-After", "0") // don't actually wait in tests
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{"error": "Throttled"})
			return
		}
		// Third attempt succeeds
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"me": []map[string]any{{"id": 42, "username": "reader"}},
		}})
	}))

	id, _, err := c.Me(context.Background())
	if err != nil {
		t.Fatalf("Me() error = %v, want nil after retries recover from 429", err)
	}
	if id != 42 {
		t.Errorf("Me() id = %d, want 42", id)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("server attempts = %d, want 3 (2 retries + 1 success)", got)
	}
}

// TestClient_DoesNotRetryOn401 verifies that a 401 Unauthorized is NOT
// retried — a bad token is a permanent client error, and retrying it
// just wastes time and spams the API.
func TestClient_DoesNotRetryOn401(t *testing.T) {
	var attempts atomic.Int32
	_, c := newRetryTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{"error": "Unable to verify token"})
	}))

	_, _, err := c.Me(context.Background())
	if err == nil {
		t.Fatal("Me() error = nil, want error for 401")
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("server attempts = %d, want 1 (401 must not be retried)", got)
	}
}

// TestClient_RetriesExhausted verifies that when 429 persists beyond
// RetryMax, the final 429 response is returned as an error to the
// caller (not swallowed or silently retried forever).
func TestClient_RetriesExhausted(t *testing.T) {
	var attempts atomic.Int32
	_, c := newRetryTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]any{"error": "Throttled"})
	}))

	_, _, err := c.Me(context.Background())
	if err == nil {
		t.Fatal("Me() error = nil, want error after retries exhausted on persistent 429")
	}
	// RetryMax=4 means 1 initial + 4 retries = 5 total attempts
	if got := attempts.Load(); got != 5 {
		t.Errorf("server attempts = %d, want 5 (1 initial + 4 retries)", got)
	}
}

// TestClient_DoesNotRetryMutationsOn429 verifies that GraphQL mutations
// (marked via the X-GraphQL-Operation: mutation header set in do()) are
// NOT retried on 429. A 429 response does not guarantee the server
// failed to process the mutation — a rate-limiter proxy may return 429
// after the upstream already committed the INSERT. Retrying a
// non-idempotent mutation like insert_user_book in that case creates
// duplicate read entries on the user's Hardcover library.
func TestClient_DoesNotRetryMutationsOn429(t *testing.T) {
	var mutationAttempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		// This is the mutation (insert_user_book) — 429 it.
		// Verify the mutation header is set so retryPolicy can identify it.
		if r.Header.Get("X-GraphQL-Operation") != "mutation" {
			t.Errorf("X-GraphQL-Operation = %q, want %q for mutation", r.Header.Get("X-GraphQL-Operation"), "mutation")
		}
		mutationAttempts.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]any{"error": "Throttled"})
	}))
	t.Cleanup(srv.Close)

	rc := retryablehttp.NewClient()
	rc.RetryMax = 4
	rc.RetryWaitMin = 1 * time.Millisecond
	rc.RetryWaitMax = 10 * time.Millisecond
	rc.CheckRetry = retryPolicy
	rc.Backoff = retryablehttp.DefaultBackoff
	rc.HTTPClient = srv.Client()
	rc.Logger = nil

	c := New("test-token")
	c.endpoint = srv.URL
	c.httpClient = rc.StandardClient()

	err := c.InsertUserBook(context.Background(), 777, StatusRead, nil, nil, nil)
	if err == nil {
		t.Fatal("InsertUserBook() error = nil, want error for 429 on mutation")
	}
	if got := mutationAttempts.Load(); got != 1 {
		t.Errorf("mutation attempts = %d, want 1 (mutations must not be retried on 429)", got)
	}
}

func TestClient_PacesRequests(t *testing.T) {
	_, c := newTestServer(t, func(string, map[string]any) (any, error) {
		return map[string]any{}, nil
	})
	c.limiter = newLimiter(50 * time.Millisecond)

	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := c.do(context.Background(), "query { me { id } }", nil, nil); err != nil {
			t.Fatalf("do: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("3 requests took %v, want >= 100ms with 50ms pacing", elapsed)
	}
}

func TestClient_PaceRespectsContextCancel(t *testing.T) {
	c := &Client{limiter: newLimiter(time.Hour)}
	c.limiter.observe(http.Header{}, time.Now())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.pace(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("pace err = %v, want context.Canceled", err)
	}
}
