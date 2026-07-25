package audible

import (
	"path/filepath"
	"testing"

	audiblesdk "github.com/mstrhakr/go-audible"
)

func TestToLibraryItem(t *testing.T) {
	book := audiblesdk.Book{
		ASIN:            "B00WRHY15Y",
		ISBN13:          "",
		Title:           "A Court of Thorns and Roses",
		Authors:         []audiblesdk.Contributor{{Name: "Sarah J. Maas"}},
		IsFinished:      true,
		PercentComplete: 100,
	}

	item := toLibraryItem(book)

	if item.ASIN != "B00WRHY15Y" {
		t.Errorf("ASIN = %q", item.ASIN)
	}
	if item.Author != "Sarah J. Maas" {
		t.Errorf("Author = %q, want %q", item.Author, "Sarah J. Maas")
	}
	if !item.IsFinished {
		t.Error("IsFinished = false, want true")
	}
	if item.PercentComplete != 100 {
		t.Errorf("PercentComplete = %v, want 100", item.PercentComplete)
	}
}

func TestToLibraryItem_NoAuthor(t *testing.T) {
	item := toLibraryItem(audiblesdk.Book{ASIN: "B000000000"})
	if item.Author != "" {
		t.Errorf("Author = %q, want empty when no authors present", item.Author)
	}
}

func TestGetAuthURL(t *testing.T) {
	client := New(audiblesdk.MarketplaceUS)

	authURL, err := client.GetAuthURL()
	if err != nil {
		t.Fatalf("GetAuthURL() returned error: %v", err)
	}
	if authURL.URL == "" {
		t.Error("authURL.URL is empty, want non-empty")
	}
	if authURL.CodeVerifier == "" {
		t.Error("authURL.CodeVerifier is empty, want non-empty")
	}
	if authURL.DeviceSerial == "" {
		t.Error("authURL.DeviceSerial is empty, want non-empty")
	}
}

func TestIsAuthenticated_FreshClient(t *testing.T) {
	client := New(audiblesdk.MarketplaceUS)

	if client.IsAuthenticated() {
		t.Error("IsAuthenticated() = true for freshly-constructed client, want false")
	}
}

func TestCredentialsFile_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")

	// A freshly-constructed client has nil (not zero-value) SDK credentials,
	// and the vendored SDK's MarshalCredentials errors on nil. Seed a
	// zero-value Credentials struct via the SDK's own exported
	// SetCredentials (reachable through the wrapper's same-package sdk
	// field) so this test exercises the file I/O and JSON marshal/unmarshal
	// round-trip through this wrapper's methods, not real Audible auth
	// state — which is the correct scope for this wrapper.
	saver := New(audiblesdk.MarketplaceUS)
	saver.sdk.SetCredentials(&audiblesdk.Credentials{})
	if err := saver.SaveCredentialsFile(path); err != nil {
		t.Fatalf("SaveCredentialsFile() returned error: %v", err)
	}

	loader := New(audiblesdk.MarketplaceUS)
	if err := loader.LoadCredentialsFile(path); err != nil {
		t.Fatalf("LoadCredentialsFile() returned error: %v", err)
	}
}

func TestLoadCredentialsFile_MissingFile(t *testing.T) {
	client := New(audiblesdk.MarketplaceUS)

	path := filepath.Join(t.TempDir(), "does-not-exist.json")
	if err := client.LoadCredentialsFile(path); err == nil {
		t.Error("LoadCredentialsFile() with missing file returned nil error, want non-nil")
	}
}

func TestResponseGroups_ExtendsDefaults(t *testing.T) {
	for _, want := range []string{"listening_status", "percent_complete", "is_finished"} {
		found := false
		for _, g := range responseGroups {
			if g == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("responseGroups missing %q; got %v", want, responseGroups)
		}
	}
	for _, want := range audiblesdk.DefaultResponseGroups {
		found := false
		for _, g := range responseGroups {
			if g == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("responseGroups dropped default group %q", want)
		}
	}
}
