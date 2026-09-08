package gmailconn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const defaultTokenURL = "https://oauth2.googleapis.com/token"

// ErrNeedsReconnect is returned when Google rejects a refresh token outright
// (invalid_grant): the tenant revoked access or the token expired from
// disuse, and the account can only be fixed by the owner reconnecting Gmail
// through the Laravel OAuth consent flow again.
var ErrNeedsReconnect = errors.New("gmailconn: refresh token rejected, account needs reconnect")

type cachedToken struct {
	accessToken string
	expiresAt   time.Time
}

// TokenSource exchanges a tenant's Gmail refresh token for a short-lived
// access token via Google's OAuth token endpoint, caching the result per
// account until shortly before it expires.
type TokenSource struct {
	clientID, clientSecret string
	tokenURL               string
	httpClient             *http.Client

	mu    sync.Mutex
	cache map[int64]cachedToken
}

// NewTokenSource builds a TokenSource. tokenURL overrides the default Google
// endpoint for tests; pass "" to use https://oauth2.googleapis.com/token.
func NewTokenSource(clientID, clientSecret, tokenURL string, httpClient *http.Client) *TokenSource {
	if tokenURL == "" {
		tokenURL = defaultTokenURL
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &TokenSource{
		clientID:     clientID,
		clientSecret: clientSecret,
		tokenURL:     tokenURL,
		httpClient:   httpClient,
		cache:        make(map[int64]cachedToken),
	}
}

// AccessToken returns a cached or freshly refreshed Gmail API access token
// for accountID. refreshToken must already be decrypted. On invalid_grant it
// returns ErrNeedsReconnect and drops the cache entry for the account.
func (t *TokenSource) AccessToken(ctx context.Context, accountID int64, refreshToken string) (string, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return "", errors.New("gmailconn: refresh token is empty")
	}

	t.mu.Lock()
	if tok, ok := t.cache[accountID]; ok && time.Now().Before(tok.expiresAt) {
		t.mu.Unlock()
		return tok.accessToken, nil
	}
	t.mu.Unlock()

	form := url.Values{
		"client_id":     {t.clientID},
		"client_secret": {t.clientSecret},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("gmailconn: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("gmailconn: token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &errBody)
		if resp.StatusCode == http.StatusBadRequest && errBody.Error == "invalid_grant" {
			t.mu.Lock()
			delete(t.cache, accountID)
			t.mu.Unlock()
			return "", ErrNeedsReconnect
		}
		return "", fmt.Errorf("gmailconn: token endpoint returned status %d: %s", resp.StatusCode, bytes.TrimSpace(body))
	}

	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("gmailconn: decode token response: %w", err)
	}
	if strings.TrimSpace(parsed.AccessToken) == "" {
		return "", errors.New("gmailconn: token response missing access_token")
	}

	ttl := time.Duration(parsed.ExpiresIn) * time.Second
	if ttl > time.Minute {
		ttl -= 60 * time.Second
	}
	t.mu.Lock()
	t.cache[accountID] = cachedToken{accessToken: parsed.AccessToken, expiresAt: time.Now().Add(ttl)}
	t.mu.Unlock()

	return parsed.AccessToken, nil
}
