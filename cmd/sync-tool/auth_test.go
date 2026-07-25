package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	audiblesdk "github.com/mstrhakr/go-audible"
)

type fakeAuthClient struct {
	authURL         *audiblesdk.AuthURL
	authURLErr      error
	authenticated   bool
	authenticateErr error
	savedPath       string
	saveErr         error
}

func (f *fakeAuthClient) GetAuthURL() (*audiblesdk.AuthURL, error) {
	if f.authURLErr != nil {
		return nil, f.authURLErr
	}
	return f.authURL, nil
}

func (f *fakeAuthClient) Authenticate(ctx context.Context, req audiblesdk.DeviceRegistrationRequest) error {
	if f.authenticateErr != nil {
		return f.authenticateErr
	}
	f.authenticated = true
	return nil
}

func (f *fakeAuthClient) SaveCredentialsFile(path string) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.savedPath = path
	return nil
}

func TestRunAuth_HappyPath(t *testing.T) {
	client := &fakeAuthClient{authURL: &audiblesdk.AuthURL{
		URL:          "https://amazon.com/ap/signin?...",
		CodeVerifier: "verifier123",
		DeviceSerial: "serial123",
	}}
	stdin := strings.NewReader("https://amazon.com/ap/maplanding?openid.oa2.authorization_code=ABC123&other=1\n")
	var stdout bytes.Buffer
	credentialsPath := filepath.Join(t.TempDir(), "credentials.json")

	err := runAuth(context.Background(), client, credentialsPath, stdin, &stdout)
	if err != nil {
		t.Fatalf("runAuth() error = %v", err)
	}
	if !client.authenticated {
		t.Error("expected Authenticate to be called")
	}
	if client.savedPath != credentialsPath {
		t.Errorf("savedPath = %q, want %q", client.savedPath, credentialsPath)
	}
	if !strings.Contains(stdout.String(), client.authURL.URL) {
		t.Error("expected the sign-in URL to be printed to stdout")
	}
}

func TestRunAuth_Failures(t *testing.T) {
	validAuthURL := &audiblesdk.AuthURL{
		URL:          "https://amazon.com/ap/signin?...",
		CodeVerifier: "verifier123",
		DeviceSerial: "serial123",
	}

	tests := []struct {
		name        string
		client      *fakeAuthClient
		stdin       string
		wantErrText string
	}{
		{
			name:        "GetAuthURL error",
			client:      &fakeAuthClient{authURLErr: errors.New("network down")},
			stdin:       "https://amazon.com/ap/maplanding?openid.oa2.authorization_code=ABC123\n",
			wantErrText: "get auth url",
		},
		{
			name:        "no redirect URL provided (empty stdin)",
			client:      &fakeAuthClient{authURL: validAuthURL},
			stdin:       "",
			wantErrText: "no redirect URL provided",
		},
		{
			name:        "redirect URL missing authorization code",
			client:      &fakeAuthClient{authURL: validAuthURL},
			stdin:       "https://amazon.com/ap/maplanding?other=1\n",
			wantErrText: "parse redirect url",
		},
		{
			name:        "Authenticate error",
			client:      &fakeAuthClient{authURL: validAuthURL, authenticateErr: errors.New("bad code")},
			stdin:       "https://amazon.com/ap/maplanding?openid.oa2.authorization_code=ABC123\n",
			wantErrText: "authenticate",
		},
		{
			name:        "SaveCredentialsFile error",
			client:      &fakeAuthClient{authURL: validAuthURL, saveErr: errors.New("disk full")},
			stdin:       "https://amazon.com/ap/maplanding?openid.oa2.authorization_code=ABC123\n",
			wantErrText: "save credentials",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			credentialsPath := filepath.Join(t.TempDir(), "credentials.json")

			err := runAuth(context.Background(), tt.client, credentialsPath, strings.NewReader(tt.stdin), &stdout)
			if err == nil {
				t.Fatalf("runAuth() error = nil, want error containing %q", tt.wantErrText)
			}
			if !strings.Contains(err.Error(), tt.wantErrText) {
				t.Errorf("runAuth() error = %q, want it to contain %q", err.Error(), tt.wantErrText)
			}
		})
	}
}
