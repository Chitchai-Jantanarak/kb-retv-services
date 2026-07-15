package chat

import (
	"encoding/json"
	"testing"

	"github.com/my/app/internal/application/dto"
)

func TestStoreResponseDropsCoverageSensitiveFields(t *testing.T) {
	resp := dto.ChatResponse{
		Reply:         "here are your cases",
		Status:        dto.ChatStatusAnswered,
		SearchResults: []dto.ChatCaseResult{{ID: 1, Title: "secret case"}},
		Case:          &dto.ChatCaseDraft{Problem: "x"},
	}
	sanitized := sanitizeForCache(resp)
	if sanitized.SearchResults != nil {
		t.Fatal("SearchResults must not be cached")
	}
	if sanitized.Case != nil {
		t.Fatal("Case draft must not be cached")
	}
	raw, _ := json.Marshal(sanitized)
	var back dto.ChatResponse
	_ = json.Unmarshal(raw, &back)
	if len(back.SearchResults) != 0 {
		t.Fatal("round-trip leaked SearchResults")
	}
}
