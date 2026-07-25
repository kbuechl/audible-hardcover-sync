package hardcover

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
		return map[string]any{"editions": []map[string]any{{"book_id": 777}}}, nil
	})

	id, found, err := c.FindBookByISBN(context.Background(), "9780641723445")
	if err != nil {
		t.Fatalf("FindBookByISBN() error = %v", err)
	}
	if !found || id != 777 {
		t.Errorf("FindBookByISBN() = (%d, %v), want (777, true)", id, found)
	}
}

func TestClient_FindBookByISBN_NotFound(t *testing.T) {
	_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
		return map[string]any{"editions": []map[string]any{}}, nil
	})

	_, found, err := c.FindBookByISBN(context.Background(), "0000000000")
	if err != nil {
		t.Fatalf("FindBookByISBN() error = %v", err)
	}
	if found {
		t.Error("found = true, want false for unknown ISBN")
	}
}

func TestClient_UpsertUserBook_InsertsWhenAbsent(t *testing.T) {
	var insertCalled bool
	_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
		switch {
		case strings.Contains(query, "me"):
			return map[string]any{"me": []map[string]any{{"id": 42, "username": "reader"}}}, nil
		case strings.Contains(query, "user_books("):
			return map[string]any{"user_books": []map[string]any{}}, nil
		case strings.Contains(query, "insert_user_book"):
			insertCalled = true
			return map[string]any{"insert_user_book": map[string]any{"id": 1}}, nil
		default:
			t.Fatalf("unexpected query: %s", query)
			return nil, nil
		}
	})

	if err := c.UpsertUserBook(context.Background(), 777, StatusRead, nil); err != nil {
		t.Fatalf("UpsertUserBook() error = %v", err)
	}
	if !insertCalled {
		t.Error("expected insert_user_book to be called when no existing row")
	}
}

func TestClient_UpsertUserBook_UpdatesWhenPresent(t *testing.T) {
	var updateCalled bool
	_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
		switch {
		case strings.Contains(query, "me"):
			return map[string]any{"me": []map[string]any{{"id": 42, "username": "reader"}}}, nil
		case strings.Contains(query, "user_books("):
			return map[string]any{"user_books": []map[string]any{{"id": 555, "status_id": 2, "rating": nil}}}, nil
		case strings.Contains(query, "update_user_book_by_pk"):
			updateCalled = true
			return map[string]any{"update_user_book_by_pk": map[string]any{"id": 555}}, nil
		default:
			t.Fatalf("unexpected query: %s", query)
			return nil, nil
		}
	})

	if err := c.UpsertUserBook(context.Background(), 777, StatusRead, nil); err != nil {
		t.Fatalf("UpsertUserBook() error = %v", err)
	}
	if !updateCalled {
		t.Error("expected update_user_book_by_pk to be called when a row already exists")
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

	_, found, err := c.FindBookByISBN(context.Background(), "9780641723445")
	if err == nil {
		t.Fatal("FindBookByISBN() error = nil, want error")
	}
	if found {
		t.Error("found = true, want false on error")
	}
}

// TestClient_UpsertUserBook_Table exercises the insert/update branches
// together with two failure modes: resolving the current user fails, and
// the existing-row lookup itself fails.
func TestClient_UpsertUserBook_Table(t *testing.T) {
	tests := []struct {
		name    string
		handler func(t *testing.T) func(query string, vars map[string]any) (any, error)
		wantErr string
	}{
		{
			name: "error resolving user id",
			handler: func(t *testing.T) func(string, map[string]any) (any, error) {
				return func(query string, vars map[string]any) (any, error) {
					if strings.Contains(query, "me") {
						return nil, errors.New("Unable to verify token")
					}
					t.Fatalf("unexpected query after me failure: %s", query)
					return nil, nil
				}
			},
			wantErr: "resolve hardcover user id",
		},
		{
			name: "error querying existing user_book",
			handler: func(t *testing.T) func(string, map[string]any) (any, error) {
				return func(query string, vars map[string]any) (any, error) {
					switch {
					case strings.Contains(query, "me"):
						return map[string]any{"me": []map[string]any{{"id": 42, "username": "reader"}}}, nil
					case strings.Contains(query, "user_books("):
						return nil, errors.New("db timeout")
					default:
						t.Fatalf("unexpected query: %s", query)
						return nil, nil
					}
				}
			},
			wantErr: "query existing user_book",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, c := newTestServer(t, tt.handler(t))

			err := c.UpsertUserBook(context.Background(), 777, StatusRead, nil)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("UpsertUserBook() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("UpsertUserBook() error = %v, want nil", err)
			}
		})
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
// invokes do() for insert_user_book and update_user_book_by_pk. Before the
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
