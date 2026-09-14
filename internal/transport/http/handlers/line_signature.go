package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	apperr "github.com/my/app/internal/shared/errors"
)

// verifyLineHMAC checks raw against the X-Line-Signature header using the
// standard LINE Messaging API scheme: HMAC-SHA256 of the raw body, base64
// encoded. Shared by LineSignatureVerifier and LineAccountSignatureVerifier.
func verifyLineHMAC(req *http.Request, raw []byte, secret string) error {
	secretBytes := []byte(strings.TrimSpace(secret))
	if len(secretBytes) == 0 {
		return apperr.New(apperr.CodeUnauthorized, "line channel secret not configured")
	}
	sig := strings.TrimSpace(req.Header.Get("X-Line-Signature"))
	if sig == "" {
		return apperr.New(apperr.CodeUnauthorized, "invalid signature")
	}
	given, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return apperr.New(apperr.CodeUnauthorized, "invalid signature")
	}
	mac := hmac.New(sha256.New, secretBytes)
	mac.Write(raw)
	if !hmac.Equal(given, mac.Sum(nil)) {
		return apperr.New(apperr.CodeUnauthorized, "invalid signature")
	}
	return nil
}

func LineSignatureVerifier(channelSecret string) SignatureVerifier {
	secret := strings.TrimSpace(channelSecret)
	return func(req *http.Request, raw []byte) error {
		return verifyLineHMAC(req, raw, secret)
	}
}

// LineAccountSignatureVerifier resolves the LINE channel secret per-account:
// it reads the webhook body's "destination" (the LINE bot user id), looks up
// that account's channel_accounts.credentials.channel_secret via lookup,
// decrypts it with decrypt, and falls back to fallbackSecret (the server env
// secret) when no per-account secret is found. This lets each tenant's LINE
// channel verify against its own secret while still supporting a single
// shared/legacy account configured purely through env.
func LineAccountSignatureVerifier(lookup func(ctx context.Context, destination string) (string, error), decrypt func(string) (string, bool), fallbackSecret string) SignatureVerifier {
	fallbackSecret = strings.TrimSpace(fallbackSecret)
	return func(req *http.Request, raw []byte) error {
		var body struct {
			Destination string `json:"destination"`
		}
		_ = json.Unmarshal(raw, &body)
		destination := strings.TrimSpace(body.Destination)

		secret := ""
		if destination != "" && lookup != nil {
			stored, err := lookup(req.Context(), destination)
			if err == nil && strings.TrimSpace(stored) != "" {
				stored = strings.TrimSpace(stored)
				if decrypt != nil {
					if plain, ok := decrypt(stored); ok {
						secret = plain
					} else {
						secret = stored
					}
				} else {
					secret = stored
				}
			}
		}
		if secret == "" {
			secret = fallbackSecret
		}
		return verifyLineHMAC(req, raw, secret)
	}
}
