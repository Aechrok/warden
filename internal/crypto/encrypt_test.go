package crypto

import (
	"bytes"
	"crypto/rand"
	"errors"
	"testing"
)

func mustKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatalf("rand key: %v", err)
	}
	return k
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := mustKey(t)
	cases := [][]byte{
		[]byte(""),
		[]byte("hello"),
		bytes.Repeat([]byte("warden-secrets-"), 200),
	}
	for _, pt := range cases {
		ct, err := Encrypt(key, pt)
		if err != nil {
			t.Fatalf("Encrypt: %v", err)
		}
		got, err := Decrypt(key, ct)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if !bytes.Equal(got, pt) {
			t.Fatalf("roundtrip mismatch: got %q want %q", got, pt)
		}
	}
}

func TestEncryptDifferentNonces(t *testing.T) {
	key := mustKey(t)
	a, err := Encrypt(key, []byte("same"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Encrypt(key, []byte("same"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a, b) {
		t.Fatalf("encryption must use random nonce; got identical ciphertexts")
	}
}

func TestDecryptRejectsTamper(t *testing.T) {
	key := mustKey(t)
	ct, err := Encrypt(key, []byte("integrity"))
	if err != nil {
		t.Fatal(err)
	}
	ct[len(ct)-1] ^= 0x01
	if _, err := Decrypt(key, ct); err == nil {
		t.Fatalf("expected error on tampered ciphertext")
	}
}

func TestKeyLength(t *testing.T) {
	if _, err := Encrypt(make([]byte, 16), []byte("x")); !errors.Is(err, ErrKeyLength) {
		t.Fatalf("expected ErrKeyLength, got %v", err)
	}
	if _, err := Decrypt(make([]byte, 16), []byte("x")); !errors.Is(err, ErrKeyLength) {
		t.Fatalf("expected ErrKeyLength, got %v", err)
	}
}

func TestDecryptShortCiphertext(t *testing.T) {
	key := mustKey(t)
	if _, err := Decrypt(key, []byte{0x00}); !errors.Is(err, ErrCiphertextTooShort) {
		t.Fatalf("expected ErrCiphertextTooShort, got %v", err)
	}
}
