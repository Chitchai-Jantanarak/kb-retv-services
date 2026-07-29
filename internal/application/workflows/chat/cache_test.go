package chat

import (
	"testing"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/shared/ctxkey"
)

func TestCacheKeyDiffersByPrincipalCoverage(t *testing.T) {
	req := dto.ChatRequest{Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "hello"}}}
	a := ctxkey.Principal{UserID: 1, Role: "agent", Coverage: []int64{1, 2}}
	b := ctxkey.Principal{UserID: 1, Role: "agent", Coverage: []int64{1, 3}}

	keyA := cacheKey(7, a, req)
	keyB := cacheKey(7, b, req)
	if keyA == keyB {
		t.Fatalf("cacheKey must differ when principal coverage differs: both = %q", keyA)
	}
}

func TestCoverageKeyOrderIndependentAndNonMutating(t *testing.T) {
	coverage := []int64{3, 1, 2}
	got := coverageKey(coverage)
	if got != "1,2,3" {
		t.Fatalf("coverageKey = %q, want 1,2,3", got)
	}
	if coverage[0] != 3 || coverage[1] != 1 || coverage[2] != 2 {
		t.Fatalf("coverageKey must not mutate caller slice, got %v", coverage)
	}
}

func TestCacheableOnlyForSourcedAnsweredReplies(t *testing.T) {
	sourced := dto.ChatResponse{Status: dto.ChatStatusAnswered, Sources: []dto.ChatSource{{ID: "chunk:1"}}}
	if !cacheable(sourced) {
		t.Fatal("answered response with sources must be cacheable")
	}

	noSources := dto.ChatResponse{Status: dto.ChatStatusAnswered}
	if cacheable(noSources) {
		t.Fatal("answered response without sources must not be cacheable")
	}

	withCase := sourced
	withCase.Case = &dto.ChatCaseDraft{Problem: "p"}
	if cacheable(withCase) {
		t.Fatal("response carrying a case draft must not be cacheable")
	}

	withSearch := sourced
	withSearch.SearchResults = []dto.ChatCaseResult{{ID: 1}}
	if cacheable(withSearch) {
		t.Fatal("response carrying search results must not be cacheable")
	}

	notAnswered := sourced
	notAnswered.Status = dto.ChatStatusNoResults
	if cacheable(notAnswered) {
		t.Fatal("non-answered status must not be cacheable")
	}
}
