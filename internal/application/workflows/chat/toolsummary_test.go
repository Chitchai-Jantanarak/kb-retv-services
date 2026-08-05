package chat

import (
	"context"
	"errors"
	"testing"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/application/tools"
	"github.com/my/app/internal/domain/ports"
)

type summaryProvider struct {
	text      string
	err       error
	genPrompt ports.Prompt
	calls     int
}

func (p *summaryProvider) Generate(_ context.Context, prompt ports.Prompt) (ports.Completion, error) {
	p.genPrompt = prompt
	p.calls++
	if p.err != nil {
		return ports.Completion{}, p.err
	}
	return ports.Completion{Text: p.text}, nil
}

func (p *summaryProvider) GenerateJSON(_ context.Context, prompt ports.Prompt) (ports.Completion, error) {
	return ports.Completion{Text: p.text}, nil
}

func (p *summaryProvider) Stream(context.Context, ports.Prompt) (<-chan ports.Completion, error) {
	ch := make(chan ports.Completion)
	close(ch)
	return ch, nil
}

func summaryFixture() skeleton.Response {
	return skeleton.Response{
		Matched:     true,
		ToolID:      "f1_find_cases",
		ComposeMode: "summary",
		Headline:    "2 cases match",
		Lines:       []string{"REP-1|wifi down|waiting", "REP-2|printer|done"},
		Params:      map[string]string{"status": "waiting"},
		Table: &skeleton.ToolTable{
			ToolID:  "f1_find_cases",
			Columns: []tools.ToolColumn{{Key: "code", Type: "case_ref", Primary: true}},
			Rows:    []map[string]string{{"code": "REP-1"}, {"code": "REP-2"}},
		},
	}
}

func summaryWorkflow(t *testing.T, provider ports.LLMProvider) *Workflow {
	t.Helper()
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), &stubFTS{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return wf
}

func TestComposeToolReplyGroundedSummary(t *testing.T) {
	provider := &summaryProvider{text: "Found 2 recent cases, REP-1 is still waiting."}
	wf := summaryWorkflow(t, provider)

	reply, ok := wf.composeToolReply(context.Background(), "en", "latest cases?", 3, summaryFixture())
	if !ok || reply != "Found 2 recent cases, REP-1 is still waiting." {
		t.Fatalf("reply = %q ok=%v", reply, ok)
	}
	if got := provider.genPrompt.User; got == "" || !containsAll(got, "latest cases?", "REP-1") {
		t.Fatalf("prompt user = %q, want question and rows", got)
	}
}

func TestComposeToolReplyRejectsFabricatedCode(t *testing.T) {
	provider := &summaryProvider{text: "Case REP-999 was found."}
	wf := summaryWorkflow(t, provider)

	if reply, ok := wf.composeToolReply(context.Background(), "en", "latest cases?", 3, summaryFixture()); ok {
		t.Fatalf("want rejection of fabricated code, got %q", reply)
	}
}

func TestComposeToolReplyProviderErrorFallsBack(t *testing.T) {
	provider := &summaryProvider{err: errors.New("upstream down")}
	wf := summaryWorkflow(t, provider)

	if reply, ok := wf.composeToolReply(context.Background(), "en", "latest cases?", 3, summaryFixture()); ok {
		t.Fatalf("want ok=false on provider error, got %q", reply)
	}
}

func TestComposeToolReplySkipsWithoutTable(t *testing.T) {
	provider := &summaryProvider{text: "anything"}
	wf := summaryWorkflow(t, provider)

	r := summaryFixture()
	r.Table = nil
	if _, ok := wf.composeToolReply(context.Background(), "en", "q", 3, r); ok {
		t.Fatal("want ok=false without table")
	}
	if provider.calls != 0 {
		t.Fatalf("provider called %d times, want 0", provider.calls)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
