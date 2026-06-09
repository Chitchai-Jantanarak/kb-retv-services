package mysql

import (
	"context"
	"strings"
	"testing"
)

func TestAgentLookupRejectsBadCompanyID(t *testing.T) {
	l := NewAgentLookup(nil, "")
	cases := []struct {
		name      string
		companyID int64
	}{
		{name: "zero", companyID: 0},
		{name: "negative", companyID: -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := l.AgentFor(context.Background(), tc.companyID)
			if err == nil {
				t.Fatal("err = nil, want company_id error")
			}
			if !strings.Contains(err.Error(), "company_id must be positive") {
				t.Fatalf("err = %q", err.Error())
			}
		})
	}
}
