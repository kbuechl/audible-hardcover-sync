package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kbuechl/audible-hardcover-sync/internal/config"
)

// TestRunOnce_Errors covers runOnce's failure modes that aren't network
// dependent: a missing/invalid Audible credentials file, and a state store
// that fails to open. The sync.Run failure path itself is already covered
// by internal/sync's tests against fakes (runOnce can't inject a fake
// AudibleClient — it constructs the real audible.Client directly — so
// exercising that path here would require a real network call).
func TestRunOnce_Errors(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T, dir string) config.Config
		wantErrText string
	}{
		{
			name: "missing credentials file",
			setup: func(t *testing.T, dir string) config.Config {
				return config.Config{
					HardcoverAPIToken: "unused",
					StateDBPath:       filepath.Join(dir, "state.db"),
					CredentialsPath:   filepath.Join(dir, "does-not-exist.json"),
				}
			},
			wantErrText: "load audible credentials",
		},
		{
			name: "invalid credentials file contents",
			setup: func(t *testing.T, dir string) config.Config {
				credPath := filepath.Join(dir, "credentials.json")
				if err := os.WriteFile(credPath, []byte("not valid json"), 0o600); err != nil {
					t.Fatalf("write credentials file: %v", err)
				}
				return config.Config{
					HardcoverAPIToken: "unused",
					StateDBPath:       filepath.Join(dir, "state.db"),
					CredentialsPath:   credPath,
				}
			},
			wantErrText: "load audible credentials",
		},
		{
			name: "state store fails to open",
			setup: func(t *testing.T, dir string) config.Config {
				credPath := filepath.Join(dir, "credentials.json")
				if err := os.WriteFile(credPath, []byte("{}"), 0o600); err != nil {
					t.Fatalf("write credentials file: %v", err)
				}
				return config.Config{
					HardcoverAPIToken: "unused",
					// Parent directory doesn't exist, so sqlite fails to
					// open/apply schema immediately in state.Open.
					StateDBPath:     filepath.Join(dir, "no-such-subdir", "state.db"),
					CredentialsPath: credPath,
				}
			},
			wantErrText: "open state store",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := tt.setup(t, dir)

			_, err := runOnce(context.Background(), cfg)
			if err == nil {
				t.Fatalf("runOnce() error = nil, want error containing %q", tt.wantErrText)
			}
			if !strings.Contains(err.Error(), tt.wantErrText) {
				t.Errorf("runOnce() error = %q, want it to contain %q", err.Error(), tt.wantErrText)
			}
		})
	}
}
