package hardcover

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestParseRateLimit(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   []rateLimitEntry
	}{
		{
			name:   "free tier with daily",
			header: `"Free (JWT)";r=4;t=0, "daily";r=4965;t=32269`,
			want: []rateLimitEntry{
				{Name: "Free (JWT)", Remaining: 4, Reset: 0},
				{Name: "daily", Remaining: 4965, Reset: 32269 * time.Second},
			},
		},
		{
			name:   "empty",
			header: "",
			want:   nil,
		},
		{
			name:   "malformed entry skipped",
			header: `garbage, "daily";r=0;t=10`,
			want:   []rateLimitEntry{{Name: "daily", Remaining: 0, Reset: 10 * time.Second}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRateLimit(tt.header)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d entries %+v, want %d", len(got), got, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("entry %d = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseRateLimitPolicy(t *testing.T) {
	got := parseRateLimitPolicy(`"Supporter (JWT)";q=60;w=60;burst=15, "daily";q=50000;w=86400`)
	want := []rateLimitPolicy{
		{Name: "Supporter (JWT)", Quota: 60, Window: 60 * time.Second, Burst: 15},
		{Name: "daily", Quota: 50000, Window: 86400 * time.Second},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func headers(rl, policy string) http.Header {
	h := http.Header{}
	if rl != "" {
		h.Set("RateLimit", rl)
	}
	if policy != "" {
		h.Set("RateLimit-Policy", policy)
	}
	return h
}

const freePolicy = `"Free (JWT)";q=60;w=60;burst=5, "daily";q=5000;w=86400`

func TestLimiter_BurstRemainingAllowsImmediateRequest(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newLimiter(time.Second)
	l.observe(headers(`"Free (JWT)";r=3;t=0, "daily";r=100;t=500`, freePolicy), now)
	if got := l.wait(now); got != 0 {
		t.Fatalf("wait = %v, want 0 while burst remains", got)
	}
}

func TestLimiter_EmptyBurstWaitsForRefill(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newLimiter(time.Second)
	// t=0 with r=0: bucket refills at q/w = 1 req/s, so wait one refill.
	l.observe(headers(`"Free (JWT)";r=0;t=0, "daily";r=100;t=500`, freePolicy), now)
	if got := l.wait(now); got != time.Second {
		t.Fatalf("wait = %v, want 1s refill", got)
	}
}

func TestLimiter_EmptyBurstUsesResetWhenLonger(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newLimiter(time.Second)
	l.observe(headers(`"Free (JWT)";r=0;t=3, "daily";r=100;t=500`, freePolicy), now)
	if got := l.wait(now); got != 3*time.Second {
		t.Fatalf("wait = %v, want 3s", got)
	}
}

func TestLimiter_NoHeadersFallsBackToFixedGap(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newLimiter(1100 * time.Millisecond)
	l.observe(http.Header{}, now)
	if got := l.wait(now.Add(100 * time.Millisecond)); got != time.Second {
		t.Fatalf("wait = %v, want 1s remaining of fallback gap", got)
	}
}

func TestLimiter_DailyExhausted(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newLimiter(time.Second)
	l.observe(headers(`"Free (JWT)";r=4;t=0, "daily";r=0;t=600`, freePolicy), now)
	if err := l.dailyErr(now.Add(time.Minute)); !errors.Is(err, ErrDailyQuotaExhausted) {
		t.Fatalf("dailyErr = %v, want ErrDailyQuotaExhausted", err)
	}
	if err := l.dailyErr(now.Add(11 * time.Minute)); err != nil {
		t.Fatalf("dailyErr after reset = %v, want nil", err)
	}
}

func TestLimiter_ReportsTier(t *testing.T) {
	l := newLimiter(time.Second)
	l.observe(headers("", `"Supporter (JWT)";q=60;w=60;burst=15, "daily";q=50000;w=86400`), time.Unix(0, 0))
	if l.tier != "Supporter (JWT)" {
		t.Fatalf("tier = %q, want Supporter (JWT)", l.tier)
	}
}

func TestClient_DailyExhaustedStopsBeforeRequest(t *testing.T) {
	calls := 0
	_, c := newTestServer(t, func(string, map[string]any) (any, error) {
		calls++
		return map[string]any{}, nil
	})
	c.limiter.observe(headers(`"Free (JWT)";r=4;t=0, "daily";r=0;t=600`, freePolicy), time.Now())
	err := c.do(context.Background(), "query { me { id } }", nil, nil)
	if !errors.Is(err, ErrDailyQuotaExhausted) {
		t.Fatalf("err = %v, want ErrDailyQuotaExhausted", err)
	}
	if calls != 0 {
		t.Fatalf("server called %d times, want 0", calls)
	}
}
