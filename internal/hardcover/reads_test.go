package hardcover

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestClient_StartRead(t *testing.T) {
	started := time.Date(2026, 10, 18, 5, 29, 0, 0, time.UTC)
	tests := []struct {
		name       string
		openReads  []map[string]any
		wantID     int64
		wantInsert bool
	}{
		{"inserts when no open read exists", nil, 7001, true},
		{"adopts an existing open read instead of duplicating", []map[string]any{{"id": 6500}}, 6500, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inserted := false
			_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
				if strings.Contains(query, "insert_user_book_read") {
					inserted = true
					read := vars["read"].(map[string]any)
					if read["started_at"] != "2026-10-18" || read["progress_seconds"] != float64(720) {
						t.Errorf("insert read = %v", read)
					}
					return map[string]any{"insert_user_book_read": map[string]any{"id": 7001, "error": nil}}, nil
				}
				if vars["since"] != "2026-10-18" {
					t.Errorf("open-read lookup since = %v, want 2026-10-18", vars["since"])
				}
				return map[string]any{"user_book_reads": tt.openReads}, nil
			})
			id, err := c.StartRead(context.Background(), 200, started, 720)
			if err != nil {
				t.Fatalf("StartRead() error = %v", err)
			}
			if id != tt.wantID || inserted != tt.wantInsert {
				t.Errorf("StartRead() = %d inserted=%v, want %d inserted=%v", id, inserted, tt.wantID, tt.wantInsert)
			}
		})
	}
}

func TestClient_FinishRead_SendsDateAndChecksError(t *testing.T) {
	_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
		obj := vars["object"].(map[string]any)
		if obj["finished_at"] != "2026-11-02" || len(obj) != 1 {
			t.Errorf("update object = %v, want only finished_at", obj)
		}
		return map[string]any{"update_user_book_read": map[string]any{"id": 7001, "error": "not yours"}}, nil
	})
	err := c.FinishRead(context.Background(), 7001, time.Date(2026, 11, 2, 20, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "not yours") {
		t.Errorf("FinishRead() error = %v, want mutation error surfaced", err)
	}
}

func TestClient_FetchUserBooks_LastReadDate(t *testing.T) {
	_, c := newTestServer(t, func(query string, vars map[string]any) (any, error) {
		if strings.Contains(query, "me") && !strings.Contains(query, "user_books(") {
			return map[string]any{"me": []map[string]any{{"id": 42, "username": "reader"}}}, nil
		}
		return map[string]any{"user_books": []map[string]any{
			{"id": 200, "book_id": 888, "status_id": StatusRead, "last_read_date": "2026-06-27"},
			{"id": 300, "book_id": 999, "status_id": StatusRead, "last_read_date": nil},
		}}, nil
	})
	books, err := c.FetchUserBooks(context.Background())
	if err != nil {
		t.Fatalf("FetchUserBooks() error = %v", err)
	}
	if d := books[888].LastReadDate; d == nil || d.Format(dateLayout) != "2026-06-27" {
		t.Errorf("books[888].LastReadDate = %v, want 2026-06-27", d)
	}
	if books[999].LastReadDate != nil {
		t.Errorf("books[999].LastReadDate = %v, want nil", books[999].LastReadDate)
	}
}
