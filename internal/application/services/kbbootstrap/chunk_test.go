package kbbootstrap

import "testing"

func TestBuildArticleBodyIncludesReportFieldsAndMetadata(t *testing.T) {
	report := ReportRecord{
		Code:          "REP-7",
		Title:         "Order not received",
		CustomerName:  "Demo Customer",
		SiteName:      "Kitchen 1",
		StatusName:    "open",
		WorkTypeName:  "support",
		SeverityName:  "high",
		ProblemFull:   "Cannot place order.",
		ProblemDetail: "Battery warning appears.",
		FixProblem:    "Reset battery connector.",
	}

	body := BuildArticleBody(report)

	for _, want := range []string{
		"Report: REP-7",
		"Title: Order not received",
		"Customer: Demo Customer",
		"Site: Kitchen 1",
		"Status: open",
		"Work type: support",
		"Severity: high",
		"Problem: Cannot place order.",
		"Detail: Battery warning appears.",
		"Resolution: Reset battery connector.",
	} {
		if !contains(body, want) {
			t.Fatalf("body missing %q in %q", want, body)
		}
	}
}

func TestChunkTextKeepsUnicodeAndSizeBound(t *testing.T) {
	text := "ภาษาไทยทดสอบระบบ order check ต้องตรวจสอบ network และ battery ก่อน escalation"

	chunks := ChunkText(text, 24)

	if len(chunks) < 3 {
		t.Fatalf("len(chunks) = %d, want at least 3", len(chunks))
	}
	for _, chunk := range chunks {
		if got := len([]rune(chunk)); got > 24 {
			t.Fatalf("chunk length = %d, want <= 24: %q", got, chunk)
		}
	}
	joined := ""
	for _, chunk := range chunks {
		joined += chunk
	}
	if joined != text {
		t.Fatalf("joined chunks = %q, want %q", joined, text)
	}
}

func TestLimitUTF8BytesDoesNotSplitRune(t *testing.T) {
	text := "abcภาษาไทย"

	limited := LimitUTF8Bytes(text, 6)

	if limited != "abcภ" {
		t.Fatalf("limited = %q, want abcภ", limited)
	}
	if len(limited) > 6 {
		t.Fatalf("byte length = %d, want <= 6", len(limited))
	}
}

func contains(s string, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
