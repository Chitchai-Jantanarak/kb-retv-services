package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"

	apperr "github.com/my/app/internal/shared/errors"
)

func LineSignatureVerifier(channelSecret string) SignatureVerifier {
	secret := []byte(strings.TrimSpace(channelSecret))
	return func(req *http.Request, raw []byte) error {
		if len(secret) == 0 {
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
		mac := hmac.New(sha256.New, secret)
		mac.Write(raw)
		if !hmac.Equal(given, mac.Sum(nil)) {
			return apperr.New(apperr.CodeUnauthorized, "invalid signature")
		}
		return nil
	}
}
