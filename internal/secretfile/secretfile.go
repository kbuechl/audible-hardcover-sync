// Package secretfile reads and writes small secret files (the Audible login,
// the Hardcover token). With a key they are encrypted with AES-256-GCM
// under an scrypt-derived key; without one they are stored as-is, with
// owner-only permissions.
package secretfile

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/scrypt"
)

// encryptedMagic starts every encrypted file, so it can be told apart from
// a plain one.
var encryptedMagic = []byte("AHS-ENC-v1\n")

const (
	saltSize = 16
	keySize  = 32 // AES-256
	// scrypt cost parameters (the package's recommended interactive values).
	scryptN = 1 << 15
	scryptR = 8
	scryptP = 1
)

// ErrKeyRequired means the file is encrypted but no key was configured.
var ErrKeyRequired = errors.New("file is encrypted; set CREDENTIALS_KEY")

// Read returns the contents of the secret file at path, decrypting it with
// key when it is encrypted. A plain file read while a key is set is
// rewritten encrypted, so turning on encryption needs no other step.
func Read(path, key string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !isEncrypted(data) {
		if key != "" {
			if err := Write(path, key, data); err != nil {
				return nil, fmt.Errorf("encrypt existing %s: %w", filepath.Base(path), err)
			}
		}
		return data, nil
	}
	if key == "" {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), ErrKeyRequired)
	}
	return openSealed(data, key)
}

// Write stores data at path with owner-only permissions, encrypted when key
// is set. It writes a temp file and renames it, so a crash can't leave a
// half-written file in place of a working one.
func Write(path, key string, data []byte) error {
	if key != "" {
		sealed, err := seal(data, key)
		if err != nil {
			return err
		}
		data = sealed
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace %s: %w", filepath.Base(path), err)
	}
	return nil
}

// isEncrypted reports whether data is an encrypted secret file.
func isEncrypted(data []byte) bool {
	return bytes.HasPrefix(data, encryptedMagic)
}

// seal encrypts plaintext with AES-256-GCM under a key derived
// from passphrase with scrypt. Layout: magic | salt | nonce | ciphertext.
func seal(plaintext []byte, passphrase string) ([]byte, error) {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}
	aead, err := newAEAD(passphrase, salt)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	out := make([]byte, 0, len(encryptedMagic)+saltSize+len(nonce)+len(plaintext)+aead.Overhead())
	out = append(out, encryptedMagic...)
	out = append(out, salt...)
	out = append(out, nonce...)
	// The magic and salt are authenticated too, so tampering with them fails.
	return aead.Seal(out, nonce, plaintext, out[:len(encryptedMagic)+saltSize]), nil
}

// openSealed reverses seal.
func openSealed(data []byte, passphrase string) ([]byte, error) {
	if !isEncrypted(data) {
		return nil, errors.New("not an encrypted file")
	}
	header := len(encryptedMagic) + saltSize
	if len(data) < header {
		return nil, errors.New("encrypted file is truncated")
	}
	aead, err := newAEAD(passphrase, data[len(encryptedMagic):header])
	if err != nil {
		return nil, err
	}
	if len(data) < header+aead.NonceSize() {
		return nil, errors.New("encrypted file is truncated")
	}
	nonce := data[header : header+aead.NonceSize()]
	plaintext, err := aead.Open(nil, nonce, data[header+aead.NonceSize():], data[:header])
	if err != nil {
		return nil, errors.New("decrypt: wrong CREDENTIALS_KEY or corrupted file")
	}
	return plaintext, nil
}

func newAEAD(passphrase string, salt []byte) (cipher.AEAD, error) {
	key, err := scrypt.Key([]byte(passphrase), salt, scryptN, scryptR, scryptP, keySize)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	return cipher.NewGCM(block)
}
