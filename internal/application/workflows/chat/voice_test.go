package chat

import (
	"testing"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/skeleton"
)

func TestVoiceRequestSkipsLLMSummary(t *testing.T) {
	summaryTool := skeleton.Response{ToolID: "f1_find_cases", ComposeMode: "summary"}

	if !summaryComposeMode(summaryTool, dto.ChatRequest{}) {
		t.Fatal("text mode must still summarise a summary-mode tool")
	}
	if summaryComposeMode(summaryTool, dto.ChatRequest{Mode: dto.ChatModeVoice}) {
		t.Fatal("voice mode must answer from the template, not a provider call")
	}
}

func TestTemplateToolIsNeverSummarised(t *testing.T) {
	templateTool := skeleton.Response{ToolID: "f4_workload", ComposeMode: "template"}

	for _, mode := range []string{"", dto.ChatModeVoice} {
		if summaryComposeMode(templateTool, dto.ChatRequest{Mode: mode}) {
			t.Fatalf("mode %q: a template tool must never reach the provider", mode)
		}
	}
}

func TestUnknownModeBehavesAsText(t *testing.T) {
	summaryTool := skeleton.Response{ToolID: "f1_find_cases", ComposeMode: "summary"}

	if !summaryComposeMode(summaryTool, dto.ChatRequest{Mode: "carrier-pigeon"}) {
		t.Fatal("an unrecognised mode must not silently disable summarisation")
	}
}
