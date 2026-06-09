package mysql

import (
	"context"
	"strings"
	"testing"

	"github.com/my/app/internal/domain/ports"
)

func TestOutboxWriterValidatesInput(t *testing.T) {
	w := NewOutboxWriter(nil)
	cases := []struct {
		name    string
		item    ports.ReviewItem
		wantSub string
	}{
		{name: "zero_company", item: ports.ReviewItem{Kind: ports.ReviewKindSymptomPropose, Payload: map[string]any{"x": 1}}, wantSub: "company_id"},
		{name: "empty_kind", item: ports.ReviewItem{CompanyID: 1, Payload: map[string]any{"x": 1}}, wantSub: "kind"},
		{name: "nil_payload", item: ports.ReviewItem{CompanyID: 1, Kind: ports.ReviewKindSymptomPropose}, wantSub: "payload"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := w.Enqueue(context.Background(), tc.item)
			if err == nil {
				t.Fatalf("err = nil, want %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %q, want sub %q", err.Error(), tc.wantSub)
			}
		})
	}
}
