package chat

import "testing"

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
