package audible

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	audiblesdk "github.com/mstrhakr/go-audible"
)

func saveTestCredentials(t *testing.T, key string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credentials.json")
	c := New(audiblesdk.MarketplaceUS)
	c.SetCredentialsKey(key)
	c.sdk.SetCredentials(&audiblesdk.Credentials{})
	if err := c.SaveCredentialsFile(path); err != nil {
		t.Fatalf("SaveCredentialsFile() error = %v", err)
	}
	return path
}

func loadWithKey(path, key string) error {
	c := New(audiblesdk.MarketplaceUS)
	c.SetCredentialsKey(key)
	return c.LoadCredentialsFile(path)
}

func TestCredentials_NoKeyStaysPlainJSON(t *testing.T) {
	path := saveTestCredentials(t, "")
	data, _ := os.ReadFile(path)
	if isEncrypted(data) || !json.Valid(data) {
		t.Errorf("without a key the file should be plain JSON, got %q", data)
	}
	if err := loadWithKey(path, ""); err != nil {
		t.Errorf("load plain file without key: %v", err)
	}
	if data2, _ := os.ReadFile(path); !bytes.Equal(data, data2) {
		t.Error("loading without a key rewrote the file")
	}
}

func TestCredentials_KeyEncrypts(t *testing.T) {
	path := saveTestCredentials(t, "correct horse")
	data, _ := os.ReadFile(path)
	if !isEncrypted(data) || json.Valid(data) {
		t.Fatalf("with a key the file should be encrypted, got %q", data)
	}
	if err := loadWithKey(path, "correct horse"); err != nil {
		t.Errorf("load with right key: %v", err)
	}
	if err := loadWithKey(path, "wrong"); err == nil {
		t.Error("load with wrong key succeeded")
	}
	if err := loadWithKey(path, ""); !errors.Is(err, ErrCredentialsKeyRequired) {
		t.Errorf("load without key = %v, want ErrCredentialsKeyRequired", err)
	}
}

func TestCredentials_PlainFileEncryptedWhenKeyAdded(t *testing.T) {
	path := saveTestCredentials(t, "")
	if err := loadWithKey(path, "new key"); err != nil {
		t.Fatalf("load plain file with key: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !isEncrypted(data) {
		t.Fatal("plain file was not encrypted after a key was set")
	}
	if err := loadWithKey(path, "new key"); err != nil {
		t.Errorf("reload after migration: %v", err)
	}
}

func TestOpenCredentials_RejectsTampering(t *testing.T) {
	sealed, err := sealCredentials([]byte(`{"a":1}`), "k")
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{len(encryptedMagic) + 1, len(sealed) - 1} { // salt byte, ciphertext byte
		bad := bytes.Clone(sealed)
		bad[i] ^= 0xff
		if _, err := openCredentials(bad, "k"); err == nil {
			t.Errorf("tampered byte %d accepted", i)
		}
	}
	if _, err := openCredentials(sealed[:len(encryptedMagic)+3], "k"); err == nil {
		t.Error("truncated file accepted")
	}
}
