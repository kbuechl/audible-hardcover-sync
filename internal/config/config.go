package config

import (
	"fmt"
	"os"
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

	return Config{
		HardcoverAPIToken: token,
		SyncInterval:      interval,
		StateDBPath:       stateDBPath,
		CredentialsPath:   credentialsPath,
		WebUIPort:         webUIPort,
		PublicURL:         publicURL,
		NotifyURLs:        notifyURLs,
	}, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
