package memgraph

import (
	"context"
	"strings"
	"testing"

	"github.com/my/app/internal/domain/ports"
)

func TestCompanyPropsAddsCompanyID(t *testing.T) {
	props := CompanyProps(42, map[string]any{"name": "test"})
	if props["company_id"] != int64(42) {
		t.Fatalf("company_id = %v, want 42", props["company_id"])
	}
	if props["name"] != "test" {
		t.Fatalf("name = %v, want test", props["name"])
	}
}

func TestExternalIDFormatsCorrectly(t *testing.T) {
	id := ExternalID(3, "report", "100")
	if id != "3:report:100" {
		t.Fatalf("ExternalID = %q, want 3:report:100", id)
	}
}

func TestUpsertNodeRejectsBadCompanyID(t *testing.T) {
	store := &Store{}
	cases := []struct {
		name string
		node ports.Node
		want string
	}{
		{
			name: "zero_company",
			node: ports.Node{CompanyID: 0, ID: "x", Labels: []string{"KbArticle"}},
			want: "company_id must be positive",
		},
		{
			name: "negative_company",
			node: ports.Node{CompanyID: -1, ID: "x", Labels: []string{"KbArticle"}},
			want: "company_id must be positive",
		},
		{
			name: "missing_label_after_valid_company",
			node: ports.Node{CompanyID: 7, ID: "x"},
			want: "at least one label",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := store.UpsertNode(context.Background(), tc.node)
			if err == nil {
				t.Fatalf("UpsertNode(%+v) error = nil, want %q", tc.node, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestUpsertEdgeRejectsBadCompanyID(t *testing.T) {
	store := &Store{}
	cases := []struct {
		name string
		edge ports.Edge
	}{
		{name: "zero_company", edge: ports.Edge{CompanyID: 0, From: "a", To: "b", Type: "SOLVES"}},
		{name: "negative_company", edge: ports.Edge{CompanyID: -5, From: "a", To: "b", Type: "SOLVES"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := store.UpsertEdge(context.Background(), tc.edge)
			if err == nil {
				t.Fatalf("UpsertEdge(%+v) error = nil, want company_id error", tc.edge)
			}
			if !strings.Contains(err.Error(), "company_id must be positive") {
				t.Fatalf("error = %q, want company_id substring", err.Error())
			}
		})
	}
}
