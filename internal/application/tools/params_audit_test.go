package tools

import (
	"context"
	"errors"
	"testing"
)

func TestMatchLookupNameRequiresWordBoundary(t *testing.T) {
	if got := matchLookupName([]string{"Ann"}, "show annual leave policy"); got != "" {
		t.Fatalf("got %q, want empty: a name must not match inside a longer word", got)
	}
	if got := matchLookupName([]string{"Ann"}, "ask ann about it"); got != "Ann" {
		t.Fatalf("got %q, want Ann: a real word-boundary match must still resolve", got)
	}
}

func TestMatchLookupNameUnsegmentedScriptKeepsSubstring(t *testing.T) {
	if got := matchLookupName([]string{"หุ่นยนต์"}, "หุ่นยนต์พัง"); got != "หุ่นยนต์" {
		t.Fatalf("got %q, want the thai name: no word delimiter exists to anchor to", got)
	}
}

func TestMatchLookupNameRejectsGenericToken(t *testing.T) {
	if got := matchLookupName([]string{"Pudu Bot"}, "reset the bot please"); got != "" {
		t.Fatalf("got %q, want empty: %q is not the distinguishing token of the name", got, "bot")
	}
}

func TestMatchLookupNameKeepsDistinctiveToken(t *testing.T) {
	names := []string{"Siam Michelin HQ", "Kantary Hotel"}
	if got := matchLookupName(names, "customer profile for michelin"); got != "Siam Michelin HQ" {
		t.Fatalf("got %q, want Siam Michelin HQ: the longest token carries the name", got)
	}
}

type failingNames struct{ err error }

func (f failingNames) Names(context.Context, string) ([]string, error) { return nil, f.err }

func TestExtractParamsPropagatesLookupError(t *testing.T) {
	boom := errors.New("tenant database unreachable")
	s := &Selector{names: failingNames{err: boom}}
	params, err := compileParams([]Param{{Name: "product", Required: true, Lookup: "nodes.title"}})
	if err != nil {
		t.Fatalf("compileParams: %v", err)
	}
	if _, _, err := s.extractParams(context.Background(), params, "cases for bella bot"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the lookup failure surfaced, not a silently unmatched param", err)
	}
}
