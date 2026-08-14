package textnorm

import (
	"strings"
	"testing"
)

func TestNormalizeLoanwords(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		wantHas    []string
		wantNotHas []string
	}{
		{
			name:       "thai transliteration report and draft",
			input:      "ขอรีพอรตล่าสุด ไม่เอาดราฟ",
			wantHas:    []string{"report", "draft"},
			wantNotHas: []string{"รีพอรต", "ดราฟ"},
		},
		{
			name:    "latin code-switch unchanged",
			input:   "ขอ report ล่าสุด",
			wantHas: []string{"ขอ", "report", "ล่าสุด"},
		},
		{
			name:    "no loanwords unchanged",
			input:   "สวัสดีครับ",
			wantHas: []string{"สวัสดีครับ"},
		},
		{
			name:       "draft with tone mark suffix",
			input:      "ดราฟท์",
			wantHas:    []string{"draft"},
			wantNotHas: []string{"draftท์"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NormalizeLoanwords(c.input)
			for _, want := range c.wantHas {
				if !strings.Contains(got, want) {
					t.Errorf("NormalizeLoanwords(%q) = %q, want contains %q", c.input, got, want)
				}
			}
			for _, notWant := range c.wantNotHas {
				if strings.Contains(got, notWant) {
					t.Errorf("NormalizeLoanwords(%q) = %q, want not contains %q", c.input, got, notWant)
				}
			}
		})
	}
}
