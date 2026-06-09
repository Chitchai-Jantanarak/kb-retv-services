package crypto

import "testing"

func TestAESStringRoundTrip(t *testing.T) {
	encrypted, err := EncryptAES("secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if encrypted == nil || *encrypted == "" {
		t.Fatal("expected encrypted text")
	}

	decrypted, err := DecryptAES(*encrypted)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if decrypted == nil || *decrypted != "secret" {
		t.Fatalf("expected decrypted secret, got %#v", decrypted)
	}
}

func TestAESByteRoundTrip(t *testing.T) {
	encrypted, err := EncryptAESByte("secret")
	if err != nil {
		t.Fatalf("encrypt bytes: %v", err)
	}
	if len(encrypted) == 0 {
		t.Fatal("expected encrypted bytes")
	}

	decrypted, err := DecryptAESByte(encrypted)
	if err != nil {
		t.Fatalf("decrypt bytes: %v", err)
	}
	if decrypted == nil || *decrypted != "secret" {
		t.Fatalf("expected decrypted secret, got %#v", decrypted)
	}
}
