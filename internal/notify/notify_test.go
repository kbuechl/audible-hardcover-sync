package notify

import (
	"context"
	"testing"

	"github.com/kbuechl/audible-hardcover-sync/internal/state"
)

func TestNotifier_NotifyUnmatched(t *testing.T) {
	tests := []struct {
		name      string
		urls      []string
		items     []state.BookState
		publicURL string
		wantErr   bool
	}{
		{
			name:      "no URLs configured is a no-op",
			urls:      nil,
			items:     []state.BookState{{ASIN: "A1"}},
			publicURL: "http://example.com",
			wantErr:   false,
		},
		{
			name: "no items is a no-op even with an invalid-looking URL",
			// An invalid-looking URL would normally fail sender creation; confirm we
			// never even try when there's nothing to report.
			urls:      []string{"invalid://not-a-real-service"},
			items:     nil,
			publicURL: "http://example.com",
			wantErr:   false,
		},
		{
			name:      "invalid service URL with items fails sender creation",
			urls:      []string{"invalid://not-a-real-service"},
			items:     []state.BookState{{ASIN: "A1"}},
			publicURL: "http://example.com",
			wantErr:   true,
		},
		{
			name:      "valid service URL with items sends successfully",
			urls:      []string{"logger://"},
			items:     []state.BookState{{ASIN: "A1"}, {ASIN: "A2"}},
			publicURL: "http://example.com",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := New(tt.urls, tt.publicURL)
			err := n.NotifyUnmatched(context.Background(), tt.items)
			if (err != nil) != tt.wantErr {
				t.Errorf("NotifyUnmatched() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuildMessage(t *testing.T) {
	tests := []struct {
		name      string
		items     []state.BookState
		publicURL string
		want      string
	}{
		{
			name:      "multiple items",
			items:     []state.BookState{{ASIN: "A1"}, {ASIN: "A2"}},
			publicURL: "http://homeserver.local:8080",
			want:      "2 book(s) need manual matching on Hardcover: http://homeserver.local:8080/unmatched",
		},
		{
			name:      "single item",
			items:     []state.BookState{{ASIN: "A1"}},
			publicURL: "http://homeserver.local:8080",
			want:      "1 book(s) need manual matching on Hardcover: http://homeserver.local:8080/unmatched",
		},
		{
			name:      "zero items",
			items:     nil,
			publicURL: "http://homeserver.local:8080",
			want:      "0 book(s) need manual matching on Hardcover: http://homeserver.local:8080/unmatched",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := buildMessage(tt.items, tt.publicURL)
			if msg != tt.want {
				t.Errorf("buildMessage() = %q, want %q", msg, tt.want)
			}
		})
	}
}
