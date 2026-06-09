package servicejwt

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"testing"
	"time"
)

func mintRS256(t *testing.T, key *rsa.PrivateKey, payload map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	claimsB64 := base64.RawURLEncoding.EncodeToString(body)
	signingInput := header + "." + claimsB64
	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func publicKeyPEM(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	raw, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: raw}))
}

func testKey(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key, publicKeyPEM(t, key)
}

func TestVerifyRS256ValidToken(t *testing.T) {
	key, pub := testKey(t)
	now := time.Now().Unix()
	tok := mintRS256(t, key, map[string]any{
		"cid":   42,
		"uid":   7,
		"role":  "supporter",
		"grps":  []string{"support"},
		"perms": []string{"ai:reply:create"},
		"iss":   "laravel",
		"aud":   "centric-rag",
		"use":   "service",
		"iat":   now,
		"nbf":   now,
		"exp":   now + 120,
	})

	claims, err := Verify(tok, pub)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.CID != 42 || claims.UID != 7 || claims.Role != "supporter" {
		t.Fatalf("claims = %+v, want cid=42 uid=7 role=supporter", claims)
	}
	if len(claims.Groups) != 1 || claims.Groups[0] != "support" {
		t.Fatalf("groups = %#v, want support", claims.Groups)
	}
	if len(claims.Perms) != 1 || claims.Perms[0] != "ai:reply:create" {
		t.Fatalf("perms = %#v, want ai:reply:create", claims.Perms)
	}
}

func TestVerifyRejectsBadSignature(t *testing.T) {
	keyA, _ := testKey(t)
	_, pubB := testKey(t)
	now := time.Now().Unix()
	tok := mintRS256(t, keyA, map[string]any{"cid": 42, "exp": now + 120})

	_, err := Verify(tok, pubB)
	if !errors.Is(err, ErrSignature) {
		t.Fatalf("err = %v, want ErrSignature", err)
	}
}

func TestVerifyRejectsHS256(t *testing.T) {
	_, pub := testKey(t)
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	body := base64.RawURLEncoding.EncodeToString([]byte(`{"cid":42}`))

	_, err := Verify(header+"."+body+".signature", pub)
	if !errors.Is(err, ErrSignature) {
		t.Fatalf("err = %v, want ErrSignature", err)
	}
}

func TestVerifyRejectsExpired(t *testing.T) {
	key, pub := testKey(t)
	now := time.Now().Unix()
	tok := mintRS256(t, key, map[string]any{"cid": 42, "exp": now - 3600})

	_, err := Verify(tok, pub)
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("err = %v, want ErrExpired", err)
	}
}

func TestVerifyRejectsMissingCID(t *testing.T) {
	key, pub := testKey(t)
	now := time.Now().Unix()
	tok := mintRS256(t, key, map[string]any{
		"iss": "laravel",
		"aud": "centric-rag",
		"use": "service",
		"exp": now + 120,
	})

	_, err := Verify(tok, pub)
	if !errors.Is(err, ErrClaim) {
		t.Fatalf("err = %v, want ErrClaim", err)
	}
}

func TestVerifyRejectsMissingExpiry(t *testing.T) {
	key, pub := testKey(t)
	tok := mintRS256(t, key, map[string]any{
		"cid": 42,
		"iss": "laravel",
		"aud": "centric-rag",
		"use": "service",
	})

	_, err := Verify(tok, pub)
	if !errors.Is(err, ErrClaim) {
		t.Fatalf("err = %v, want ErrClaim", err)
	}
}

func TestVerifyRejectsWrongIssuerAudienceOrUse(t *testing.T) {
	key, pub := testKey(t)
	now := time.Now().Unix()
	cases := []struct {
		name     string
		override map[string]any
	}{
		{name: "wrong_issuer", override: map[string]any{"iss": "other"}},
		{name: "wrong_audience", override: map[string]any{"aud": "other"}},
		{name: "wrong_use", override: map[string]any{"use": "spa"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{
				"cid": 42,
				"iss": "laravel",
				"aud": "centric-rag",
				"use": "service",
				"exp": now + 120,
			}
			for key, value := range tc.override {
				payload[key] = value
			}
			tok := mintRS256(t, key, payload)

			_, err := Verify(tok, pub)
			if !errors.Is(err, ErrClaim) {
				t.Fatalf("err = %v, want ErrClaim", err)
			}
		})
	}
}

func TestVerifyRejectsMalformed(t *testing.T) {
	_, pub := testKey(t)
	cases := []string{"", "a.b", "only-one-part", "a.b.c.d"}
	for _, tc := range cases {
		if _, err := Verify(tc, pub); !errors.Is(err, ErrMalformed) {
			t.Fatalf("token %q: err = %v, want ErrMalformed", tc, err)
		}
	}
}

func TestVerifyRejectsEmptyPublicKey(t *testing.T) {
	if _, err := Verify("a.b.c", ""); !errors.Is(err, ErrMalformed) {
		t.Fatalf("err = %v, want ErrMalformed", err)
	}
}
