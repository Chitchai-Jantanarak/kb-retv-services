package skeleton

import (
	"context"
	"strings"
	"testing"

	"github.com/my/app/internal/application/tools"
)

type fieldValueHandler struct{}

func (fieldValueHandler) Run(_ context.Context, _ Query) ([]Row, error) {
	return []Row{
		{"field": "title", "value": "wifi down"},
		{"field": "status", "value": "waiting"},
	}, nil
}

func i18nTool() tools.Tool {
	return tools.Tool{
		ID:      "f1_find_cases",
		Handler: "reports.find",
		Audit:   "ai.find_cases",
		Compose: tools.Compose{
			Mode:     "template",
			Headline: "latest is {code}",
			HeadlineI18n: map[string]string{
				"th": "เคสล่าสุดคือ {code}",
				"en": "latest case is {code}",
			},
			Row:  "{code}",
			Cite: "code",
		},
	}
}

func TestComposeHeadlineVariantsRenderFromFirstRow(t *testing.T) {
	resp := composeFor(t, i18nTool(), []Row{
		{"code": "REP-4106"},
		{"code": "REP-4105"},
	})
	if resp.HeadlineVariants["th"] != "เคสล่าสุดคือ REP-4106" {
		t.Fatalf("th variant = %q", resp.HeadlineVariants["th"])
	}
	if resp.HeadlineVariants["en"] != "latest case is REP-4106" {
		t.Fatalf("en variant = %q", resp.HeadlineVariants["en"])
	}
	if resp.Headline != "latest is REP-4106" {
		t.Fatalf("default Headline = %q, want default template still filled from first row", resp.Headline)
	}
}

func TestComposeHeadlineVariantsNilWithoutI18n(t *testing.T) {
	resp := composeFor(t, findTool(), []Row{{"code": "R1"}})
	if resp.HeadlineVariants != nil {
		t.Fatalf("HeadlineVariants = %v, want nil when no headline_i18n is defined (f4-style tools stay byte-identical)", resp.HeadlineVariants)
	}
}

func TestComposeHeadlineVariantsNeverLeakPlaceholders(t *testing.T) {
	resp := composeFor(t, i18nTool(), nil)
	for locale, headline := range resp.HeadlineVariants {
		if strings.Contains(headline, "{") {
			t.Fatalf("variant %q leaked a placeholder: %q", locale, headline)
		}
	}
}

func TestComposeHeadlineFallsBackToSelectionParams(t *testing.T) {
	tool := tools.Tool{
		ID:      "f2_case_status",
		Handler: "reports.track",
		Audit:   "ai.case_status",
		Compose: tools.Compose{
			Mode:     "template",
			Headline: "case {code}",
			Row:      "{field}|{value}",
			Cite:     "code",
		},
	}
	sel := fakeSelector{sel: tools.Selection{
		ToolID:  tool.ID,
		Matched: true,
		Params:  map[string]string{"code": "REP-4106"},
	}}
	o := New(sel, []tools.Tool{tool}, map[string]Handler{tool.Handler: fieldValueHandler{}}, &fakeAuditor{})

	resp, err := o.Handle(context.Background(), Actor{CompanyID: 3, Coverage: []int64{3}}, "status of REP-4106")
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.Headline != "case REP-4106" {
		t.Fatalf("headline = %q, want the code filled from selection params when the row itself has no code field", resp.Headline)
	}
}
