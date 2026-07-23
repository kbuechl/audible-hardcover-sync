package config

import (
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want Config
	}{
		{
			name: "defaults",
			env:  map[string]string{"HARDCOVER_API_TOKEN": "test-token"},
			want: Config{
				HardcoverAPIToken: "test-token",
				SyncInterval:      6 * time.Hour,
				StateDBPath:       "/data/state.db",
				CredentialsPath:   "/data/credentials.json",
				WebUIPort:         "8080",
				PublicURL:         "",
				NotifyURLs:        nil,
			},
		},
		{
			name: "overrides",
			env: map[string]string{
				"HARDCOVER_API_TOKEN": "test-token",
				"SYNC_INTERVAL":       "30m",
				"STATE_DB_PATH":       "/tmp/x.db",
				"CREDENTIALS_PATH":    "/tmp/creds.json",
				"WEB_UI_PORT":         "9090",
				"PUBLIC_URL":          "http://homeserver.local:9090",
				"NOTIFY_URLS":         "discord://token@id,mailto://user:pass@smtp.example.com",
			},
			want: Config{
				HardcoverAPIToken: "test-token",
				SyncInterval:      30 * time.Minute,
				StateDBPath:       "/tmp/x.db",
				CredentialsPath:   "/tmp/creds.json",
				WebUIPort:         "9090",
				PublicURL:         "http://homeserver.local:9090",
				NotifyURLs:        []string{"discord://token@id", "mailto://user:pass@smtp.example.com"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range []string{"HARDCOVER_API_TOKEN", "SYNC_INTERVAL", "STATE_DB_PATH", "CREDENTIALS_PATH", "WEB_UI_PORT", "PUBLIC_URL", "NOTIFY_URLS"} {
				t.Setenv(k, tt.env[k]) // empty string for unset keys, which clears any inherited value
			}
			got, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v, want nil", err)
			}
			if got.HardcoverAPIToken != tt.want.HardcoverAPIToken {
				t.Errorf("HardcoverAPIToken = %q, want %q", got.HardcoverAPIToken, tt.want.HardcoverAPIToken)
			}
			if got.SyncInterval != tt.want.SyncInterval {
				t.Errorf("SyncInterval = %v, want %v", got.SyncInterval, tt.want.SyncInterval)
			}
			if got.StateDBPath != tt.want.StateDBPath {
				t.Errorf("StateDBPath = %q, want %q", got.StateDBPath, tt.want.StateDBPath)
			}
			if got.CredentialsPath != tt.want.CredentialsPath {
				t.Errorf("CredentialsPath = %q, want %q", got.CredentialsPath, tt.want.CredentialsPath)
			}
			if got.WebUIPort != tt.want.WebUIPort {
				t.Errorf("WebUIPort = %q, want %q", got.WebUIPort, tt.want.WebUIPort)
			}
			if got.PublicURL != tt.want.PublicURL {
				t.Errorf("PublicURL = %q, want %q", got.PublicURL, tt.want.PublicURL)
			}
			if len(got.NotifyURLs) != len(tt.want.NotifyURLs) {
				t.Fatalf("NotifyURLs = %v, want %v", got.NotifyURLs, tt.want.NotifyURLs)
			}
			for i := range tt.want.NotifyURLs {
				if got.NotifyURLs[i] != tt.want.NotifyURLs[i] {
					t.Errorf("NotifyURLs[%d] = %q, want %q", i, got.NotifyURLs[i], tt.want.NotifyURLs[i])
				}
			}
		})
	}
}

func TestLoad_MissingToken(t *testing.T) {
	t.Setenv("HARDCOVER_API_TOKEN", "")
	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error for missing HARDCOVER_API_TOKEN")
	}
}

func TestLoad_InvalidSyncInterval(t *testing.T) {
	t.Setenv("HARDCOVER_API_TOKEN", "test-token")
	t.Setenv("SYNC_INTERVAL", "not-a-duration")
	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error for invalid SYNC_INTERVAL")
	}
}
