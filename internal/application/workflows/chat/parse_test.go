package chat

import (
	"testing"

	"github.com/my/app/internal/application/dto"
)

func TestParseTurnStructuredOK(t *testing.T) {
	_, ok := parseTurn(`{"reply":"hi","case":null,"search":null}`)
	if !ok {
		t.Fatal("valid JSON must report structuredOK=true")
	}
}

func TestParseTurnRawFallbackNotStructured(t *testing.T) {
	turn, ok := parseTurn("just some prose the model emitted")
	if ok {
		t.Fatal("raw prose must report structuredOK=false")
	}
	if turn.Reply != "just some prose the model emitted" {
		t.Fatalf("reply not preserved: %q", turn.Reply)
	}
}

func TestNormalizeSearchBlanksProseQuery(t *testing.T) {
	req := normalizeSearch(&dto.ChatSearchRequest{Query: "latest incoming cases"})
	if req == nil {
		t.Fatal("normalizeSearch(prose query) = nil, want a surviving unfiltered search")
	}
	if req.Query != "" {
		t.Fatalf("Query = %q, want blanked prose", req.Query)
	}
	if req.Limit != chatSearchLimit {
		t.Fatalf("Limit = %d, want default %d", req.Limit, chatSearchLimit)
	}
}

func TestNormalizeSearchKeepsCaseCode(t *testing.T) {
	req := normalizeSearch(&dto.ChatSearchRequest{Query: "rep-606"})
	if req == nil {
		t.Fatal("normalizeSearch(case code) = nil")
	}
	if req.Query != "REP-606" {
		t.Fatalf("Query = %q, want REP-606", req.Query)
	}
}
