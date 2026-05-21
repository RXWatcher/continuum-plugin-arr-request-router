package crypto_test

import (
	"strings"
	"testing"

	"github.com/RXWatcher/continuum-plugin-arr-request-router/internal/crypto"
)

func TestSealOpenRoundTrip(t *testing.T) {
	key := strings.Repeat("k", 32)
	plain := "super-secret-api-key"
	sealed, err := crypto.Seal(key, plain)
	if err != nil { t.Fatal(err) }
	if sealed == plain { t.Fatal("seal returned plaintext") }
	got, err := crypto.Open(key, sealed)
	if err != nil { t.Fatal(err) }
	if got != plain { t.Fatalf("got %q want %q", got, plain) }
}

func TestSealUsesFreshNonce(t *testing.T) {
	key := strings.Repeat("k", 32)
	a, err := crypto.Seal(key, "x")
	if err != nil { t.Fatal(err) }
	b, err := crypto.Seal(key, "x")
	if err != nil { t.Fatal(err) }
	if a == b { t.Fatal("seal must use fresh nonce per call") }
}

func TestOpenWrongKey(t *testing.T) {
	a, err := crypto.Seal(strings.Repeat("k", 32), "x")
	if err != nil { t.Fatal(err) }
	if _, err := crypto.Open(strings.Repeat("z", 32), a); err == nil {
		t.Fatal("expected error from wrong key")
	}
}

func TestOpenTamperedCiphertextFails(t *testing.T) {
	key := strings.Repeat("k", 32)
	sealed, err := crypto.Seal(key, "x")
	if err != nil { t.Fatal(err) }
	// flip a couple of base64 chars near the end (the tag region)
	tampered := sealed[:len(sealed)-2] + "AA"
	if tampered == sealed {
		t.Fatal("tamper attempt produced identical string; pick different chars")
	}
	if _, err := crypto.Open(key, tampered); err == nil {
		t.Fatal("expected error from tampered ciphertext")
	}
}

func TestOpenShortInputFails(t *testing.T) {
	if _, err := crypto.Open(strings.Repeat("k", 32), "AAAA"); err == nil {
		t.Fatal("expected error for ciphertext shorter than nonce")
	}
}

func TestOpenInvalidBase64Fails(t *testing.T) {
	if _, err := crypto.Open(strings.Repeat("k", 32), "not!base64!"); err == nil {
		t.Fatal("expected error from invalid base64")
	}
}

func TestSealKeyOfAnyLength(t *testing.T) {
	// Key is hashed to 32 bytes, so any non-empty string should work.
	for _, k := range []string{"a", "longer key", strings.Repeat("x", 200)} {
		sealed, err := crypto.Seal(k, "v")
		if err != nil { t.Fatalf("Seal(%q): %v", k, err) }
		got, err := crypto.Open(k, sealed)
		if err != nil { t.Fatalf("Open(%q): %v", k, err) }
		if got != "v" { t.Fatalf("got %q", got) }
	}
}
