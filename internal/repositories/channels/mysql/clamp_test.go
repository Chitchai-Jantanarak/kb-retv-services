package mysql

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestClampMessageBodyLeavesNormalBodyAlone(t *testing.T) {
	body := "the robot will not leave the dock"
	if got := clampMessageBody(body); got != body {
		t.Fatalf("got %q, want unchanged", got)
	}
}

func TestClampMessageBodyFitsTextColumn(t *testing.T) {
	// Multibyte on purpose: a naive byte cut splits a rune and corrupts the row.
	body := strings.Repeat("ก", maxMessageBodyBytes/3+10)
	got := clampMessageBody(body)

	if len(got) > maxMessageBodyBytes+len("\n[truncated]") {
		t.Fatalf("clamped body is %d bytes, still too wide", len(got))
	}
	if !utf8.ValidString(got) {
		t.Fatal("clamped body is not valid utf8")
	}
	if !strings.HasSuffix(got, "[truncated]") {
		t.Fatal("truncation must be visible in the stored body")
	}
}
