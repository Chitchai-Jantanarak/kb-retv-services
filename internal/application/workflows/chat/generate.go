package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/domain/ports"
)

func (w *Workflow) generateTurn(
	ctx context.Context,
	req dto.ChatRequest,
	companyID int64,
	knowledge string,
	profileBlock string,
	timings map[string]int64,
) (llmTurn, error) {
	var prompt ports.Prompt
	err := timedChatErr(timings, "render", func() error {
		var err error
		prompt, err = w.tmpl.Render(map[string]string{
			"language":   promptLanguage(req.Locale),
			"transcript": buildTranscript(req.Messages),
			"knowledge":  knowledgeSection(knowledge),
			"profile":    profileSection(profileBlock),
		})
		return err
	})
	if err != nil {
		return llmTurn{}, fmt.Errorf("chat: render prompt: %w", err)
	}
	prompt.Attachments = toPromptAttachments(lastUserAttachments(req))

	var provider ports.LLMProvider
	err = timedChatErr(timings, "resolve", func() error {
		var err error
		provider, err = w.resolve(ctx, companyID)
		return err
	})
	if err != nil {
		return llmTurn{}, fmt.Errorf("chat: resolve provider: %w", err)
	}

	var completion ports.Completion
	err = timedChatErr(timings, "generate", func() error {
		var err error
		completion, err = provider.GenerateJSON(ctx, prompt)
		return err
	})
	if err != nil {
		return llmTurn{}, err
	}

	var turn llmTurn
	var structuredOK bool
	timedChat(timings, "parse", func() {
		turn, structuredOK = parseTurn(completion.Text)
	})
	if !structuredOK && strings.TrimSpace(completion.Text) != "" {
		repair := prompt
		repair.User = prompt.User + "\n\nYour previous output was not a single JSON object. Reply again with ONLY the JSON object described above."
		if retry, rerr := provider.GenerateJSON(ctx, repair); rerr == nil {
			if repairedTurn, ok := parseTurn(retry.Text); ok {
				turn = repairedTurn
				completion = retry
			}
		}
	}

	turn.Model = completion.Model
	turn.Vendor = completion.Vendor
	turn.TokensIn = completion.Usage.Input
	turn.TokensOut = completion.Usage.Output
	return turn, nil
}
