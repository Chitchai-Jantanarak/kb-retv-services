package mysql

import (
	"context"
	"strings"
	"testing"

	"github.com/my/app/internal/domain/ports"
)

func TestRecorderValidatesInput(t *testing.T) {
	rec := New(nil)
	cases := []struct {
		name    string
		entry   ports.ActivityEntry
		wantSub string
	}{
		{name: "zero_company", entry: ports.ActivityEntry{ActorType: "channel", Action: "create"}, wantSub: "company_id"},
		{name: "missing_actor_type", entry: ports.ActivityEntry{CompanyID: 1, ActorType: "  ", Action: "create"}, wantSub: "actor_type"},
		{name: "missing_action", entry: ports.ActivityEntry{CompanyID: 1, ActorType: "channel", Action: ""}, wantSub: "action"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := rec.RecordActivity(context.Background(), tc.entry)
			if err == nil {
				t.Fatalf("err = nil, want %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %q, want sub %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestEncodeContextEmptyIsNull(t *testing.T) {
	got, err := encodeContext(nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != nil {
		t.Fatalf("got = %v, want nil", got)
	}

	got, err = encodeContext(map[string]any{})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != nil {
		t.Fatalf("got = %v, want nil for empty map", got)
	}
}

func TestEncodeContextMarshalsToJSON(t *testing.T) {
	got, err := encodeContext(map[string]any{"conversation_id": 7})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	s, ok := got.(string)
	if !ok {
		t.Fatalf("got type %T, want string", got)
	}
	if !strings.Contains(s, `"conversation_id":7`) {
		t.Fatalf("got = %q, want conversation_id", s)
	}
}

func TestNullableHelpers(t *testing.T) {
	if nullableID(0) != nil {
		t.Fatal("nullableID(0) should be nil")
	}
	if nullableID(-3) != nil {
		t.Fatal("nullableID(-3) should be nil")
	}
	if nullableID(5) != int64(5) {
		t.Fatal("nullableID(5) should be 5")
	}
	if nullableText("   ") != nil {
		t.Fatal("blank text should be nil")
	}
	if nullableText(" line-user-1 ") != "line-user-1" {
		t.Fatal("text should be trimmed")
	}
}
