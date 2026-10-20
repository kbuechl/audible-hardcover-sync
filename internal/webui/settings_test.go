package webui

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kbuechl/audible-hardcover-sync/internal/app"
	"github.com/kbuechl/audible-hardcover-sync/internal/hardcover"
	"github.com/kbuechl/audible-hardcover-sync/internal/state"
)

type fakeService struct {
	syncing    bool
	tokenErr   error
	finishErr  error
	resetErr   error
	syncCalled chan struct{}
	targets    []app.NotifyTarget
	testErr    error
	prefsErr   error
}

func (f *fakeService) Preferences() (state.Preferences, error) {
	return state.Preferences{SyncInterval: "6h0m0s", MatchConfidence: "strict"}, nil
}
func (f *fakeService) SavePreferences(state.Preferences) error { return f.prefsErr }

func (f *fakeService) Hardcover() *hardcover.Client { return nil }
func (f *fakeService) AudibleStatus() app.AudibleStatus {
	return app.AudibleStatus{Connected: true}
}
func (f *fakeService) HardcoverStatus(context.Context) app.HardcoverStatus {
	return app.HardcoverStatus{Connected: true, Username: "reader"}
}
func (f *fakeService) StartAudibleLogin() (string, error) {
	return "https://amazon.example/signin", nil
}
func (f *fakeService) FinishAudibleLogin(context.Context, string) error {
	return f.finishErr
}
func (f *fakeService) LogoutAudible(context.Context) error { return nil }
func (f *fakeService) SetHardcoverToken(context.Context, string) (string, error) {
	return "reader", f.tokenErr
}
func (f *fakeService) Sync(context.Context) (state.SyncReport, error) {
	if f.syncCalled != nil {
		close(f.syncCalled)
	}
	return state.SyncReport{}, nil
}
func (f *fakeService) Syncing() bool                              { return f.syncing }
func (f *fakeService) LastSync() (*state.SyncReport, error)       { return &state.SyncReport{Synced: 2}, nil }
func (f *fakeService) ResetSync() error                           { return f.resetErr }
func (f *fakeService) NotifyTargets() ([]app.NotifyTarget, error) { return f.targets, nil }
func (f *fakeService) AddNotifyURL(string) error                  { return nil }
func (f *fakeService) RemoveNotifyURL(int) error                  { return nil }
func (f *fakeService) TestNotifyTarget(int) error                 { return f.testErr }

func serve(t *testing.T, svc *fakeService, method, path, body string) (int, string) {
	t.Helper()
	srv := New(newTestStore(t), &fakeSearcher{}).WithService(svc, context.Background(), func(string) error { return nil })
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	b, _ := io.ReadAll(rec.Body)
	return rec.Code, string(b)
}

func TestSettingsEndpoints(t *testing.T) {
	tests := []struct {
		name         string
		svc          *fakeService
		method, path string
		body         string
		wantCode     int
		wantBody     string
	}{
		{"status", &fakeService{}, "GET", "/api/status", "", 200, `"username":"reader"`},
		{"start audible login", &fakeService{}, "POST", "/api/audible/login", "", 200, `amazon.example`},
		{"finish audible login error", &fakeService{finishErr: errors.New("bad redirect")}, "POST", "/api/audible/login/finish", `{"redirectUrl":"x"}`, 400, `"error":"bad redirect"`},
		{"logout", &fakeService{}, "POST", "/api/audible/logout", "", 204, ""},
		{"set token", &fakeService{}, "PUT", "/api/hardcover/token", `{"token":"t"}`, 200, `"username":"reader"`},
		{"set token from env", &fakeService{tokenErr: app.ErrTokenFromEnv}, "PUT", "/api/hardcover/token", `{"token":"t"}`, 409, `HARDCOVER_API_TOKEN`},
		{"bad json", &fakeService{}, "PUT", "/api/hardcover/token", `{`, 400, `invalid request body`},
		{"sync while running", &fakeService{syncing: true}, "POST", "/api/sync", "", 409, `already running`},
		{"reset while running", &fakeService{resetErr: app.ErrSyncRunning}, "POST", "/api/sync/reset", "", 409, `already running`},
		{"list notifications", &fakeService{targets: []app.NotifyTarget{{ID: 0, Masked: "discord://****@1"}}}, "GET", "/api/notifications", "", 200, `discord://****@1`},
		{"test target failure", &fakeService{testErr: errors.New("401 from discord")}, "POST", "/api/notifications/0/test", "", 502, `401 from discord`},
		{"get preferences", &fakeService{}, "GET", "/api/preferences", "", 200, `"matchConfidence":"strict"`},
		{"save bad preferences", &fakeService{prefsErr: errors.New("not one of the offered choices")}, "PUT", "/api/preferences", `{"syncInterval":"7m"}`, 400, `offered choices`},
		{"bad id", &fakeService{}, "DELETE", "/api/notifications/abc", "", 400, `invalid id`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, body := serve(t, tt.svc, tt.method, tt.path, tt.body)
			if code != tt.wantCode || !strings.Contains(body, tt.wantBody) {
				t.Errorf("%s %s = %d %s, want %d containing %q", tt.method, tt.path, code, body, tt.wantCode, tt.wantBody)
			}
		})
	}
}

func TestSyncNowRunsInBackground(t *testing.T) {
	svc := &fakeService{syncCalled: make(chan struct{})}
	if code, _ := serve(t, svc, "POST", "/api/sync", ""); code != http.StatusAccepted {
		t.Fatalf("code = %d, want 202", code)
	}
	<-svc.syncCalled
}

func TestSearchWithoutHardcoverIs503(t *testing.T) {
	if code, body := serve(t, &fakeService{}, "GET", "/api/search?q=x", ""); code != 503 || !strings.Contains(body, "isn't connected") {
		t.Errorf("search = %d %s, want 503", code, body)
	}
}
