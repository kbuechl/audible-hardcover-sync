package hardcover

import (
	"context"
	"strings"
	"testing"
)

func TestClient_FetchUserBooks_LatestReadProgress(t *testing.T) {
	_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
		if strings.Contains(query, "me") && !strings.Contains(query, "user_books(") {
			return map[string]any{"me": []map[string]any{{"id": 42, "username": "reader"}}}, nil
		}
		if !strings.Contains(query, "progress_seconds") {
			t.Errorf("FetchUserBooks query does not select progress_seconds: %s", query)
		}
		return map[string]any{"user_books": []map[string]any{
			{"id": 200, "book_id": 888, "status_id": StatusCurrentlyReading,
				"edition":         map[string]any{"audio_seconds": 55440},
				"user_book_reads": []map[string]any{{"progress_seconds": 1200}}},
			{"id": 300, "book_id": 999, "status_id": StatusCurrentlyReading,
				"user_book_reads": []map[string]any{{"progress_seconds": nil}}},
		}}, nil
	})

	books, err := c.FetchUserBooks(context.Background())
	if err != nil {
		t.Fatalf("FetchUserBooks() error = %v", err)
	}
	if got := books[888].ProgressSeconds; got != 1200 {
		t.Errorf("books[888].ProgressSeconds = %d, want 1200", got)
	}
	if got := books[888].AudioSeconds; got != 55440 {
		t.Errorf("books[888].AudioSeconds = %d, want 55440", got)
	}
	if got := books[999].ProgressSeconds; got != 0 {
		t.Errorf("books[999].ProgressSeconds = %d, want 0 for null progress", got)
	}
}

func TestClient_InsertUserBook_ReturnsID(t *testing.T) {
	_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
		return map[string]any{"insert_user_book": map[string]any{"id": 4321}}, nil
	})
	id, err := c.InsertUserBook(context.Background(), 777, StatusCurrentlyReading, nil, nil, nil)
	if err != nil {
		t.Fatalf("InsertUserBook() error = %v", err)
	}
	if id != 4321 {
		t.Errorf("InsertUserBook() id = %d, want 4321", id)
	}
}

func TestClient_SetReadProgress(t *testing.T) {
	tests := []struct {
		name         string
		existingRead []map[string]any
		audioSeconds any // edition length on Hardcover; nil when unknown
		wantMutation string
		wantSeconds  float64
		mutationErr  string
		wantErr      bool
	}{
		{name: "uses Hardcover edition length", existingRead: []map[string]any{{"id": 55}}, audioSeconds: 55440, wantMutation: "update_user_book_read", wantSeconds: 31046},
		{name: "falls back to Audible runtime", existingRead: []map[string]any{{"id": 55}}, audioSeconds: nil, wantMutation: "update_user_book_read", wantSeconds: 900},
		{name: "inserts read when none exists", existingRead: []map[string]any{}, audioSeconds: nil, wantMutation: "insert_user_book_read", wantSeconds: 900},
		{name: "mutation error field is an error", existingRead: []map[string]any{{"id": 55}}, wantMutation: "update_user_book_read", wantSeconds: 900, mutationErr: "nope", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called string
			var gotSeconds any
			_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
				switch {
				case strings.Contains(query, "update_user_book_read"):
					called = "update_user_book_read"
					if vars["id"] != float64(55) {
						t.Errorf("update id = %v, want 55", vars["id"])
					}
					gotSeconds = vars["object"].(map[string]any)["progress_seconds"]
					return map[string]any{"update_user_book_read": map[string]any{"id": 55, "error": nilIfEmpty(tt.mutationErr)}}, nil
				case strings.Contains(query, "insert_user_book_read"):
					called = "insert_user_book_read"
					if vars["userBookID"] != float64(4321) {
						t.Errorf("insert user_book_id = %v, want 4321", vars["userBookID"])
					}
					gotSeconds = vars["read"].(map[string]any)["progress_seconds"]
					return map[string]any{"insert_user_book_read": map[string]any{"id": 56, "error": nilIfEmpty(tt.mutationErr)}}, nil
				case strings.Contains(query, "user_book_reads("):
					return map[string]any{
						"user_book_reads": tt.existingRead,
						"user_books":      []map[string]any{{"edition": map[string]any{"audio_seconds": tt.audioSeconds}}},
					}, nil
				}
				t.Fatalf("unexpected query: %s", query)
				return nil, nil
			})

			// 56% of a 15.4h edition = 31046s; fallback is Audible's 900s.
			err := c.SetReadProgress(context.Background(), 4321, 56, 900)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SetReadProgress() error = %v, wantErr %v", err, tt.wantErr)
			}
			if called != tt.wantMutation {
				t.Errorf("called %q, want %q", called, tt.wantMutation)
			}
			if gotSeconds != tt.wantSeconds {
				t.Errorf("progress_seconds = %v, want %v", gotSeconds, tt.wantSeconds)
			}
		})
	}
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
