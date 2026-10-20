package secretfile

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadWrite(t *testing.T) {
	secret := []byte(`{"token":"abc"}`)
	tests := []struct {
		name               string
		writeKey, readKey  string
		wantErr            error // nil, ErrKeyRequired, or errAny
		wantEncryptedAfter bool
	}{
		{"no key stays plain", "", "", nil, false},
		{"key encrypts", "k1", "k1", nil, true},
		{"plain file encrypted when key added", "", "k1", nil, true},
		{"encrypted without key", "k1", "", ErrKeyRequired, true},
		{"wrong key", "k1", "k2", errAny, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "secret")
			if err := Write(path, tt.writeKey, secret); err != nil {
				t.Fatal(err)
			}
			got, err := Read(path, tt.readKey)
			switch {
			case tt.wantErr == nil && (err != nil || !bytes.Equal(got, secret)):
				t.Errorf("Read() = %q, %v; want %q", got, err, secret)
			case tt.wantErr == errAny && err == nil:
				t.Error("Read() succeeded, want error")
			case tt.wantErr == ErrKeyRequired && !errors.Is(err, ErrKeyRequired):
				t.Errorf("Read() error = %v, want ErrKeyRequired", err)
			}
			raw, _ := os.ReadFile(path)
			if isEncrypted(raw) != tt.wantEncryptedAfter {
				t.Errorf("encrypted on disk = %v, want %v", isEncrypted(raw), tt.wantEncryptedAfter)
			}
			if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
				t.Errorf("mode = %v, want 0600", info.Mode().Perm())
			}
		})
	}
}

var errAny = errors.New("any error")

func TestOpenSealed_RejectsTampering(t *testing.T) {
	sealed, err := seal([]byte(`{"a":1}`), "k")
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{len(encryptedMagic) + 1, len(sealed) - 1} { // salt byte, ciphertext byte
		bad := bytes.Clone(sealed)
		bad[i] ^= 0xff
		if _, err := openSealed(bad, "k"); err == nil {
			t.Errorf("tampered byte %d accepted", i)
		}
	}
	if _, err := openSealed(sealed[:len(encryptedMagic)+3], "k"); err == nil {
		t.Error("truncated file accepted")
	}
}
