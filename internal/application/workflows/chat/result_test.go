package chat

import (
	"testing"

	"github.com/my/app/internal/application/skeleton"
)

func TestToolChatResponseUsesLocaleHeadlineVariant(t *testing.T) {
	r := skeleton.Response{
		Matched:  true,
		ToolID:   "f1_find_cases",
		Headline: "latest case is REP-4106",
		HeadlineVariants: map[string]string{
			"th": "เคสล่าสุดคือ REP-4106",
			"en": "latest case is REP-4106",
		},
	}

	got := toolChatResponse("th", r)
	if got.Reply != "เคสล่าสุดคือ REP-4106" {
		t.Fatalf("reply = %q, want the th variant", got.Reply)
	}
}

func TestToolChatResponseFallsBackToDefaultHeadlineWithoutLocaleMatch(t *testing.T) {
	r := skeleton.Response{
		Matched:  true,
		ToolID:   "f1_find_cases",
		Headline: "latest case is REP-4106",
		HeadlineVariants: map[string]string{
			"en": "latest case is REP-4106",
		},
	}

	got := toolChatResponse("th", r)
	if got.Reply != "latest case is REP-4106" {
		t.Fatalf("reply = %q, want fallback to default Headline when locale has no variant", got.Reply)
	}
}

func TestToolChatResponsePlainHeadlineUnchangedAcrossLocales(t *testing.T) {
	r := skeleton.Response{
		Matched:  true,
		ToolID:   "f4_workload",
		Headline: "team workload",
	}

	th := toolChatResponse("th", r)
	en := toolChatResponse("en", r)
	if th.Reply != "team workload" || en.Reply != "team workload" {
		t.Fatalf("plain headline must stay byte-identical regardless of locale, got th=%q en=%q", th.Reply, en.Reply)
	}
}
