package audible

import (
	"errors"
	"path/filepath"
	"testing"

	audiblesdk "github.com/mstrhakr/go-audible"

	"github.com/kbuechl/audible-hardcover-sync/internal/secretfile"
)

func TestCredentialsFile_EncryptedRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	saver := New(audiblesdk.MarketplaceUS)
	saver.SetCredentialsKey("k")
	saver.sdk.SetCredentials(&audiblesdk.Credentials{})
	if err := saver.SaveCredentialsFile(path); err != nil {
		t.Fatal(err)
	}

	loader := New(audiblesdk.MarketplaceUS)
	loader.SetCredentialsKey("k")
	if err := loader.LoadCredentialsFile(path); err != nil {
		t.Errorf("load with key: %v", err)
	}
	if err := New(audiblesdk.MarketplaceUS).LoadCredentialsFile(path); !errors.Is(err, secretfile.ErrKeyRequired) {
		t.Errorf("load without key = %v, want ErrKeyRequired", err)
	}
}
