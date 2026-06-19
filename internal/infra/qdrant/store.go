package qdrant

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/my/app/internal/domain/ports"
)

type Config struct {
	URL    string
	APIKey string
}

type Store struct {
	url    string
	apiKey string
	client *http.Client
}

func NewStore(cfg Config) *Store {
	return &Store{
		url:    strings.TrimRight(cfg.URL, "/"),
		apiKey: cfg.APIKey,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *Store) EnsureCollection(ctx context.Context, name string, dim int) error {
	if err := validateCollectionName(name); err != nil {
		return err
	}
	body := map[string]any{
		"vectors": map[string]any{
			"size":     dim,
			"distance": "Cosine",
		},
	}
	err := s.do(ctx, http.MethodPut, "/collections/"+name, body, nil)
	var apiErr qdrantError
	if errors.As(err, &apiErr) && apiErr.statusCode == http.StatusConflict {
		return nil
	}
	return err
}

func (s *Store) CollectionVectorSize(ctx context.Context, name string) (int, bool, error) {
	if err := validateCollectionName(name); err != nil {
		return 0, false, err
	}
	var resp struct {
		Result struct {
			Config struct {
				Params struct {
					Vectors struct {
						Size int `json:"size"`
					} `json:"vectors"`
				} `json:"params"`
			} `json:"config"`
		} `json:"result"`
	}
	err := s.do(ctx, http.MethodGet, "/collections/"+name, nil, &resp)
	var apiErr qdrantError
	if errors.As(err, &apiErr) && apiErr.statusCode == http.StatusNotFound {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return resp.Result.Config.Params.Vectors.Size, true, nil
}

func (s *Store) RecreateCollection(ctx context.Context, name string, dim int) error {
	if err := validateCollectionName(name); err != nil {
		return err
	}
	err := s.do(ctx, http.MethodDelete, "/collections/"+name, nil, nil)
	var apiErr qdrantError
	if err != nil && !(errors.As(err, &apiErr) && apiErr.statusCode == http.StatusNotFound) {
		return err
	}
	return s.EnsureCollection(ctx, name, dim)
}

func (s *Store) Upsert(ctx context.Context, collection string, points []ports.VectorPoint) error {
	if err := validateCollectionName(collection); err != nil {
		return err
	}
	const upsertBatch = 200
	for start := 0; start < len(points); start += upsertBatch {
		end := min(start+upsertBatch, len(points))
		reqPoints := make([]map[string]any, 0, end-start)
		for _, point := range points[start:end] {
			reqPoints = append(reqPoints, map[string]any{
				"id":      qdrantPointID(point.ID),
				"vector":  point.Vector,
				"payload": point.Metadata,
			})
		}
		if err := s.do(ctx, http.MethodPut, "/collections/"+collection+"/points", map[string]any{"points": reqPoints}, nil); err != nil {
			return err
		}
	}
	return nil
}

func qdrantPointID(id string) any {
	if parsed, err := strconv.ParseUint(id, 10, 64); err == nil {
		return parsed
	}
	return id
}

func (s *Store) Search(ctx context.Context, q ports.VectorSearch) ([]ports.VectorHit, error) {
	if err := validateCollectionName(q.Collection); err != nil {
		return nil, err
	}
	body := map[string]any{
		"vector":       q.Query,
		"limit":        q.Limit,
		"with_payload": true,
	}
	if len(q.Filter) > 0 {
		must := make([]map[string]any, 0, len(q.Filter))
		for key, value := range q.Filter {
			must = append(must, map[string]any{
				"key":   key,
				"match": map[string]any{"value": value},
			})
		}
		body["filter"] = map[string]any{"must": must}
	}

	var resp struct {
		Result []struct {
			ID      any            `json:"id"`
			Score   float32        `json:"score"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	if err := s.do(ctx, http.MethodPost, "/collections/"+q.Collection+"/points/search", body, &resp); err != nil {
		return nil, err
	}

	hits := make([]ports.VectorHit, 0, len(resp.Result))
	for _, hit := range resp.Result {
		hits = append(hits, ports.VectorHit{
			ID:       fmt.Sprint(hit.ID),
			Score:    hit.Score,
			Metadata: hit.Payload,
		})
	}
	return hits, nil
}

func (s *Store) Delete(ctx context.Context, collection string, ids []string) error {
	if err := validateCollectionName(collection); err != nil {
		return err
	}
	body := map[string]any{"points": ids}
	return s.do(ctx, http.MethodPost, "/collections/"+collection+"/points/delete", body, nil)
}

func validateCollectionName(name string) error {
	if strings.TrimSpace(name) != name || name == "" {
		return errors.New("qdrant: collection is required")
	}
	sep := strings.LastIndex(name, "__")
	if sep <= 0 || sep+2 >= len(name) {
		return fmt.Errorf("qdrant: collection %q must be tenant scoped", name)
	}
	for _, r := range name[:sep] {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return fmt.Errorf("qdrant: collection %q has invalid prefix", name)
	}
	companyID, err := strconv.ParseInt(name[sep+2:], 10, 64)
	if err != nil || companyID <= 0 {
		return fmt.Errorf("qdrant: collection %q must end with a positive company_id", name)
	}
	return nil
}

func (s *Store) do(ctx context.Context, method string, path string, body any, out any) error {
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			return err
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, s.url+path, &payload)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		req.Header.Set("api-key", s.apiKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return qdrantError{
			method:     method,
			path:       path,
			status:     resp.Status,
			statusCode: resp.StatusCode,
			body:       strings.TrimSpace(string(raw)),
		}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type qdrantError struct {
	method     string
	path       string
	status     string
	statusCode int
	body       string
}

func (e qdrantError) Error() string {
	if e.body != "" {
		return fmt.Sprintf("qdrant %s %s returned %s: %s", e.method, e.path, e.status, e.body)
	}
	return fmt.Sprintf("qdrant %s %s returned %s", e.method, e.path, e.status)
}

var _ ports.VectorStore = (*Store)(nil)
