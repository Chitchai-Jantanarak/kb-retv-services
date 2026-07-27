package dto

import (
	"strings"
	"testing"

	apperr "github.com/my/app/internal/shared/errors"
)

func TestSearchRequestValidateAppliesDefaults(t *testing.T) {
	req := SearchRequest{Query: "  printer offline  "}

	if err := req.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if req.Query != "printer offline" {
		t.Fatalf("Query = %q, want trimmed printer offline", req.Query)
	}
	if req.Limit != 10 {
		t.Fatalf("Limit = %d, want 10", req.Limit)
	}
	if len(req.Types) != 1 || req.Types[0] != SearchTypeKB {
		t.Fatalf("Types = %v, want [kb]", req.Types)
	}
}

func TestSearchRequestValidateRejectsEmptyQuery(t *testing.T) {
	req := SearchRequest{Query: "   "}
	err := req.Validate()
	appErr, ok := apperr.As(err)
	if !ok || appErr.Code != apperr.CodeInvalidInput {
		t.Fatalf("Validate() error = %v, want invalid_input", err)
	}
}

func TestSearchRequestValidateRejectsOversizeQuery(t *testing.T) {
	req := SearchRequest{Query: strings.Repeat("a", 4001)}
	err := req.Validate()
	appErr, ok := apperr.As(err)
	if !ok || appErr.Code != apperr.CodeInvalidInput {
		t.Fatalf("Validate() error = %v, want invalid_input", err)
	}
}

func TestSearchRequestValidateRejectsBadLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit int
	}{
		{"too large", 51},
		{"negative", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := SearchRequest{Query: "printer", Limit: tt.limit}
			err := req.Validate()
			appErr, ok := apperr.As(err)
			if !ok || appErr.Code != apperr.CodeInvalidInput {
				t.Fatalf("Validate() error = %v, want invalid_input", err)
			}
		})
	}
}

func TestSearchRequestValidateRejectsUnknownType(t *testing.T) {
	req := SearchRequest{Query: "printer", Types: []string{"kb", "web"}}
	err := req.Validate()
	if err == nil || !strings.Contains(err.Error(), "unsupported type: web") {
		t.Fatalf("Validate() error = %v, want unsupported type: web", err)
	}
	appErr, ok := apperr.As(err)
	if !ok || appErr.Code != apperr.CodeInvalidInput {
		t.Fatalf("Validate() error code = %v, want invalid_input", err)
	}
}

func TestSearchRequestValidateAcceptsExplicitLimitInRange(t *testing.T) {
	req := SearchRequest{Query: "printer", Limit: 50}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if req.Limit != 50 {
		t.Fatalf("Limit = %d, want 50", req.Limit)
	}
}
