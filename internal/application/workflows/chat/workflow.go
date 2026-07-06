package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/ai/rag"
	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
)

type FTSSource interface {
	SearchChunks(ctx context.Context, coverage []int64, query string, limit int) ([]rag.FTSChunk, error)
}

type Workflow struct {
	tmpl    prompts.Template
	resolve rag.ProviderForCompany
	fts     FTSSource
}

func New(registry *prompts.Registry, resolve rag.ProviderForCompany, fts FTSSource) (*Workflow, error) {
	if registry == nil {
		return nil, fmt.Errorf("chat: prompt registry is required")
	}
	if resolve == nil {
		return nil, fmt.Errorf("chat: provider resolver is required")
	}
	tmpl, err := registry.Get(prompts.NameChat)
	if err != nil {
		return nil, fmt.Errorf("chat: %w", err)
	}
	return &Workflow{tmpl: tmpl, resolve: resolve, fts: fts}, nil
}

func (w *Workflow) Run(ctx context.Context, req dto.ChatRequest) (dto.ChatResponse, error) {
	if err := req.Validate(); err != nil {
		return dto.ChatResponse{}, err
	}
	companyID, ok := ctxkey.CompanyID(ctx)
	if !ok || companyID <= 0 {
		return dto.ChatResponse{}, apperr.New(apperr.CodeUnauthorized, "company scope is required")
	}
	lastUser := req.LastUserMessage()
	if lastUser == "" {
		return dto.ChatResponse{}, apperr.New(apperr.CodeInvalidInput, "a user message is required")
	}

	sources, knowledge := w.knowledgeContext(ctx, companyID, lastUser)
	prompt, err := w.tmpl.Render(map[string]string{
		"language":   promptLanguage(req.Locale),
		"transcript": buildTranscript(req.Messages),
		"knowledge":  knowledgeSection(knowledge),
	})
	if err != nil {
		return dto.ChatResponse{}, fmt.Errorf("chat: render prompt: %w", err)
	}

	provider, err := w.resolve(ctx, companyID)
	if err != nil {
		return dto.ChatResponse{}, fmt.Errorf("chat: resolve provider: %w", err)
	}
	completion, err := provider.GenerateJSON(ctx, prompt)
	if err != nil {
		return dto.ChatResponse{}, err
	}

	turn := parseTurn(completion.Text)
	return dto.ChatResponse{
		Reply:   turn.Reply,
		Sources: sources,
		Case:    normalizeCaseDraft(turn.Case),
	}, nil
}

func (w *Workflow) knowledgeContext(ctx context.Context, companyID int64, query string) ([]string, string) {
	if w.fts == nil {
		return nil, ""
	}
	chunks, err := w.fts.SearchChunks(ctx, []int64{companyID}, query, knowledgeChunkLimit)
	if err != nil {
		return nil, ""
	}

	var (
		sources []string
		block   strings.Builder
	)
	for _, chunk := range chunks {
		title := strings.TrimSpace(chunk.Title)
		if title != "" {
			sources = append(sources, title)
		}
		snippet := strings.TrimSpace(chunk.Content)
		if len(snippet) > knowledgeSnippetMaxChars {
			snippet = snippet[:knowledgeSnippetMaxChars]
		}
		if snippet != "" {
			fmt.Fprintf(&block, "- %s: %s\n", title, snippet)
		}
	}
	return sources, block.String()
}
