package servicejwt

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Claims struct {
	CID    int64    `json:"cid"`
	UID    int64    `json:"uid"`
	Role   string   `json:"role"`
	Groups []string `json:"grps"`
	Perms  []string `json:"perms"`
	Aud    Audience `json:"aud"`
	Use    string   `json:"use"`
	Exp    int64    `json:"exp"`
	Nbf    int64    `json:"nbf"`
	Iss    string   `json:"iss"`
}

type Audience []string

type header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

var (
	ErrMalformed = errors.New("servicejwt: malformed token")
	ErrSignature = errors.New("servicejwt: signature mismatch")
	ErrExpired   = errors.New("servicejwt: token expired")
	ErrClaim     = errors.New("servicejwt: missing or invalid cid claim")
)

const (
	expectedIssuer   = "laravel"
	expectedAudience = "centric-rag"
	expectedUse      = "service"
)

func Verify(token, publicKeyPEM string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || strings.TrimSpace(publicKeyPEM) == "" {
		return Claims{}, ErrMalformed
	}

	var h header
	if err := decodeSegment(parts[0], &h); err != nil {
		return Claims{}, fmt.Errorf("%w: parse header: %w", ErrMalformed, err)
	}
	if h.Alg != "RS256" {
		return Claims{}, ErrSignature
	}

	pub, err := parseRSAPublicKey(publicKeyPEM)
	if err != nil {
		return Claims{}, fmt.Errorf("%w: parse public key: %w", ErrMalformed, err)
	}

	signingInput := parts[0] + "." + parts[1]
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, fmt.Errorf("%w: decode signature: %w", ErrMalformed, err)
	}
	sum := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		return Claims{}, ErrSignature
	}

	var c Claims
	if err := decodeSegment(parts[1], &c); err != nil {
		return Claims{}, fmt.Errorf("%w: parse claims: %w", ErrMalformed, err)
	}

	now := time.Now().Unix()
	const skew = 30
	if c.Exp == 0 {
		return Claims{}, ErrClaim
	}
	if now > c.Exp+skew {
		return Claims{}, ErrExpired
	}
	if c.Nbf != 0 && now+skew < c.Nbf {
		return Claims{}, ErrExpired
	}
	if c.CID <= 0 {
		return Claims{}, ErrClaim
	}
	if c.Iss != expectedIssuer || !c.Aud.Contains(expectedAudience) || c.Use != expectedUse {
		return Claims{}, ErrClaim
	}
	return c, nil
}

func (a *Audience) UnmarshalJSON(raw []byte) error {
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		*a = Audience{single}
		return nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return err
	}
	*a = Audience(many)
	return nil
}

func (a Audience) Contains(expected string) bool {
	for _, got := range a {
		if got == expected {
			return true
		}
	}
	return false
}

func decodeSegment(segment string, out any) error {
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func parseRSAPublicKey(raw string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, errors.New("missing PEM block")
	}

	switch block.Type {
	case "PUBLIC KEY":
		key, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		pub, ok := key.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("public key is not RSA")
		}
		return pub, nil
	case "RSA PUBLIC KEY":
		return x509.ParsePKCS1PublicKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unsupported PEM block type %q", block.Type)
	}
}
