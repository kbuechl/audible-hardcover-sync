package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("HARDCOVER_API_TOKEN", "test-token")
	t.Setenv("DATA_DIR", "")
	t.Setenv("WEB_UI_PORT", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := Config{
		HardcoverAPIToken:  "test-token",
		HardcoverTokenPath: "/data/hardcover-token",
		StateDBPath:        "/data/state.db",
		CredentialsPath:    "/data/credentials.json",
		WebUIPort:          "8080",
	}
	if cfg != want {
		t.Errorf("Load() = %+v, want %+v", cfg, want)
	}
}

func TestLoad_Overrides(t *testing.T) {
	t.Setenv("HARDCOVER_API_TOKEN", "")
	t.Setenv("DATA_DIR", "/tmp/ahs")
	t.Setenv("WEB_UI_PORT", "9090")
	t.Setenv("CREDENTIALS_KEY", "k")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.StateDBPath != "/tmp/ahs/state.db" || cfg.WebUIPort != "9090" || cfg.CredentialsKey != "k" {
		t.Errorf("Load() = %+v", cfg)
	}
}

// The Hardcover token is optional in the environment: it can be set from
// the web UI instead, saved in the data folder.
func TestLoad_TokenOptional(t *testing.T) {
	t.Setenv("HARDCOVER_API_TOKEN", "")
	t.Setenv("DATA_DIR", "/tmp/ahs")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil without HARDCOVER_API_TOKEN", err)
	}
	if cfg.HardcoverAPIToken != "" || cfg.HardcoverTokenPath != "/tmp/ahs/hardcover-token" {
		t.Errorf("token = %q, path = %q", cfg.HardcoverAPIToken, cfg.HardcoverTokenPath)
	}
}
