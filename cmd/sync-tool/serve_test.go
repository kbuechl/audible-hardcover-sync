package main

import (
	"context"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kbuechl/audible-hardcover-sync/internal/config"
)

func TestRunServe_MountsWebUIAndShutsDownOnContextCancel(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{
		HardcoverAPIToken: "unused-in-this-test",
		StateDBPath:       filepath.Join(dir, "state.db"),
		CredentialsPath:   filepath.Join(dir, "credentials.json"),
		WebUIPort:         "18080",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- runServe(ctx, cfg) }()

	// Give the HTTP server a moment to start.
	var resp *http.Response
	var err error
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		resp, err = http.Get("http://localhost:18080/api/unmatched")
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET /api/unmatched error = %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	if err := <-errCh; err != nil && err != context.DeadlineExceeded {
		t.Errorf("runServe() returned unexpected error = %v", err)
	}
}

func TestSchedule_RunsAndPicksUpNewInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	interval := time.Hour
	var mu sync.Mutex
	get := func() time.Duration { mu.Lock(); defer mu.Unlock(); return interval }
	changed := make(chan struct{}, 1)
	ran := make(chan struct{}, 10)
	go schedule(ctx, get, changed, func() { ran <- struct{}{} })

	select {
	case <-ran:
		t.Fatal("ran before the hour-long interval")
	case <-time.After(50 * time.Millisecond):
	}
	mu.Lock()
	interval = 10 * time.Millisecond
	mu.Unlock()
	changed <- struct{}{}
	select {
	case <-ran:
	case <-time.After(time.Second):
		t.Fatal("new interval not picked up")
	}
}
