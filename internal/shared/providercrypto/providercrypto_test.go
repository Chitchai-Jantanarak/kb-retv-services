package providercrypto

import (
	"bytes"
	"encoding/base64"
	"testing"
)

const (
	interopPlaintext = "sk-interop-ABC-1234567890"
	interopFromPHP   = "CVGaOrW8T4fHEXdWxoZtF9dN3bPYnMen2vftNBBWfnoPSyG+k8clZOxzyEz4J0cXL9ojIb4="
)

func testKey() []byte { return bytes.Repeat([]byte{0x42}, keySize) }

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := testKey()
	for _, pt := range []string{"", "sk-abc123XYZ", "ключ-京-secret"} {
		ct, err := Encrypt(pt, key)
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", pt, err)
		}
		got, err := Decrypt(ct, key)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if got != pt {
			t.Fatalf("round trip = %q, want %q", got, pt)
		}
	}
}

func TestEncryptUsesRandomNonce(t *testing.T) {
	key := testKey()
	a, _ := Encrypt("same", key)
	b, _ := Encrypt("same", key)
	if a == b {
		t.Fatal("expected distinct ciphertexts from random nonce")
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	ct, _ := Encrypt("secret", testKey())
	if _, err := Decrypt(ct, bytes.Repeat([]byte{0x99}, keySize)); err == nil {
		t.Fatal("expected failure with wrong key")
	}
}

func TestDecryptTamperedFails(t *testing.T) {
	ct, _ := Encrypt("secret", testKey())
	raw, _ := base64.StdEncoding.DecodeString(ct)
	raw[len(raw)-1] ^= 0xFF
	if _, err := Decrypt(base64.StdEncoding.EncodeToString(raw), testKey()); err == nil {
		t.Fatal("expected failure on tampered ciphertext")
	}
}

func TestParseKeyValidation(t *testing.T) {
	if _, err := ParseKey(base64.StdEncoding.EncodeToString(testKey())); err != nil {
		t.Fatalf("valid key: %v", err)
	}
	if _, err := ParseKey(base64.StdEncoding.EncodeToString([]byte("short"))); err == nil {
		t.Fatal("expected error for short key")
	}
	if _, err := ParseKey("!!!notbase64!!!"); err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestDecryptInteropFromPHP(t *testing.T) {
	got, err := Decrypt(interopFromPHP, testKey())
	if err != nil {
		t.Fatalf("decrypt PHP vector: %v", err)
	}
	if got != interopPlaintext {
		t.Fatalf("interop = %q, want %q", got, interopPlaintext)
	}
}
