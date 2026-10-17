package hardcover

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ErrDailyQuotaExhausted is returned when Hardcover reports the token's
// daily request quota is used up. Every further request would 429, so
// callers should stop the run rather than fail each remaining book.
var ErrDailyQuotaExhausted = errors.New("hardcover daily request quota exhausted")

const dailyEntryName = "daily"

// rateLimitEntry is one item of Hardcover's RateLimit header, e.g.
// `"Free (JWT)";r=4;t=0` — r requests remaining, resetting in t seconds.
type rateLimitEntry struct {
	Name      string
	Remaining int
	Reset     time.Duration
}

// rateLimitPolicy is one item of the RateLimit-Policy header, e.g.
// `"Free (JWT)";q=60;w=60;burst=5` — q requests per w-second window.
type rateLimitPolicy struct {
	Name   string
	Quota  int
	Window time.Duration
	Burst  int
}

// parseParams splits `"name";k=v;k=v` into the unquoted name and params.
// ok is false when the item has no quoted name.
func parseParams(item string) (name string, params map[string]int, ok bool) {
	parts := strings.Split(strings.TrimSpace(item), ";")
	name = strings.TrimSpace(parts[0])
	if len(name) < 2 || name[0] != '"' || name[len(name)-1] != '"' {
		return "", nil, false
	}
	params = make(map[string]int, len(parts)-1)
	for _, p := range parts[1:] {
		k, v, found := strings.Cut(strings.TrimSpace(p), "=")
		if !found {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			continue
		}
		params[k] = n
	}
	return name[1 : len(name)-1], params, true
}

func parseRateLimit(header string) []rateLimitEntry {
	var out []rateLimitEntry
	for _, item := range strings.Split(header, ",") {
		name, p, ok := parseParams(item)
		if !ok {
			continue
		}
		r, hasR := p["r"]
		if !hasR {
			continue
		}
		out = append(out, rateLimitEntry{Name: name, Remaining: r, Reset: time.Duration(p["t"]) * time.Second})
	}
	return out
}

func parseRateLimitPolicy(header string) []rateLimitPolicy {
	var out []rateLimitPolicy
	for _, item := range strings.Split(header, ",") {
		name, p, ok := parseParams(item)
		if !ok {
			continue
		}
		out = append(out, rateLimitPolicy{
			Name:   name,
			Quota:  p["q"],
			Window: time.Duration(p["w"]) * time.Second,
			Burst:  p["burst"],
		})
	}
	return out
}

// limiter paces requests using what Hardcover reports in its RateLimit
// and RateLimit-Policy response headers, so a Supporter token's larger
// burst and daily quota are used automatically. When a response carries
// no headers, it falls back to a fixed gap between requests.
type limiter struct {
	fallbackGap time.Duration

	nextAllowed    time.Time
	dailyResetAt   time.Time // non-zero while the daily quota is exhausted
	tier           string
	refillInterval time.Duration // window/quota of the tier policy
}

func newLimiter(fallbackGap time.Duration) *limiter {
	return &limiter{fallbackGap: fallbackGap}
}

// observe updates pacing from a response's headers, received at now.
func (l *limiter) observe(h http.Header, now time.Time) {
	for _, p := range parseRateLimitPolicy(h.Get("RateLimit-Policy")) {
		if p.Name == dailyEntryName {
			continue
		}
		if p.Quota > 0 && p.Window > 0 {
			l.refillInterval = p.Window / time.Duration(p.Quota)
		}
		if p.Name != l.tier {
			slog.Info("hardcover rate limit tier", "tier", p.Name, "per_window", p.Quota, "window", p.Window, "burst", p.Burst)
			l.tier = p.Name
		}
	}

	entries := parseRateLimit(h.Get("RateLimit"))
	if len(entries) == 0 {
		l.nextAllowed = now.Add(l.fallbackGap)
		return
	}

	l.nextAllowed = now
	for _, e := range entries {
		if e.Name == dailyEntryName {
			if e.Remaining <= 0 {
				l.dailyResetAt = now.Add(e.Reset)
			} else {
				l.dailyResetAt = time.Time{}
			}
			continue
		}
		if e.Remaining > 0 {
			continue
		}
		// Burst bucket empty: wait for it to reset, and at least one
		// refill interval since t=0 just means "refilling now".
		wait := max(e.Reset, l.refillInterval, l.fallbackGapIfNoPolicy())
		l.nextAllowed = now.Add(wait)
	}
}

// fallbackGapIfNoPolicy covers an empty bucket reported without a policy
// header, where the refill rate is unknown.
func (l *limiter) fallbackGapIfNoPolicy() time.Duration {
	if l.refillInterval > 0 {
		return 0
	}
	return l.fallbackGap
}

// wait returns how long to sleep at now before the next request.
func (l *limiter) wait(now time.Time) time.Duration {
	return max(l.nextAllowed.Sub(now), 0)
}

// dailyErr returns ErrDailyQuotaExhausted while the daily quota is used up.
func (l *limiter) dailyErr(now time.Time) error {
	if l.dailyResetAt.IsZero() || !now.Before(l.dailyResetAt) {
		return nil
	}
	return fmt.Errorf("%w (resets in %s)", ErrDailyQuotaExhausted, l.dailyResetAt.Sub(now).Round(time.Second))
}
