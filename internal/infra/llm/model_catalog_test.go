package llm

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	apperr "github.com/my/app/internal/shared/errors"
)

type catalogTestTransport func(*http.Request) (*http.Response, error)

func (f catalogTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCatalogProviderDiscovery(t *testing.T) {
	cases := []struct{ provider, host, header, body string }{
		{VendorOpenAI, "api.openai.com", "Authorization", `{"data":[{"id":"model-a"}]}`},
		{VendorOpenRouter, "openrouter.ai", "Authorization", `{"data":[{"id":"model-a"}]}`},
		{VendorAnthropic, "api.anthropic.com", "x-api-key", `{"data":[{"id":"model-a","display_name":"Model A"}],"has_more":false}`},
		{VendorGemini, "generativelanguage.googleapis.com", "x-goog-api-key", `{"models":[{"name":"models/model-a","displayName":"Model A","supportedGenerationMethods":["generateContent"]}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			catalog := NewModelCatalog(nil, "")
			catalog.client.Transport = catalogTestTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host != tc.host || !strings.Contains(req.Header.Get(tc.header), "test-key") || strings.Contains(req.URL.String(), "test-key") {
					t.Fatal("incorrect endpoint or credential transport")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			models, err := catalog.discover(context.Background(), ProviderConnection{Provider: tc.provider, APIKey: "test-key"})
			if err != nil || len(models) != 1 || models[0].ID != "model-a" {
				t.Fatalf("models=%+v err=%v", models, err)
			}
			if tc.provider == VendorOpenAI && len(models[0].Capabilities) != 0 {
				t.Fatal("unknown capabilities fabricated")
			}
		})
	}
}

func TestCatalogPaginatesAndRejectsRepeatedCursor(t *testing.T) {
	for _, repeated := range []bool{false, true} {
		catalog := NewModelCatalog(nil, "")
		calls := 0
		catalog.client.Transport = catalogTestTransport(func(req *http.Request) (*http.Response, error) {
			calls++
			body := `{"data":[{"id":"first"}],"has_more":true,"last_id":"first"}`
			if calls == 2 {
				if req.URL.Query().Get("after_id") != "first" {
					t.Fatal("cursor missing")
				}
				if !repeated {
					body = `{"data":[{"id":"second"}],"has_more":false}`
				}
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		models, err := catalog.discover(context.Background(), ProviderConnection{Provider: VendorAnthropic, APIKey: "key"})
		if repeated && err == nil {
			t.Fatal("repeated cursor accepted")
		}
		if !repeated && (err != nil || len(models) != 2) {
			t.Fatalf("models=%+v err=%v", models, err)
		}
	}
}

type catalogTestRepo struct {
	connection ProviderConnection
	version    int
	failure    string
}

func (r *catalogTestRepo) ConnectionFor(context.Context, int64, int64) (ProviderConnection, error) {
	return r.connection, nil
}

func (r *catalogTestRepo) SaveCatalog(_ context.Context, _ int64, _ int64, version int, models []ProviderModel, failure string) error {
	if version != r.version {
		return apperr.New(apperr.CodeConflict, "rotated")
	}
	r.failure = failure
	if failure == "" {
		r.connection.Models = models
		now := time.Now()
		r.connection.RefreshedAt = &now
	}
	return nil
}

func TestCatalogFailureRetainsModelsAndRotationRejectsStaleRefresh(t *testing.T) {
	repo := &catalogTestRepo{version: 1, connection: ProviderConnection{Provider: VendorOpenAI, APIKey: "key", CredentialVersion: 1, Models: []ProviderModel{{ID: "saved"}}}}
	catalog := NewModelCatalog(repo, "")
	catalog.client.Transport = catalogTestTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("private-key"))}, nil
	})
	_, err := catalog.Refresh(context.Background(), 7, 1)
	if err == nil || strings.Contains(err.Error(), "private-key") || repo.connection.Models[0].ID != "saved" || repo.failure != "discovery_failed" {
		t.Fatalf("err=%v repo=%+v", err, repo)
	}
	catalog.client.Transport = catalogTestTransport(func(*http.Request) (*http.Response, error) {
		repo.version = 2
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"old-key-model"}]}`))}, nil
	})
	_, err = catalog.Refresh(context.Background(), 7, 1)
	appErr, ok := apperr.As(err)
	if !ok || appErr.Code != apperr.CodeConflict || repo.connection.Models[0].ID != "saved" {
		t.Fatalf("rotation not respected: %v", err)
	}
}
