// internal/matcher/matcher_test.go
package matcher

import "testing"

func TestBestISBN(t *testing.T) {
	tests := []struct {
		name     string
		item     AudibleItem
		wantISBN string
		wantOK   bool
	}{
		{"prefers isbn13", AudibleItem{ISBN13: "9780641723445", ISBN10: "0641723440"}, "9780641723445", true},
		{"falls back to isbn10", AudibleItem{ISBN13: "", ISBN10: "0641723440"}, "0641723440", true},
		{"no isbn available", AudibleItem{ASIN: "B00WRHY15Y"}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isbn, ok := BestISBN(tt.item)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if isbn != tt.wantISBN {
				t.Errorf("isbn = %q, want %q", isbn, tt.wantISBN)
			}
		})
	}
}
