package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	apperr "github.com/my/app/internal/shared/errors"
)

type CatalogRepository interface {
	ConnectionFor(context.Context, int64, int64) (ProviderConnection, error)
	SaveCatalog(context.Context, int64, int64, int, []ProviderModel, string) error
}

type ModelCatalog struct {
	repo     CatalogRepository
	client   *http.Client
	localURL string
}

type CatalogSnapshot struct {
	ConnectionID int64           `json:"connection_id"`
	Models       []ProviderModel `json:"models"`
	RefreshedAt  *time.Time      `json:"refreshed_at"`
	Error        string          `json:"error,omitempty"`
}

func NewModelCatalog(repo CatalogRepository, localURL string) *ModelCatalog {
	return &ModelCatalog{repo: repo, localURL: localURL, client: &http.Client{
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (c *ModelCatalog) List(ctx context.Context, companyID, id int64) (CatalogSnapshot, error) {
	connection, err := c.repo.ConnectionFor(ctx, companyID, id)
	if err != nil {
		return CatalogSnapshot{}, err
	}
	return CatalogSnapshot{ConnectionID: id, Models: connection.Models, RefreshedAt: connection.RefreshedAt, Error: connection.ModelsError}, nil
}

func (c *ModelCatalog) Refresh(ctx context.Context, companyID, id int64) (CatalogSnapshot, error) {
	connection, err := c.repo.ConnectionFor(ctx, companyID, id)
	if err != nil {
		return CatalogSnapshot{}, err
	}
	refreshCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	models, err := c.discover(refreshCtx, connection)
	if err != nil {
		saveCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer stop()
		if saveErr := c.repo.SaveCatalog(saveCtx, companyID, id, connection.CredentialVersion, nil, "discovery_failed"); saveErr != nil {
			return CatalogSnapshot{}, saveErr
		}
		return CatalogSnapshot{}, apperr.New(apperr.CodeUpstreamFailed, "model discovery failed; previous catalog retained")
	}
	if err := c.repo.SaveCatalog(ctx, companyID, id, connection.CredentialVersion, models, ""); err != nil {
		return CatalogSnapshot{}, err
	}
	return c.List(ctx, companyID, id)
}

type catalogPage struct {
	Data []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
	} `json:"data"`
	Models []struct {
		Name        string   `json:"name"`
		DisplayName string   `json:"displayName"`
		Methods     []string `json:"supportedGenerationMethods"`
	} `json:"models"`
	HasMore       bool   `json:"has_more"`
	LastID        string `json:"last_id"`
	NextPageToken string `json:"nextPageToken"`
}

func (c *ModelCatalog) discover(ctx context.Context, connection ProviderConnection) ([]ProviderModel, error) {
	if connection.APIKey == "" && connection.Provider != VendorLocal {
		return nil, apperr.New(apperr.CodeInvalidInput, "connection credential missing")
	}
	models := []ProviderModel{}
	seen := map[string]bool{}
	token := ""
	for page := 0; page < 100; page++ {
		payload, err := c.fetchPage(ctx, connection, token)
		if err != nil {
			return nil, err
		}
		for _, model := range payload.models() {
			if model.ID != "" && len(model.ID) <= 190 && !seen[model.ID] {
				models = append(models, model)
				seen[model.ID] = true
			}
		}
		if len(models) > 10000 {
			return nil, catalogError("provider model limit exceeded")
		}
		next, err := payload.nextToken(connection.Provider)
		if err != nil {
			return nil, err
		}
		if next == "" {
			slices.SortFunc(models, func(a, b ProviderModel) int { return strings.Compare(a.ID, b.ID) })
			return models, nil
		}
		if next == token {
			return nil, catalogError("provider pagination repeated")
		}
		token = next
	}
	return nil, catalogError("provider pagination limit exceeded")
}

func (c *ModelCatalog) endpoint(provider string) (string, error) {
	switch provider {
	case VendorOpenAI:
		return "https://api.openai.com/v1/models", nil
	case VendorAnthropic:
		return "https://api.anthropic.com/v1/models", nil
	case VendorGemini:
		return "https://generativelanguage.googleapis.com/v1beta/models", nil
	case VendorOpenRouter:
		return "https://openrouter.ai/api/v1/models", nil
	case VendorLocal:
		base := c.localURL
		if base == "" {
			base = "http://localhost:11434/v1"
		}
		return strings.TrimRight(base, "/") + "/models", nil
	default:
		return "", apperr.New(apperr.CodeInvalidInput, "unsupported provider")
	}
}

func (c *ModelCatalog) modelRequest(ctx context.Context, connection ProviderConnection, token string) (*http.Request, error) {
	endpoint, err := c.endpoint(connection.Provider)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	switch connection.Provider {
	case VendorAnthropic:
		q.Set("limit", "1000")
		if token != "" {
			q.Set("after_id", token)
		}
	case VendorGemini:
		q.Set("pageSize", "1000")
		if token != "" {
			q.Set("pageToken", token)
		}
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	switch connection.Provider {
	case VendorAnthropic:
		req.Header.Set("x-api-key", connection.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	case VendorGemini:
		req.Header.Set("x-goog-api-key", connection.APIKey)
	default:
		if connection.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+connection.APIKey)
		}
	}
	return req, nil
}

func (c *ModelCatalog) fetchPage(ctx context.Context, connection ProviderConnection, token string) (catalogPage, error) {
	var payload catalogPage
	req, err := c.modelRequest(ctx, connection, token)
	if err != nil {
		return payload, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return payload, catalogError("provider discovery unavailable")
	}
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024+1))
	closeErr := resp.Body.Close()
	if resp.StatusCode != http.StatusOK || readErr != nil || closeErr != nil || len(raw) > 4*1024*1024 {
		return payload, catalogError("provider discovery response invalid")
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return payload, catalogError("provider discovery response invalid")
	}
	if payload.Data == nil && payload.Models == nil {
		return payload, catalogError("provider model list missing")
	}
	return payload, nil
}

func (p catalogPage) nextToken(provider string) (string, error) {
	if provider == VendorAnthropic && p.HasMore {
		if p.LastID == "" {
			return "", catalogError("provider pagination invalid")
		}
		return p.LastID, nil
	}
	return p.NextPageToken, nil
}

func (p catalogPage) models() []ProviderModel {
	models := make([]ProviderModel, 0, len(p.Data)+len(p.Models))
	for _, item := range p.Data {
		name := item.DisplayName
		if name == "" {
			name = item.Name
		}
		if name == "" {
			name = item.ID
		}
		models = append(models, ProviderModel{ID: item.ID, Name: name, Capabilities: map[string]bool{}})
	}
	for _, item := range p.Models {
		models = append(models, ProviderModel{
			ID: strings.TrimPrefix(item.Name, "models/"), Name: item.DisplayName,
			Capabilities: map[string]bool{
				"generate": slices.Contains(item.Methods, "generateContent"),
				"embed":    slices.Contains(item.Methods, "embedContent"),
			},
		})
	}
	return models
}

func catalogError(message string) error { return apperr.New(apperr.CodeUpstreamFailed, message) }
