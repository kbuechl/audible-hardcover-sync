package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	// HardcoverAPIToken comes from the environment. When empty, a token
	// saved from the web UI (HardcoverTokenPath) is used instead.
	HardcoverAPIToken  string
	HardcoverTokenPath string
	StateDBPath        string
	CredentialsPath    string
	// CredentialsKey encrypts the Audible credentials file when set.
	CredentialsKey string
	WebUIPort      string
}

func Load() (Config, error) {
	token := os.Getenv("HARDCOVER_API_TOKEN")

	dataDir := DataDir()
	stateDBPath := filepath.Join(dataDir, "state.db")
	credentialsPath := CredentialsPath()
	webUIPort := EnvOrDefault("WEB_UI_PORT", "8080")

	return Config{
		HardcoverAPIToken:  token,
		HardcoverTokenPath: filepath.Join(dataDir, "hardcover-token"),
		StateDBPath:        stateDBPath,
		CredentialsPath:    credentialsPath,
		CredentialsKey:     os.Getenv("CREDENTIALS_KEY"),
		WebUIPort:          webUIPort,
	}, nil
}

// DataDir is where the state database and Audible credentials live:
// DATA_DIR, or /data (the container's volume) when unset.
func DataDir() string {
	return EnvOrDefault("DATA_DIR", "/data")
}

// CredentialsPath is the Audible credentials file inside DataDir.
func CredentialsPath() string {
	return filepath.Join(DataDir(), "credentials.json")
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
