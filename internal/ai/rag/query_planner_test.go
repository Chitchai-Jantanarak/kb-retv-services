package rag

import (
	"context"
	"strings"
	"testing"
)

func TestDefaultQueryPlannerNormalizesClassifiesAndBoundsLimit(t *testing.T) {
	planner := DefaultQueryPlanner{}

	plan, err := planner.Plan(context.Background(), Query{
		CompanyID: 1,
		Text:      "  Printer    OFFLINE   now  ",
		Limit:     20,
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if plan.OriginalText != "  Printer    OFFLINE   now  " {
		t.Fatalf("OriginalText = %q", plan.OriginalText)
	}
	if plan.RetrievalText != "Printer OFFLINE now" {
		t.Fatalf("RetrievalText = %q, want whitespace-collapsed text", plan.RetrievalText)
	}
	if plan.CanonicalText != "printer offline now" {
		t.Fatalf("CanonicalText = %q, want lower whitespace-collapsed text", plan.CanonicalText)
	}
	if plan.Meta.Intent != "troubleshooting" {
		t.Fatalf("Intent = %q, want troubleshooting", plan.Meta.Intent)
	}
	if plan.EffectiveLimit != maxQueryPlanLimit {
		t.Fatalf("EffectiveLimit = %d, want cap %d", plan.EffectiveLimit, maxQueryPlanLimit)
	}
	if len(plan.RetrievalQueries) != 1 {
		t.Fatalf("RetrievalQueries = %+v, want exactly one original query", plan.RetrievalQueries)
	}
	if plan.RetrievalQueries[0].Kind != PlannedQueryOriginal || plan.RetrievalQueries[0].Text != "Printer OFFLINE now" {
		t.Fatalf("RetrievalQueries[0] = %+v, want original retrieval text", plan.RetrievalQueries[0])
	}
	for _, stage := range []string{"rewrite", "hyde", "rag_fusion"} {
		if plan.SkipReasons[stage] == "" {
			t.Fatalf("SkipReasons[%q] missing in %+v", stage, plan.SkipReasons)
		}
	}
}

func TestDefaultQueryPlannerDetectsNavigationalQueriesAndDisablesExpansion(t *testing.T) {
	planner := DefaultQueryPlanner{}

	plan, err := planner.Plan(context.Background(), Query{
		CompanyID: 1,
		Text:      "open the ticket page",
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if !plan.IsNavigational {
		t.Fatal("IsNavigational = false, want true")
	}
	for _, stage := range []string{"hyde", "rag_fusion"} {
		if !strings.Contains(plan.SkipReasons[stage], "navigational") {
			t.Fatalf("SkipReasons[%q] = %q, want navigational reason", stage, plan.SkipReasons[stage])
		}
	}
	if len(plan.RetrievalQueries) != 1 || plan.RetrievalQueries[0].Kind != PlannedQueryOriginal {
		t.Fatalf("RetrievalQueries = %+v, want only original query", plan.RetrievalQueries)
	}
}

func TestDefaultQueryPlannerCreatesConfirmedSymptomGraphAnchor(t *testing.T) {
	planner := DefaultQueryPlanner{}

	plan, err := planner.Plan(context.Background(), Query{
		CompanyID: 1,
		Text:      "tray keeps jamming",
		Metadata:  map[string]string{"symptom_node_id": "44"},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if len(plan.GraphAnchors) != 1 {
		t.Fatalf("GraphAnchors = %+v, want one symptom anchor", plan.GraphAnchors)
	}
	if plan.GraphAnchors[0].Kind != GraphAnchorSymptom || plan.GraphAnchors[0].ID != 44 {
		t.Fatalf("GraphAnchors[0] = %+v, want symptom 44", plan.GraphAnchors[0])
	}
	if reason := plan.SkipReasons["graph_anchor"]; reason != "" {
		t.Fatalf("graph_anchor skip reason = %q, want empty when anchor is present", reason)
	}
}

func TestDefaultQueryPlannerUsesAISymptomGraphAnchorAboveConfidence(t *testing.T) {
	planner := DefaultQueryPlanner{}

	plan, err := planner.Plan(context.Background(), Query{
		CompanyID: 1,
		Text:      "tray keeps jamming",
		Metadata: map[string]string{
			"ai_symptom_node_id": "55",
			"ai_symptom_conf":    "0.72",
		},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if len(plan.GraphAnchors) != 1 || plan.GraphAnchors[0].ID != 55 {
		t.Fatalf("GraphAnchors = %+v, want AI symptom 55", plan.GraphAnchors)
	}
}

func TestDefaultQueryPlannerSkipsLowConfidenceAISymptomGraphAnchor(t *testing.T) {
	planner := DefaultQueryPlanner{}

	plan, err := planner.Plan(context.Background(), Query{
		CompanyID: 1,
		Text:      "tray keeps jamming",
		Metadata: map[string]string{
			"ai_symptom_node_id": "55",
			"ai_symptom_conf":    "0.69",
		},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if len(plan.GraphAnchors) != 0 {
		t.Fatalf("GraphAnchors = %+v, want none below confidence gate", plan.GraphAnchors)
	}
	if !strings.Contains(plan.SkipReasons["graph_anchor"], "confidence") {
		t.Fatalf("graph_anchor skip reason = %q, want confidence reason", plan.SkipReasons["graph_anchor"])
	}
}
