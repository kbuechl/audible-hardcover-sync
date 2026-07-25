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
			name: "insert when absent",
			handler: func(t *testing.T) func(string, map[string]any) (any, error) {
				return func(query string, vars map[string]any) (any, error) {
					switch {
					case strings.Contains(query, "me"):
						return map[string]any{"me": []map[string]any{{"id": 42, "username": "reader"}}}, nil
					case strings.Contains(query, "user_books("):
						return map[string]any{"user_books": []map[string]any{}}, nil
					case strings.Contains(query, "insert_user_book"):
						return map[string]any{"insert_user_book": map[string]any{"id": 1}}, nil
					default:
						t.Fatalf("unexpected query: %s", query)
						return nil, nil
					}
				}
			},
		},
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
