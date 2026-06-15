package reply

import (
	"context"
	"strings"
	"testing"

	"github.com/my/app/internal/ai/rag"
	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/domain/kb"
	"github.com/my/app/internal/repositories/kb/memory"
	"github.com/my/app/internal/shared/ctxkey"
)

func TestWorkflowRunReturnsCompanyAwareSuggestion(t *testing.T) {
	repo := memory.NewKnowledgeRepository([]kb.Article{
		{
			ID:        "kb-001",
			CompanyID: 1,
			Title:     "Printer offline",
			Summary:   "Ask the customer to verify power, network, and printer queue status.",
		},
		{
			ID:        "kb-002",
			CompanyID: 2,
			Title:     "Other company article",
			Summary:   "This article must not leak across companies.",
		},
	})
	workflow := NewWorkflow(repo)

	ctx := ctxkey.WithCompanyID(context.Background(), 1)
	resp, err := workflow.Run(ctx, dto.ReplyRequest{
		CustomerID: "cust-001",
		Message:    "The printer is offline.",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if resp.Intent != "troubleshooting" {
		t.Fatalf("Intent = %q, want troubleshooting", resp.Intent)
	}
	if resp.Suggestion == "" {
		t.Fatal("Suggestion is empty")
	}
	if len(resp.Sources) != 1 {
		t.Fatalf("Sources length = %d, want 1", len(resp.Sources))
	}
	if resp.Sources[0].ID != "kb-001" {
		t.Fatalf("Source ID = %q, want kb-001", resp.Sources[0].ID)
	}
}

func TestWorkflowRunCanUseAdditionalRetriever(t *testing.T) {
	repo := memory.NewKnowledgeRepository(nil)
	workflow := NewWorkflow(repo, WithRetriever(&singleCandidateRetriever{}))

	ctx := ctxkey.WithCompanyID(context.Background(), 1)
	resp, err := workflow.Run(ctx, dto.ReplyRequest{
		CustomerID: "cust-001",
		Message:    "Order not received.",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(resp.Sources) != 1 {
		t.Fatalf("Sources length = %d, want 1", len(resp.Sources))
	}
	if resp.Sources[0].ID != "vector-1" {
		t.Fatalf("Source ID = %q, want vector-1", resp.Sources[0].ID)
	}
}

func TestWorkflowRunSurfacesDebugStageTimings(t *testing.T) {
	repo := memory.NewKnowledgeRepository(nil)
	workflow := NewWorkflow(repo, WithRetriever(&singleCandidateRetriever{}))

	ctx := ctxkey.WithCompanyID(context.Background(), 1)
	resp, err := workflow.Run(ctx, dto.ReplyRequest{
		CustomerID: "cust-001",
		Message:    "Order not received.",
		Mode:       rag.ModeDebug,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(resp.StageTimingsMS) == 0 {
		t.Fatal("StageTimingsMS is empty in debug mode")
	}
	if _, ok := resp.StageTimingsMS["retrieve"]; !ok {
		t.Fatalf("retrieve timing missing: %+v", resp.StageTimingsMS)
	}
	if resp.DebugTrace == nil {
		t.Fatal("DebugTrace is nil in debug mode")
	}
}

func TestWorkflowRunUsesSymptomMetadataForGraphAnchor(t *testing.T) {
	repo := memory.NewKnowledgeRepository(nil)
	graph := &metadataGraphRetriever{
		candidates: []rag.Candidate{{ID: "graph-44", Title: "Tray jam", Content: "Reset the tray sensor.", Source: "graph", Score: 1}},
	}
	workflow := NewWorkflow(repo, WithGraphRetriever(graph))

	ctx := ctxkey.WithCompanyID(context.Background(), 1)
	resp, err := workflow.Run(ctx, dto.ReplyRequest{
		CustomerID: "cust-001",
		Message:    "Tray keeps jamming",
		Metadata:   map[string]string{"symptom_node_id": "44"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if graph.calls != 1 {
		t.Fatalf("graph calls = %d, want 1", graph.calls)
	}
	if len(graph.lastPlan.GraphAnchors) != 1 || graph.lastPlan.GraphAnchors[0].ID != 44 {
		t.Fatalf("graph anchors = %+v, want symptom 44", graph.lastPlan.GraphAnchors)
	}
	if len(resp.Sources) == 0 || resp.Sources[0].ID != "graph-44" {
		t.Fatalf("Sources = %+v, want graph source first", resp.Sources)
	}
}

type singleCandidateRetriever struct{}

func (singleCandidateRetriever) Retrieve(ctx context.Context, query rag.Query, meta rag.Meta) ([]rag.Candidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []rag.Candidate{{ID: "vector-1", Title: "Vector hit", Content: "Use vector candidate", Source: "vector", Score: 1}}, nil
}

type metadataGraphRetriever struct {
	calls      int
	lastPlan   rag.QueryPlan
	candidates []rag.Candidate
}

func (r *metadataGraphRetriever) RetrieveGraph(_ context.Context, _ rag.Query, plan rag.QueryPlan) ([]rag.Candidate, error) {
	r.calls++
	r.lastPlan = plan
	return r.candidates, nil
}

func TestSelectSuggestionEscalateReturnsStub(t *testing.T) {
	result := rag.Result{
		Decision:   rag.DecisionEscalate,
		Candidates: []rag.Candidate{{ID: "x", Content: "raw doc body that must not leak"}},
		Meta:       rag.Meta{Intent: "troubleshooting"},
	}

	got := selectSuggestion(result)

	if strings.Contains(got, "raw doc body") {
		t.Fatalf("escalate leaked candidate content: %q", got)
	}
	if !strings.Contains(got, "Escalating") {
		t.Fatalf("got %q, want escalation stub", got)
	}
}

func TestSelectSuggestionFallbackStripsImages(t *testing.T) {
	result := rag.Result{
		Decision:   rag.DecisionDraft,
		Candidates: []rag.Candidate{{ID: "x", Content: `<p>fix</p><img src="data:image/png;base64,AAAA">`}},
		Meta:       rag.Meta{Intent: "troubleshooting"},
	}

	got := selectSuggestion(result)

	if strings.Contains(got, "base64") || strings.Contains(got, "<img") {
		t.Fatalf("fallback leaked image data: %q", got)
	}
	if !strings.Contains(got, "Suggested reply") {
		t.Fatalf("got %q, want buildSuggestion output", got)
	}
}

func TestSelectSuggestionPrefersExistingDraft(t *testing.T) {
	result := rag.Result{
		Decision: rag.DecisionAuto,
		Draft:    "ready draft",
	}

	if got := selectSuggestion(result); got != "ready draft" {
		t.Fatalf("got %q, want existing draft", got)
	}
}
