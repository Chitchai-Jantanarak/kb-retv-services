package llm

import (
	"strings"
	"testing"
)

func TestResolveFailureCases(t *testing.T) {
	cases := []struct {
		name    string
		s       Settings
		wantSub string
	}{
		{
			name:    "missing_vendor",
			s:       Settings{},
			wantSub: "vendor is required",
		},
		{
			name:    "unknown_vendor",
			s:       Settings{Vendor: "cohere"},
			wantSub: `unsupported vendor "cohere"`,
		},
		{
			name:    "openai_missing_key",
			s:       Settings{Vendor: "openai"},
			wantSub: "api key is required",
		},
		{
			name:    "openrouter_missing_key",
			s:       Settings{Vendor: "openrouter"},
			wantSub: "OpenRouter",
		},
		{
			name:    "anthropic_missing_key",
			s:       Settings{Vendor: "anthropic"},
			wantSub: "api key is required",
		},
		{
			name:    "claude_alias_missing_key",
			s:       Settings{Vendor: "claude"},
			wantSub: "api key is required",
		},
		{
			name:    "gemini_missing_key",
			s:       Settings{Vendor: "gemini"},
			wantSub: "api key is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Resolve(tc.s)
			if err == nil {
				t.Fatalf("Resolve(%+v) err = nil, want %q", tc.s, tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestResolveSuccessCases(t *testing.T) {
	cases := []struct {
		name string
		s    Settings
	}{
		{name: "openai", s: Settings{Vendor: "openai", OpenAIKey: "k", Model: "gpt-4o-mini"}},
		{name: "openrouter_reuses_openai_client", s: Settings{Vendor: "openrouter", OpenRouterKey: "or-k", Model: "google/gemini-2.0-flash-exp:free"}},
		{name: "anthropic", s: Settings{Vendor: "anthropic", AnthropicKey: "k"}},
		{name: "claude_alias", s: Settings{Vendor: "claude", AnthropicKey: "k"}},
		{name: "gemini", s: Settings{Vendor: "Gemini", GeminiKey: "k"}}, // case-insensitive
		{name: "local_no_key", s: Settings{Vendor: "local", LocalURL: "http://localhost:11434/v1", Model: "llama3"}},
		{name: "local_with_key", s: Settings{Vendor: "LOCAL", LocalKey: "k", Model: "llama3"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Resolve(tc.s)
			if err != nil {
				t.Fatalf("Resolve(%+v) err = %v", tc.s, err)
			}
			if p == nil {
				t.Fatal("provider = nil")
			}
		})
	}
}
