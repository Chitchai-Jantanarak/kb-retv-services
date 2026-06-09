package ports

import "context"

type Prompt struct {
	System  string
	User    string
	Vars    map[string]string
	MaxToks int
	Temp    float32
	Stop    []string
}

type Completion struct {
	Text   string
	Usage  TokenUsage
	Vendor string
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
