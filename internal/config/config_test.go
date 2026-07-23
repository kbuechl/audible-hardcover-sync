package config

import (
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("HARDCOVER_API_TOKEN", "test-token")
	for _, k := range []string{"SYNC_INTERVAL", "STATE_DB_PATH", "CREDENTIALS_PATH", "WEB_UI_PORT", "PUBLIC_URL", "NOTIFY_URLS"} {
		t.Setenv(k, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.HardcoverAPIToken != "test-token" {
		t.Errorf("HardcoverAPIToken = %q, want %q", cfg.HardcoverAPIToken, "test-token")
	}
	if cfg.SyncInterval != 6*time.Hour {
		t.Errorf("SyncInterval = %v, want %v", cfg.SyncInterval, 6*time.Hour)
	}
	if cfg.StateDBPath != "/data/state.db" {
		t.Errorf("StateDBPath = %q, want %q", cfg.StateDBPath, "/data/state.db")
	}
	if cfg.CredentialsPath != "/data/credentials.json" {
		t.Errorf("CredentialsPath = %q, want %q", cfg.CredentialsPath, "/data/credentials.json")
	}
	if cfg.WebUIPort != "8080" {
		t.Errorf("WebUIPort = %q, want %q", cfg.WebUIPort, "8080")
	}
	if len(cfg.NotifyURLs) != 0 {
		t.Errorf("NotifyURLs = %v, want empty", cfg.NotifyURLs)
	}
}

func TestLoad_Overrides(t *testing.T) {
	t.Setenv("HARDCOVER_API_TOKEN", "test-token")
	t.Setenv("SYNC_INTERVAL", "30m")
	t.Setenv("STATE_DB_PATH", "/tmp/x.db")
	t.Setenv("CREDENTIALS_PATH", "/tmp/creds.json")
	t.Setenv("WEB_UI_PORT", "9090")
	t.Setenv("PUBLIC_URL", "http://homeserver.local:9090")
	t.Setenv("NOTIFY_URLS", "discord://token@id,mailto://user:pass@smtp.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.SyncInterval != 30*time.Minute {
		t.Errorf("SyncInterval = %v, want 30m", cfg.SyncInterval)
	}
	if len(cfg.NotifyURLs) != 2 {
		t.Fatalf("NotifyURLs = %v, want 2 entries", cfg.NotifyURLs)
	}
	if cfg.NotifyURLs[0] != "discord://token@id" {
		t.Errorf("NotifyURLs[0] = %q", cfg.NotifyURLs[0])
	}
}

func TestLoad_MissingToken(t *testing.T) {
	t.Setenv("HARDCOVER_API_TOKEN", "")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error for missing HARDCOVER_API_TOKEN")
	}
}
