package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/my/app/internal/domain/ports"
)

type fakeLLM struct {
	json string
	err  error
}

func (f *fakeLLM) Generate(ctx context.Context, p ports.Prompt) (ports.Completion, error) {
	return ports.Completion{}, nil
}

func (f *fakeLLM) GenerateJSON(ctx context.Context, p ports.Prompt) (ports.Completion, error) {
	if f.err != nil {
		return ports.Completion{}, f.err
	}
	return ports.Completion{Text: f.json}, nil
}

func (f *fakeLLM) Stream(ctx context.Context, p ports.Prompt) (<-chan ports.Completion, error) {
	return nil, nil
}

func modelSelectorCatalog() []Tool {
	return []Tool{
		{
			ID:      "f1_find_cases",
			Kind:    "read",
			Handler: "reports.find",
			Params:  []Param{{Name: "customer_name"}},
			RBAC:    RBAC{RequiresPermission: "report.view"},
		},
		{
			ID:      "f9_delete_case",
			Kind:    "write",
			Handler: "reports.delete",
			Params:  []Param{{Name: "case_code"}},
			RBAC:    RBAC{RequiresPermission: "report.delete"},
		},
	}
}

func TestModelSelectorNoToolAbstains(t *testing.T) {
	llm := &fakeLLM{json: `{"tool_id":"no_tool","params":{}}`}
	sel := NewModelSelector(llm, modelSelectorCatalog())

	got, err := sel.Select(context.Background(), "hello", []string{"report.view"})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if got.Matched {
		t.Fatalf("Select() = %+v, want Matched=false", got)
	}
}

func TestModelSelectorUnknownToolIsError(t *testing.T) {
	llm := &fakeLLM{json: `{"tool_id":"not_a_real_tool","params":{}}`}
	sel := NewModelSelector(llm, modelSelectorCatalog())

	got, err := sel.Select(context.Background(), "do the thing", []string{"report.view"})
	if err == nil {
		t.Fatalf("Select() error = nil, want error for unknown tool")
	}
	if got.Matched {
		t.Fatalf("Select() = %+v, want Matched=false", got)
	}
}

func TestModelSelectorDropsUndeclaredParams(t *testing.T) {
	llm := &fakeLLM{json: `{"tool_id":"f1_find_cases","params":{"customer_name":"Somchai","bogus_param":"x"}}`}
	sel := NewModelSelector(llm, modelSelectorCatalog())

	got, err := sel.Select(context.Background(), "find cases for Somchai", []string{"report.view"})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if !got.Matched || got.ToolID != "f1_find_cases" {
		t.Fatalf("Select() = %+v, want matched f1_find_cases", got)
	}
	if _, ok := got.Params["bogus_param"]; ok {
		t.Fatalf("Select() params = %+v, want bogus_param dropped", got.Params)
	}
	if got.Params["customer_name"] != "Somchai" {
		t.Fatalf("Select() params = %+v, want customer_name preserved", got.Params)
	}
}

func TestModelSelectorRejectsToolOutsidePermission(t *testing.T) {
	llm := &fakeLLM{json: `{"tool_id":"f9_delete_case","params":{"case_code":"REP-1"}}`}
	sel := NewModelSelector(llm, modelSelectorCatalog())

	got, err := sel.Select(context.Background(), "delete case REP-1", []string{"report.view"})
	if err == nil {
		t.Fatalf("Select() error = nil, want error: tool not offered without permission")
	}
	if got.Matched {
		t.Fatalf("Select() = %+v, want Matched=false", got)
	}
}

func TestModelSelectorPropagatesLLMError(t *testing.T) {
	llm := &fakeLLM{err: errors.New("upstream boom")}
	sel := NewModelSelector(llm, modelSelectorCatalog())

	_, err := sel.Select(context.Background(), "find cases", []string{"report.view"})
	if err == nil {
		t.Fatalf("Select() error = nil, want propagated upstream error")
	}
}
