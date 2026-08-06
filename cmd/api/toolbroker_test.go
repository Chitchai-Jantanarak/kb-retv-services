package main

import (
	"context"
	"testing"

	"github.com/my/app/internal/ai/rag"
	"github.com/my/app/internal/application/tools"
)

type stubBrokerFTS struct{}

func (stubBrokerFTS) SearchChunks(context.Context, []int64, string, int) ([]rag.FTSChunk, error) {
	return nil, nil
}

func toolBrokerCatalog() []tools.Tool {
	return []tools.Tool{
		{ID: "f7_knowledge", Handler: "knowledge"},
		{ID: "f1_find_cases", Handler: "reports.find"},
	}
}

func TestToolBrokerRegistersKnowledgeWhenFTSPresent(t *testing.T) {
	t.Parallel()
	b := newToolBroker(toolBrokerDeps{FTS: stubBrokerFTS{}})
	if _, ok := b.Handlers()["knowledge"]; !ok {
		t.Fatal("knowledge handler must be registered when FTS present")
	}
}

func TestToolBrokerSkipsKnowledgeWhenNoFTS(t *testing.T) {
	t.Parallel()
	b := newToolBroker(toolBrokerDeps{})
	if len(b.Handlers()) != 0 {
		t.Fatalf("no deps should register no handlers, got %v", b.Handlers())
	}
}

func TestToolBrokerBoundExposesOnlyRunnableTools(t *testing.T) {
	t.Parallel()
	b := newToolBroker(toolBrokerDeps{FTS: stubBrokerFTS{}})
	bound := b.Bound(toolBrokerCatalog())
	if len(bound) != 1 || bound[0].ID != "f7_knowledge" {
		t.Fatalf("bound = %v, want only f7_knowledge", bound)
	}
	unbound := b.UnboundIDs(toolBrokerCatalog())
	if len(unbound) != 1 || unbound[0] != "f1_find_cases" {
		t.Fatalf("unbound = %v, want [f1_find_cases]", unbound)
	}
}
