package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strings"

	audiblesdk "github.com/mstrhakr/go-audible"

	"github.com/kbuechl/audible-hardcover-sync/internal/hardcover"
	"github.com/kbuechl/audible-hardcover-sync/internal/secretfile"
)

// ErrTokenFromEnv is returned when the web UI tries to change a Hardcover
// token that is set by the HARDCOVER_API_TOKEN environment variable.
var ErrTokenFromEnv = errors.New("the Hardcover token is set by HARDCOVER_API_TOKEN; change it there")

// AudibleStatus says whether the Audible login works. It never includes
// the login itself.
type AudibleStatus struct {
	Connected bool   `json:"connected"`
	Error     string `json:"error,omitempty"`
}

// HardcoverStatus says whether a Hardcover token is set and whose account
// it is. It never includes the token.
type HardcoverStatus struct {
	Connected bool   `json:"connected"`
	Username  string `json:"username,omitempty"`
	FromEnv   bool   `json:"fromEnv"`
	Error     string `json:"error,omitempty"`
}

// AudibleStatus checks the saved Audible login can be read.
func (a *App) AudibleStatus() AudibleStatus {
	if _, err := a.loadAudible(); err != nil {
		if errors.Is(err, ErrNotConnected) {
			return AudibleStatus{}
		}
		return AudibleStatus{Error: err.Error()}
	}
	return AudibleStatus{Connected: true}
}

// StartAudibleLogin returns the Amazon sign-in URL for a new login. The
// matching FinishAudibleLogin must come from the same App.
func (a *App) StartAudibleLogin() (string, error) {
	authURL, err := a.newAudibleClient().GetAuthURL()
	if err != nil {
		return "", fmt.Errorf("get Audible sign-in URL: %w", err)
	}
	a.mu.Lock()
	a.pending = authURL
	a.mu.Unlock()
	return authURL.URL, nil
}

// FinishAudibleLogin completes the login started by StartAudibleLogin with
// the URL the browser landed on after signing in, and saves the login,
// replacing any previous one.
func (a *App) FinishAudibleLogin(ctx context.Context, redirectURL string) error {
	a.mu.Lock()
	pending := a.pending
	a.mu.Unlock()
	if pending == nil {
		return errors.New("start the Audible sign-in first")
	}
	code, err := audiblesdk.HandleAuthRedirect(strings.TrimSpace(redirectURL))
	if err != nil {
		return fmt.Errorf("that doesn't look like the page Amazon sent you to: %w", err)
	}
	c := a.newAudibleClient()
	if err := c.Authenticate(ctx, audiblesdk.DeviceRegistrationRequest{
		AuthorizationCode: code,
		CodeVerifier:      pending.CodeVerifier,
		DeviceSerial:      pending.DeviceSerial,
	}); err != nil {
		return fmt.Errorf("Audible sign-in failed: %w", err)
	}
	if err := c.SaveCredentialsFile(a.cfg.CredentialsPath); err != nil {
		return fmt.Errorf("save Audible login: %w", err)
	}
	a.mu.Lock()
	a.pending = nil
	a.mu.Unlock()
	slog.Info("audible connected")
	return nil
}

// LogoutAudible removes this device from the Audible account (best effort)
// and deletes the saved login.
func (a *App) LogoutAudible(ctx context.Context) error {
	if c, err := a.loadAudible(); err == nil {
		if err := c.Deregister(ctx); err != nil {
			slog.Warn("deregister audible device; deleting the login anyway", "error", err)
		}
	}
	if err := os.Remove(a.cfg.CredentialsPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("delete Audible login: %w", err)
	}
	slog.Info("audible logged out")
	return nil
}

// HardcoverStatus reports the Hardcover connection, looking up the account
// name once per token.
func (a *App) HardcoverStatus(ctx context.Context) HardcoverStatus {
	a.mu.Lock()
	hc, username := a.hc, a.hcUsername
	a.mu.Unlock()
	st := HardcoverStatus{FromEnv: a.cfg.HardcoverAPIToken != ""}
	if hc == nil {
		return st
	}
	if username == "" {
		_, name, err := hc.Me(ctx)
		if err != nil {
			st.Error = err.Error()
			return st
		}
		a.mu.Lock()
		if a.hc == hc {
			a.hcUsername = name
		}
		a.mu.Unlock()
		username = name
	}
	st.Connected, st.Username = true, username
	return st
}

// SetHardcoverToken checks token against Hardcover and, if it works, saves
// it and switches to it. It returns the account's username.
func (a *App) SetHardcoverToken(ctx context.Context, token string) (string, error) {
	if a.cfg.HardcoverAPIToken != "" {
		return "", ErrTokenFromEnv
	}
	token = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(token), "Bearer "))
	if token == "" {
		return "", errors.New("paste a Hardcover API token")
	}
	hc := hardcover.New(token)
	_, username, err := hc.Me(ctx)
	if err != nil {
		return "", fmt.Errorf("Hardcover didn't accept that token: %w", err)
	}
	if err := secretfile.Write(a.cfg.HardcoverTokenPath, a.cfg.CredentialsKey, []byte(token)); err != nil {
		return "", fmt.Errorf("save Hardcover token: %w", err)
	}
	a.mu.Lock()
	a.hc, a.hcUsername = hc, username
	a.mu.Unlock()
	slog.Info("hardcover token updated", "username", username)
	return username, nil
}

// hardcoverToken returns the token from the environment, else the one
// saved from the web UI, else "".
func (a *App) hardcoverToken() (string, error) {
	if a.cfg.HardcoverAPIToken != "" {
		return a.cfg.HardcoverAPIToken, nil
	}
	data, err := secretfile.Read(a.cfg.HardcoverTokenPath, a.cfg.CredentialsKey)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read saved Hardcover token: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}
