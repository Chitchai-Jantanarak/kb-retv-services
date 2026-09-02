package rag

import "testing"

func TestExtractJSONObject(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", `{"verdict":"relevant"}`, `{"verdict":"relevant"}`},
		{"padded", "  {\"a\":1}\n", `{"a":1}`},
		{"preamble", "Here is the JSON requested:\n{\"a\":1}", `{"a":1}`},
		{"fenced", "```json\n{\"a\":1}\n```", `{"a":1}`},
		{"preamble and fence", "Sure!\n```\n{\"a\":1}\n```\n", `{"a":1}`},
		{"nested", `x {"a":{"b":2}} y`, `{"a":{"b":2}}`},
		{"brace in string", `x {"a":"}"} y`, `{"a":"}"}`},
		{"escaped quote in string", `x {"a":"\"}"} y`, `{"a":"\"}"}`},
		{"no object returns input", "not json at all", "not json at all"},
		{"unbalanced returns input", `{"a":1`, `{"a":1`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extractJSONObject(c.in); got != c.want {
				t.Fatalf("extractJSONObject(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
