package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HardcoverAPIToken string
	SyncInterval      time.Duration
	StateDBPath       string
	CredentialsPath   string
	WebUIPort         string
	PublicURL         string
	NotifyURLs        []string
	MatchConfidence   float64
}

func Load() (Config, error) {
	token := os.Getenv("HARDCOVER_API_TOKEN")
	if token == "" {
		return Config{}, fmt.Errorf("HARDCOVER_API_TOKEN is required")
	}

	interval := 6 * time.Hour
	if v := os.Getenv("SYNC_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid SYNC_INTERVAL %q: %w", v, err)
		}
		interval = d
	}

	stateDBPath := envOrDefault("STATE_DB_PATH", "/data/state.db")
	credentialsPath := envOrDefault("CREDENTIALS_PATH", "/data/credentials.json")
	webUIPort := envOrDefault("WEB_UI_PORT", "8080")
	publicURL := os.Getenv("PUBLIC_URL")

	var notifyURLs []string
	if v := os.Getenv("NOTIFY_URLS"); v != "" {
		for _, u := range strings.Split(v, ",") {
			u = strings.TrimSpace(u)
			if u != "" {
				notifyURLs = append(notifyURLs, u)
			}
		}
	}

	matchConfidence, err := parseMatchConfidence(os.Getenv("MATCH_CONFIDENCE"))
	if err != nil {
		return Config{}, err
	}

	return Config{
		HardcoverAPIToken: token,
		SyncInterval:      interval,
		StateDBPath:       stateDBPath,
		CredentialsPath:   credentialsPath,
		WebUIPort:         webUIPort,
		PublicURL:         publicURL,
		NotifyURLs:        notifyURLs,
		MatchConfidence:   matchConfidence,
	}, nil
}

// parseMatchConfidence parses the MATCH_CONFIDENCE setting, accepting either
// a named preset ("strict"=0.95, "medium"=0.75, "loose"=0.55) or a raw
// number. Values > 1 (or with a trailing "%") are treated as a percentage
// and divided by 100; values in [0,1] are used as-is. An empty string
// defaults to "strict" (0.95) for safe-by-default behavior.
func parseMatchConfidence(v string) (float64, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "":
		return 0.95, nil
	case "strict":
		return 0.95, nil
	case "medium":
		return 0.75, nil
	case "loose":
		return 0.55, nil
	}

	trimmed := strings.TrimSpace(v)
	hadPercentSign := strings.HasSuffix(trimmed, "%")
	numStr := strings.TrimSuffix(trimmed, "%")

	f, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid MATCH_CONFIDENCE %q: %w", v, err)
	}

	if hadPercentSign || f > 1 {
		f = f / 100
	}

	if f < 0 || f > 1 {
		return 0, fmt.Errorf("MATCH_CONFIDENCE %q is out of range [0,1] after normalization (got %v)", v, f)
	}
	return f, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
