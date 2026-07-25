package audible

import (
	"encoding/json"
	"testing"
)

func TestBook_UnmarshalJSON_IsFinished(t *testing.T) {
	tests := []struct {
		name string
		json string
		want bool
	}{
		{"finished true", `{"asin":"B00WRHY15Y","title":"x","is_finished":true}`, true},
		{"finished false", `{"asin":"B00WRHY15Y","title":"x","is_finished":false}`, false},
		{"field absent", `{"asin":"B00WRHY15Y","title":"x"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b Book
			if err := json.Unmarshal([]byte(tt.json), &b); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if b.IsFinished != tt.want {
				t.Errorf("IsFinished = %v, want %v", b.IsFinished, tt.want)
			}
		})
	}
}
