package gmailconn

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const defaultBaseURL = "https://gmail.googleapis.com/gmail/v1"

// Client talks to the Gmail REST API over plain net/http (no generated SDK:
// the surface used here is three endpoints). baseURL is injectable so tests
// can point it at an httptest server.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient builds a Client. baseURL overrides the default Gmail API host for
// tests; pass "" to use https://gmail.googleapis.com/gmail/v1.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient}
}

type historyResponse struct {
	History []struct {
		MessagesAdded []struct {
			Message struct {
				ID string `json:"id"`
			} `json:"message"`
		} `json:"messagesAdded"`
	} `json:"history"`
	NextPageToken string `json:"nextPageToken"`
	HistoryID     string `json:"historyId"`
}

type listMessagesResponse struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
}

type profileResponse struct {
	HistoryID string `json:"historyId"`
}

type rawMessageResponse struct {
	Raw string `json:"raw"`
}

// History returns the ids of every message added to INBOX since
// startHistoryID (paginating through nextPageToken) along with the freshest
// historyId to store for the next poll. If Google has aged startHistoryID out
// (HTTP 404, "history too old"), it falls back to listing the recent inbox
// directly and reports a fresh historyId from the account profile.
func (c *Client) History(ctx context.Context, accessToken, startHistoryID string) ([]string, string, error) {
	if strings.TrimSpace(startHistoryID) == "" {
		return c.fallbackRecent(ctx, accessToken)
	}

	var ids []string
	newHistoryID := startHistoryID
	pageToken := ""
	for {
		q := url.Values{
			"startHistoryId": {startHistoryID},
			"historyTypes":   {"messageAdded"},
			"labelId":        {"INBOX"},
		}
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		req, err := c.newRequest(ctx, accessToken, "/users/me/history?"+q.Encode())
		if err != nil {
			return nil, "", err
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, "", fmt.Errorf("gmailconn: history request: %w", err)
		}

		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			return c.fallbackRecent(ctx, accessToken)
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, "", fmt.Errorf("gmailconn: history returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		var parsed historyResponse
		err = json.NewDecoder(resp.Body).Decode(&parsed)
		resp.Body.Close()
		if err != nil {
			return nil, "", fmt.Errorf("gmailconn: decode history response: %w", err)
		}

		for _, h := range parsed.History {
			for _, ma := range h.MessagesAdded {
				if ma.Message.ID != "" {
					ids = append(ids, ma.Message.ID)
				}
			}
		}
		if parsed.HistoryID != "" {
			newHistoryID = parsed.HistoryID
		}
		if parsed.NextPageToken == "" {
			break
		}
		pageToken = parsed.NextPageToken
	}

	return dedupeStrings(ids), newHistoryID, nil
}

// fallbackRecent is used when Google reports startHistoryId as too old to
// replay (HTTP 404): it lists the recent inbox directly and re-anchors on the
// account's current historyId from its profile.
func (c *Client) fallbackRecent(ctx context.Context, accessToken string) ([]string, string, error) {
	q := url.Values{
		"labelIds":   {"INBOX"},
		"q":          {"newer_than:1d"},
		"maxResults": {"50"},
	}
	req, err := c.newRequest(ctx, accessToken, "/users/me/messages?"+q.Encode())
	if err != nil {
		return nil, "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("gmailconn: list messages request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("gmailconn: list messages returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed listMessagesResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, "", fmt.Errorf("gmailconn: decode list messages response: %w", err)
	}

	ids := make([]string, 0, len(parsed.Messages))
	for _, m := range parsed.Messages {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}

	historyID, err := c.Profile(ctx, accessToken)
	if err != nil {
		return nil, "", err
	}
	return ids, historyID, nil
}

// Profile fetches the account's current Gmail historyId.
func (c *Client) Profile(ctx context.Context, accessToken string) (string, error) {
	req, err := c.newRequest(ctx, accessToken, "/users/me/profile")
	if err != nil {
		return "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("gmailconn: profile request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gmailconn: profile returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed profileResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("gmailconn: decode profile response: %w", err)
	}
	if parsed.HistoryID == "" {
		return "", errors.New("gmailconn: profile response missing historyId")
	}
	return parsed.HistoryID, nil
}

// Raw fetches a message's full raw RFC 822 bytes.
func (c *Client) Raw(ctx context.Context, accessToken, id string) ([]byte, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("gmailconn: message id is required")
	}
	req, err := c.newRequest(ctx, accessToken, "/users/me/messages/"+url.PathEscape(id)+"?format=raw")
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gmailconn: raw message request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gmailconn: raw message returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed rawMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("gmailconn: decode raw message response: %w", err)
	}
	raw, err := decodeBase64URL(parsed.Raw)
	if err != nil {
		return nil, fmt.Errorf("gmailconn: decode raw message body: %w", err)
	}
	return raw, nil
}

func (c *Client) newRequest(ctx context.Context, accessToken, path string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("gmailconn: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	return req, nil
}

// decodeBase64URL decodes Gmail's base64url message payloads, which are
// usually unpadded but not guaranteed to be.
func decodeBase64URL(s string) ([]byte, error) {
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.URLEncoding.DecodeString(s)
}

func dedupeStrings(in []string) []string {
	if len(in) < 2 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := in[:0]
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
