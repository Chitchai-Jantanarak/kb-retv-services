package ports

import (
	"context"
	"errors"
)

// ErrAIDisabled means the company has no active ai_agents row. Callers degrade to human handling.
var ErrAIDisabled = errors.New("ai disabled for company")

type Prompt struct {
	System      string
	User        string
	Vars        map[string]string
	MaxToks     int
	ThinkBudget *int
	Temp        float32
	Stop        []string
	Attachments []Attachment
	Images      []PromptImage
}

type Attachment struct {
	ID         string
	MIMEType   string
	StorageKey string
	URL        string
	SizeBytes  int64
}

type PromptImage struct {
	MIMEType string
	Data     []byte
}

type Completion struct {
	Text   string
	Usage  TokenUsage
	Vendor string
	Model  string
}

type TokenUsage struct {
	Input  int
	Output int
	Total  int
}

type LLMProvider interface {
	Generate(ctx context.Context, p Prompt) (Completion, error)
	// GenerateJSON calls the model with json_object / structured-output mode.
	// The returned Completion.Text must be valid JSON unmarshallable by the caller.
	GenerateJSON(ctx context.Context, p Prompt) (Completion, error)
	Stream(ctx context.Context, p Prompt) (<-chan Completion, error)
}

type EmbeddingProvider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dim() int
}
