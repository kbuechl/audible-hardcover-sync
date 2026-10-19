package audible

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"

	"golang.org/x/crypto/scrypt"
)

// encryptedMagic starts every encrypted credentials file, so a file can be
// told apart from the plain JSON written when no key is configured.
var encryptedMagic = []byte("AHS-ENC-v1\n")

const (
	saltSize = 16
	keySize  = 32 // AES-256
	// scrypt cost parameters (the package's recommended interactive values).
	scryptN = 1 << 15
	scryptR = 8
	scryptP = 1
)

// ErrCredentialsKeyRequired means the credentials file is encrypted but no
// key was configured.
var ErrCredentialsKeyRequired = errors.New("credentials file is encrypted; set CREDENTIALS_KEY")

// isEncrypted reports whether data is an encrypted credentials file.
func isEncrypted(data []byte) bool {
	return bytes.HasPrefix(data, encryptedMagic)
}

// sealCredentials encrypts plaintext with AES-256-GCM under a key derived
// from passphrase with scrypt. Layout: magic | salt | nonce | ciphertext.
func sealCredentials(plaintext []byte, passphrase string) ([]byte, error) {
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

// openCredentials reverses sealCredentials.
func openCredentials(data []byte, passphrase string) ([]byte, error) {
	if !isEncrypted(data) {
		return nil, errors.New("not an encrypted credentials file")
	}
	header := len(encryptedMagic) + saltSize
	if len(data) < header {
		return nil, errors.New("encrypted credentials file is truncated")
	}
	aead, err := newAEAD(passphrase, data[len(encryptedMagic):header])
	if err != nil {
		return nil, err
	}
	if len(data) < header+aead.NonceSize() {
		return nil, errors.New("encrypted credentials file is truncated")
	}
	nonce := data[header : header+aead.NonceSize()]
	plaintext, err := aead.Open(nil, nonce, data[header+aead.NonceSize():], data[:header])
	if err != nil {
		return nil, errors.New("decrypt credentials: wrong CREDENTIALS_KEY or corrupted file")
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
