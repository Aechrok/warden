// Package crypto provides AES-256-GCM encryption helpers used by Warden to
// protect integration credentials at rest. The key material is supplied by
// configuration (ENCRYPTION_KEY) and is never persisted by this package.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// ErrKeyLength is returned when an encryption key is not exactly 32 bytes.
var ErrKeyLength = errors.New("crypto: key must be 32 bytes for AES-256-GCM")

// ErrCiphertextTooShort is returned when a ciphertext is shorter than the
// expected nonce prefix.
var ErrCiphertextTooShort = errors.New("crypto: ciphertext too short")

// Encrypt seals plaintext under key with AES-256-GCM. The 12-byte random
// nonce is prepended to the returned ciphertext so that Decrypt can recover
// it without a side channel.
func Encrypt(key, plaintext []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, ErrKeyLength
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: read nonce: %w", err)
	}
	ct := gcm.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, len(nonce)+len(ct))
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

// Decrypt opens a ciphertext previously produced by Encrypt (nonce || ct).
// Returns ErrCiphertextTooShort or an authentication failure error if the
// input is malformed or tampered with.
func Decrypt(key, ciphertext []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, ErrKeyLength
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}
	ns := gcm.NonceSize()
	if len(ciphertext) < ns {
		return nil, ErrCiphertextTooShort
	}
	nonce, ct := ciphertext[:ns], ciphertext[ns:]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: open: %w", err)
	}
	return pt, nil
}
