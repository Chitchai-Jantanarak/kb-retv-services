package rag

import (
	"strings"
	"testing"
)

func TestBuildFTSBooleanQueryTermsAreOptional(t *testing.T) {
	got := buildFTSBooleanQuery("reset the robot")
	want := `"reset" "the" "robot"`
	if got != want {
		t.Fatalf("buildFTSBooleanQuery() = %q, want %q", got, want)
	}
}

func TestBuildFTSBooleanQueryDropsSingleCharacterWords(t *testing.T) {
	got := buildFTSBooleanQuery("a robot is x")
	want := `"robot" "is"`
	if got != want {
		t.Fatalf("buildFTSBooleanQuery() = %q, want %q", got, want)
	}
}

func TestBuildFTSBooleanQueryWindowsUnsegmentedRuns(t *testing.T) {
	got := buildFTSBooleanQuery("หุ่นยนต์ล้างจาน")
	terms := strings.Split(got, " ")
	if len(terms) < 2 {
		t.Fatalf("buildFTSBooleanQuery() = %q, want several overlapping windows", got)
	}
	for _, term := range terms {
		if !strings.HasPrefix(term, `"`) || !strings.HasSuffix(term, `"`) {
			t.Fatalf("term %q is not quoted", term)
		}
		if runes := []rune(term); len(runes) != ftsWindowSize+2 {
			t.Fatalf("term %q has %d runes, want a %d-rune window", term, len(runes)-2, ftsWindowSize)
		}
	}
}

func TestBuildFTSBooleanQuerySplitsScripts(t *testing.T) {
	got := buildFTSBooleanQuery("Bella Bot พัง")
	if !strings.Contains(got, `"Bella"`) || !strings.Contains(got, `"Bot"`) {
		t.Fatalf("buildFTSBooleanQuery() = %q, want latin words kept whole", got)
	}
	if !strings.Contains(got, `"พัง"`) {
		t.Fatalf("buildFTSBooleanQuery() = %q, want the short thai run kept whole", got)
	}
}

func TestBuildFTSBooleanQueryRejectsBooleanOperators(t *testing.T) {
	got := buildFTSBooleanQuery(`robot" -exclude +(force) ~near*`)
	for _, bad := range []string{`-`, `~`, `*`, `(`, `)`, `+`} {
		if strings.Contains(got, bad) {
			t.Fatalf("buildFTSBooleanQuery() = %q, leaked operator %q", got, bad)
		}
	}
	if strings.Count(got, `"`) != 2*len(strings.Split(got, " ")) {
		t.Fatalf("buildFTSBooleanQuery() = %q, quotes are unbalanced", got)
	}
}

func TestBuildFTSBooleanQueryEmptyForTermlessInput(t *testing.T) {
	for _, in := range []string{"", "   ", "!!! ??", "a"} {
		if got := buildFTSBooleanQuery(in); got != "" {
			t.Fatalf("buildFTSBooleanQuery(%q) = %q, want empty so the caller matches nothing", in, got)
		}
	}
}

func TestBuildFTSBooleanQueryCapsTermCount(t *testing.T) {
	got := buildFTSBooleanQuery(strings.Repeat("robot ", 100))
	if n := len(strings.Split(got, " ")); n != ftsMaxTerms {
		t.Fatalf("term count = %d, want %d", n, ftsMaxTerms)
	}
}
