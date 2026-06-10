package embeddings

import (
	"context"
	"fmt"
	"strings"
)

type BatchPollResult struct {
	State     string
	Done      bool
	Succeeded bool
	Results   []BatchEmbedResult
}

type BatchEmbedder interface {
	Submit(ctx context.Context, displayName string, reqs []BatchEmbedRequest) (string, error)
	Poll(ctx context.Context, jobName string) (BatchPollResult, error)
	Model() string
	Dim() int
}

type GeminiBatchEmbedder struct {
	client *GeminiBatchClient
	model  string
	dim    int
}

func NewGeminiBatchEmbedder(cfg GeminiConfig) *GeminiBatchEmbedder {
	model := strings.TrimPrefix(strings.TrimSpace(cfg.Model), "models/")
	if model == "" {
		model = "gemini-embedding-2"
	}
	dim := cfg.Dimensions
	if dim <= 0 {
		dim = 3072
	}
	return &GeminiBatchEmbedder{
		client: NewGeminiBatchClient(cfg),
		model:  model,
		dim:    dim,
	}
}

func (e *GeminiBatchEmbedder) Model() string { return e.model }

func (e *GeminiBatchEmbedder) Dim() int { return e.dim }

func (e *GeminiBatchEmbedder) Submit(ctx context.Context, displayName string, reqs []BatchEmbedRequest) (string, error) {
	if len(reqs) == 0 {
		return "", fmt.Errorf("batch submit: no requests")
	}
	jsonl := buildEmbedJSONL(e.model, e.dim, reqs)
	fileName, err := e.client.UploadJSONL(ctx, displayName, jsonl)
	if err != nil {
		return "", err
	}
	return e.client.CreateEmbedBatch(ctx, e.model, displayName, fileName)
}

func (e *GeminiBatchEmbedder) Poll(ctx context.Context, jobName string) (BatchPollResult, error) {
	job, err := e.client.GetBatch(ctx, jobName)
	if err != nil {
		return BatchPollResult{}, err
	}

	result := BatchPollResult{State: job.State, Done: job.Done(), Succeeded: job.Succeeded()}
	if !job.Done() {
		return result, nil
	}
	if !job.Succeeded() {
		return result, fmt.Errorf("batch %s ended in state %s", jobName, job.State)
	}
	if job.ResultFileName == "" {
		return result, fmt.Errorf("batch %s succeeded but carried no result file", jobName)
	}

	data, err := e.client.DownloadResults(ctx, job.ResultFileName)
	if err != nil {
		return result, err
	}
	results, err := parseEmbedResultsJSONL(data)
	if err != nil {
		return result, err
	}
	result.Results = results
	return result, nil
}

func NewBatchEmbedder(settings ProviderSettings) (string, string, BatchEmbedder, error) {
	provider := ResolveProvider(settings)
	model := DefaultModel(provider, settings.Model)
	switch provider {
	case "gemini":
		return provider, model, NewGeminiBatchEmbedder(GeminiConfig{
			BaseURL:    settings.BaseURL,
			APIKey:     settings.GeminiKey,
			Model:      model,
			Dimensions: settings.Dimensions,
		}), nil
	default:
		return "", "", nil, fmt.Errorf("batch embedding not supported for provider %q; use online mode", provider)
	}
}

var _ BatchEmbedder = (*GeminiBatchEmbedder)(nil)
