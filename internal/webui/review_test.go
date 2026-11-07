package webui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kbuechl/audible-hardcover-sync/internal/app"
	"github.com/kbuechl/audible-hardcover-sync/internal/state"
)

func TestReviewEndpoints(t *testing.T) {
	svc := func() *fakeService {
		return &fakeService{reviewItems: sampleReview(), waiting: 3}
	}
	tests := []struct {
		name         string
		svc          *fakeService
		method, path string
		body         string
		wantCode     int
		wantBody     string
	}{
		{"waiting filter", svc(), "GET", "/api/review?filter=waiting", "", 200, `"asin":"A1","title":"Dune","author":"Frank","coverUrl":"c","shared":true,"status":"in_progress","percentComplete":40,"decision":"","canRemove":true`},
		{"synced filter", svc(), "GET", "/api/review?filter=synced", "", 200, `"asin":"A1"`},
		{"not_wanted filter", svc(), "GET", "/api/review?filter=not_wanted", "", 200, `"asin":"A1"`},
		{"empty list is []", &fakeService{}, "GET", "/api/review?filter=waiting", "", 200, `[]`},
		{"bad filter", svc(), "GET", "/api/review?filter=x", "", 400, `filter must be`},
		{"missing filter", svc(), "GET", "/api/review", "", 400, `filter must be`},
		{"decide sync", svc(), "PUT", "/api/review/decision", `{"asins":["A1"],"decision":"sync"}`, 204, ""},
		{"decide not wanted", svc(), "PUT", "/api/review/decision", `{"asins":["A1"],"decision":"not_wanted"}`, 204, ""},
		{"decide undecided", svc(), "PUT", "/api/review/decision", `{"asins":["A1"],"decision":""}`, 204, ""},
		{"bad decision", svc(), "PUT", "/api/review/decision", `{"asins":["A1"],"decision":"maybe"}`, 400, `decision must be`},
		{"empty asins", svc(), "PUT", "/api/review/decision", `{"asins":[],"decision":"sync"}`, 400, `at least one`},
		{"unknown asin", &fakeService{decideErr: state.ErrUnknownASIN}, "PUT", "/api/review/decision", `{"asins":["zz"],"decision":"sync"}`, 400, `unknown ASIN`},
		{"bad body", svc(), "PUT", "/api/review/decision", `{`, 400, `invalid request body`},
		{"status survives waiting error", &fakeService{waitingErr: errors.New("db"), waiting: 5}, "GET", "/api/status", "", 200, `"waiting":0`},
		{"status has waiting", svc(), "GET", "/api/status", "", 200, `"waiting":3`},
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

func TestReviewPassesFilterQueryAndDecision(t *testing.T) {
	svc := &fakeService{}
	serve(t, svc, "GET", "/api/review?filter=synced&q=dune", "")
	if svc.gotFilter != "synced" || svc.gotQ != "dune" {
		t.Errorf("service got filter %q q %q", svc.gotFilter, svc.gotQ)
	}
	serve(t, svc, "PUT", "/api/review/decision", `{"asins":["A1","A2"],"decision":"not_wanted"}`)
	if len(svc.gotASINs) != 2 || svc.gotDecision != state.DecisionNotWanted {
		t.Errorf("service got %v %q", svc.gotASINs, svc.gotDecision)
	}
}

func TestUnmatchedHidesHeldAndNotWanted(t *testing.T) {
	store := newTestStore(t)
	for _, asin := range []string{"U1", "U2"} {
		if err := store.Upsert(state.BookState{ASIN: asin, Title: asin, Status: state.StatusFinished, Unmatched: true}); err != nil {
			t.Fatal(err)
		}
	}
	svc := &fakeService{hidden: map[string]bool{"U2": true}}
	srv := New(store, &fakeSearcher{}).WithService(svc, context.Background(), func(string) error { return nil })
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/unmatched", nil))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, `"U1"`) || strings.Contains(body, `"U2"`) {
		t.Errorf("GET /api/unmatched = %d %s, want U1 only", rec.Code, body)
	}
}

func sampleReview() []app.ReviewItem {
	return []app.ReviewItem{{
		ASIN: "A1", Title: "Dune", Author: "Frank", CoverURL: "c", Shared: true,
		Status: state.StatusInProgress, PercentComplete: 40, CanRemove: true,
	}}
}
