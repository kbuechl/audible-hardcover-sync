package notify

import (
	"strings"
	"testing"
)

func TestMask(t *testing.T) {
	tests := []struct {
		in, want string
		secret   string // must not appear in the output
	}{
		{"discord://s3cr3tT0ken@123456789", "discord://****@123456789", "s3cr3tT0ken"},
		{"ntfy://ntfy.sh/books-topic", "ntfy://ntfy.sh/bo****", "books-topic"},
		{"slack://hook:T000-B000-XXXXXXXX@webhook", "slack://****@webhook", "XXXXXXXX"},
		{"smtp://user:pa55@mail.example.com:587/?from=a@b.c&to=me@x.y", "smtp://****@mail.example.com:587", "pa55"},
		{"not a url", "****", "not a url"},
	}
	for _, tt := range tests {
		got := Mask(tt.in)
		if !strings.HasPrefix(got, tt.want) {
			t.Errorf("Mask(%q) = %q, want prefix %q", tt.in, got, tt.want)
		}
		if strings.Contains(got, tt.secret) {
			t.Errorf("Mask(%q) = %q leaks %q", tt.in, got, tt.secret)
		}
	}
}

func TestValidateAndSendTest(t *testing.T) {
	if err := Validate("logger://"); err != nil {
		t.Errorf("Validate(logger://) = %v", err)
	}
	for _, bad := range []string{"", "invalid://nope"} {
		if err := Validate(bad); err == nil {
			t.Errorf("Validate(%q) = nil, want error", bad)
		}
	}
	if err := SendTest("logger://"); err != nil {
		t.Errorf("SendTest(logger://) = %v", err)
	}
	if err := SendTest("invalid://nope"); err == nil {
		t.Error("SendTest(invalid) = nil, want error")
	}
}
