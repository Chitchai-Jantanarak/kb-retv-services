package chat

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/my/app/internal/application/skeleton"
)

func TestClarifyViaModelUsesMissingFields(t *testing.T) {
	provider := &summaryProvider{text: "Which case do you mean? Codes look like REP-606."}
	wf := summaryWorkflow(t, provider)

	reply, ok := wf.clarifyViaModel(context.Background(), "en", "close my case", 3, []string{"case_code"}, map[string]string{"status": "done"})
	if !ok || reply == "" {
		t.Fatalf("reply = %q ok=%v", reply, ok)
	}
	if !containsAll(provider.genPrompt.User, "close my case", "case_code", "status=done") {
		t.Fatalf("prompt user = %q", provider.genPrompt.User)
	}
}

func TestClarifyFallsBackWhenProviderFails(t *testing.T) {
	provider := &summaryProvider{err: errors.New("down")}
	wf := summaryWorkflow(t, provider)

	if reply, ok := wf.clarifyViaModel(context.Background(), "en", "close my case", 3, []string{"case_code"}, nil); ok {
		t.Fatalf("want ok=false, got %q", reply)
	}
}

func TestMissingFieldsUnwrapsThroughOrchestratorWrap(t *testing.T) {
	wrapped := fmt.Errorf("skeleton: handler %q: %w", "reports.track", skeleton.NeedsParams("case_code"))
	fields := missingFields(wrapped)
	if len(fields) != 1 || fields[0] != "case_code" {
		t.Fatalf("fields = %v", fields)
	}
	if !errors.Is(wrapped, skeleton.ErrNeedsParam) {
		t.Fatal("wrapped error must still satisfy ErrNeedsParam")
	}
}
