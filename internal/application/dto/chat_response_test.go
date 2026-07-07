package dto

import (
	"encoding/json"
	"testing"
)

func TestChatResponseSerializesStatusSourcesActivity(t *testing.T) {
	resp := ChatResponse{
		Reply:    "answer",
		Status:   ChatStatusAnswered,
		Sources:  []ChatSource{{ID: "chunk:1", Title: "KB", Snippet: "steps", Source: "fts", Score: 4.2}},
		Activity: []ChatActivity{{Code: "searched_knowledge", Label: "ค้นหาแหล่งอ้างอิง"}},
	}

	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got["status"] != "answered" {
		t.Fatalf("status = %v, want answered", got["status"])
	}
	sources, ok := got["sources"].([]any)
	if !ok || len(sources) != 1 {
		t.Fatalf("sources = %#v, want one structured source", got["sources"])
	}
	first := sources[0].(map[string]any)
	if first["id"] != "chunk:1" || first["title"] != "KB" {
		t.Fatalf("source = %#v", first)
	}
}
