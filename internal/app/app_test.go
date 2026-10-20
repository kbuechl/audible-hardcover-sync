package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kbuechl/audible-hardcover-sync/internal/config"
	"github.com/kbuechl/audible-hardcover-sync/internal/state"
)

func newTestApp(t *testing.T, mutate func(*config.Config)) *App {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{
		StateDBPath:        filepath.Join(dir, "state.db"),
		CredentialsPath:    filepath.Join(dir, "credentials.json"),
		HardcoverTokenPath: filepath.Join(dir, "hardcover-token"),
	}
	if mutate != nil {
		mutate(&cfg)
	}
	store, err := state.Open(cfg.StateDBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	a, err := New(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestNew_HardcoverTokenSources(t *testing.T) {
	if a := newTestApp(t, nil); a.Hardcover() != nil {
		t.Error("client created without any token")
	}
	if a := newTestApp(t, func(c *config.Config) { c.HardcoverAPIToken = "env" }); a.Hardcover() == nil {
		t.Error("env token not used")
	}
	saved := newTestApp(t, func(c *config.Config) {
		if err := os.WriteFile(c.HardcoverTokenPath, []byte("saved\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	})
	if saved.Hardcover() == nil {
		t.Error("saved token not used")
	}
}

func TestSetHardcoverToken_RefusesWhenFromEnv(t *testing.T) {
	a := newTestApp(t, func(c *config.Config) { c.HardcoverAPIToken = "env" })
	if _, err := a.SetHardcoverToken(context.Background(), "new"); !errors.Is(err, ErrTokenFromEnv) {
		t.Errorf("SetHardcoverToken() = %v, want ErrTokenFromEnv", err)
	}
	if st := a.HardcoverStatus(context.Background()); !st.FromEnv {
		t.Error("status should report the token comes from the environment")
	}
}

func TestSync_NotConnectedIsNotRecorded(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		wantErr string
	}{
		{"no Hardcover token", "", "Hardcover isn't connected"},
		{"no Audible login", "tok", "Audible isn't connected"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestApp(t, func(c *config.Config) { c.HardcoverAPIToken = tt.token })
			_, err := a.Sync(context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !errors.Is(err, ErrNotConnected) {
				t.Fatalf("Sync() = %v, want ErrNotConnected %q", err, tt.wantErr)
			}
			if last, _ := a.LastSync(); last != nil {
				t.Errorf("LastSync() = %+v, want nothing recorded before setup", last)
			}
		})
	}
}

func TestSyncAndResetRefuseWhileRunning(t *testing.T) {
	a := newTestApp(t, nil)
	a.running.Lock()
	defer a.running.Unlock()
	if _, err := a.Sync(context.Background()); !errors.Is(err, ErrSyncRunning) {
		t.Errorf("Sync() = %v, want ErrSyncRunning", err)
	}
	if err := a.ResetSync(); !errors.Is(err, ErrSyncRunning) {
		t.Errorf("ResetSync() = %v, want ErrSyncRunning", err)
	}
}

func TestAudibleStatusAndLogout(t *testing.T) {
	a := newTestApp(t, nil)
	if st := a.AudibleStatus(); st.Connected || st.Error != "" {
		t.Errorf("status with no login = %+v, want not connected, no error", st)
	}
	if err := os.WriteFile(a.cfg.CredentialsPath, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if st := a.AudibleStatus(); st.Connected || st.Error == "" {
		t.Errorf("status with broken login = %+v, want an error", st)
	}
	if err := a.LogoutAudible(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.cfg.CredentialsPath); !errors.Is(err, os.ErrNotExist) {
		t.Error("login file not deleted on logout")
	}
	if err := a.FinishAudibleLogin(context.Background(), "https://example.com"); err == nil {
		t.Error("finish without start should fail")
	}
}

func TestNotifyTargets(t *testing.T) {
	a := newTestApp(t, nil)
	if err := a.AddNotifyURL("discord://secret@123"); err != nil {
		t.Fatalf("add discord URL: %v", err)
	}
	if err := a.AddNotifyURL("logger://"); err != nil {
		t.Fatalf("add valid URL: %v", err)
	}
	if err := a.AddNotifyURL("logger://"); err == nil {
		t.Error("duplicate URL accepted")
	}
	if err := a.AddNotifyURL("invalid://x"); err == nil {
		t.Error("invalid URL accepted")
	}
	targets, err := a.NotifyTargets()
	if err != nil || len(targets) != 2 {
		t.Fatalf("NotifyTargets() = %+v, %v", targets, err)
	}
	if strings.Contains(targets[0].Masked, "secret") {
		t.Errorf("targets = %+v", targets)
	}
	if err := a.TestNotifyTarget(1); err != nil {
		t.Errorf("test logger target: %v", err)
	}
	if err := a.RemoveNotifyURL(5); err == nil {
		t.Error("removed a missing URL")
	}
	if err := a.RemoveNotifyURL(1); err != nil {
		t.Errorf("remove saved URL: %v", err)
	}
	if targets, _ := a.NotifyTargets(); len(targets) != 1 {
		t.Errorf("after remove: %+v", targets)
	}
}

func TestPreferences(t *testing.T) {
	a := newTestApp(t, nil)
	p, err := a.Preferences()
	if err != nil || p.SyncInterval != "6h0m0s" || p.MatchConfidence != "strict" || p.PublicURL != "" {
		t.Fatalf("defaults = %+v, %v", p, err)
	}
	for _, bad := range []state.Preferences{
		{SyncInterval: "7m", MatchConfidence: "strict"},
		{SyncInterval: "30m", MatchConfidence: "0.8"},
		{SyncInterval: "30m", MatchConfidence: "strict", PublicURL: "nas:8080"},
	} {
		if err := a.SavePreferences(bad); err == nil {
			t.Errorf("SavePreferences(%+v) accepted", bad)
		}
	}
	if err := a.SavePreferences(state.Preferences{SyncInterval: "30m", MatchConfidence: "loose", PublicURL: " http://nas:8084/ "}); err != nil {
		t.Fatalf("save: %v", err)
	}
	select {
	case <-a.PreferencesChanged():
	default:
		t.Error("no change signal after save")
	}
	p, _ = a.Preferences()
	if a.SyncInterval() != 30*time.Minute || p.MatchConfidence != "loose" || p.PublicURL != "http://nas:8084" {
		t.Errorf("after save = %+v, interval %v", p, a.SyncInterval())
	}
}
