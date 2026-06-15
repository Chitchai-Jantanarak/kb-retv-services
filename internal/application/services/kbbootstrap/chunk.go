package kbbootstrap

import (
	"github.com/my/app/internal/shared/htmltext"
	"strings"
)

const (
	DefaultChunkRunes   = 1200
	MaxArticleBodyBytes = 60000
)

type ReportRecord struct {
	ID            int64
	CompanyID     int64
	Code          string
	Title         string
	CustomerName  string
	SiteName      string
	StatusName    string
	WorkTypeName  string
	SeverityName  string
	ProblemFull   string
	ProblemDetail string
	FixProblem    string
}

func BuildArticleTitle(report ReportRecord) string {
	code := strings.TrimSpace(report.Code)
	title := strings.TrimSpace(report.Title)
	switch {
	case code != "" && title != "":
		return code + " - " + title
	case title != "":
		return title
	case code != "":
		return code
	default:
		return "Report"
	}
}

func BuildArticleBody(report ReportRecord) string {
	parts := make([]string, 0, 10)
	add := func(label string, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, label+": "+value)
		}
	}

	add("Report", report.Code)
	add("Title", report.Title)
	add("Customer", report.CustomerName)
	add("Site", report.SiteName)
	add("Status", report.StatusName)
	add("Work type", report.WorkTypeName)
	add("Severity", report.SeverityName)
	add("Problem", htmltext.StripInlineImages(report.ProblemFull))
	add("Detail", htmltext.StripInlineImages(report.ProblemDetail))
	add("Resolution", htmltext.StripInlineImages(report.FixProblem))

	return strings.Join(parts, "\n")
}

func ChunkText(text string, maxRunes int) []string {
	if maxRunes <= 0 {
		maxRunes = DefaultChunkRunes
	}
	if text == "" {
		return nil
	}

	runes := []rune(text)
	chunks := make([]string, 0, (len(runes)/maxRunes)+1)
	for start := 0; start < len(runes); start += maxRunes {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}

func LimitUTF8Bytes(text string, maxBytes int) string {
	if maxBytes <= 0 || len(text) <= maxBytes {
		return text
	}

	used := 0
	var out strings.Builder
	for _, r := range text {
		next := used + len(string(r))
		if next > maxBytes {
			break
		}
		out.WriteRune(r)
		used = next
	}
	return out.String()
}
