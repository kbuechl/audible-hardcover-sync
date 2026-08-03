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

	stateDBPath := EnvOrDefault("STATE_DB_PATH", "/data/state.db")
	credentialsPath := EnvOrDefault("CREDENTIALS_PATH", "/data/credentials.json")
	webUIPort := EnvOrDefault("WEB_UI_PORT", "8080")
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
// number in [0,1]. Values outside [0,1] are rejected. An empty string
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

	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid MATCH_CONFIDENCE %q: %w", v, err)
	}

	if f < 0 || f > 1 {
		return 0, fmt.Errorf("MATCH_CONFIDENCE %q is out of range [0,1] (use a preset name or a value between 0 and 1)", v)
	}
	return f, nil
}

// EnvOrDefault returns the value of the environment variable key, or def
// when the variable is unset or empty. Exported so other packages (e.g.
// cmd/sync-tool) can share the same behavior.
func EnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
