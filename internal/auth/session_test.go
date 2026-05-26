package auth

import (
	"strings"
	"testing"
)

func TestGenerateTokenLength(t *testing.T) {
	tok, err := generateToken(SessionTokenBytes)
	if err != nil {
		t.Fatalf("generateToken: %v", err)
	}
	if tok == "" {
		t.Fatal("expected non-empty token")
	}
	// base64 URL no padding: 32 bytes → 43 characters.
	if len(tok) != 43 {
		t.Errorf("expected 43 chars, got %d (%q)", len(tok), tok)
	}
	if strings.ContainsAny(tok, "+/=") {
		t.Errorf("URL-safe base64 should not contain +/= : %q", tok)
	}
}

func TestHashTokenStable(t *testing.T) {
	got := hashToken("hello")
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Errorf("hashToken(hello) = %q, want %q", got, want)
	}
}

func TestHashTokenDifferentInputs(t *testing.T) {
	a := hashToken("a")
	b := hashToken("b")
	if a == b {
		t.Error("different inputs must produce different hashes")
	}
}

func TestValidateState(t *testing.T) {
	if !ValidateState("abc123", "abc123") {
		t.Error("matching states should validate")
	}
	if ValidateState("abc123", "abc124") {
		t.Error("mismatched states should not validate")
	}
	if ValidateState("", "abc") {
		t.Error("empty expected should not validate")
	}
	if ValidateState("abc", "") {
		t.Error("empty actual should not validate")
	}
	if ValidateState("abc", "abcd") {
		t.Error("length-mismatched should not validate")
	}
}
