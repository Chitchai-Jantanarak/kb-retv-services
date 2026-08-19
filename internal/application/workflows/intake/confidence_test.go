package intake

import "testing"

func boolPtr(b bool) *bool { return &b }

func TestConfidence(t *testing.T) {
	cases := []struct {
		name           string
		score          int
		reasons        []string
		classification string
		missing        []string
		catalog        *bool
		refcase        string
		wantConf       int
		wantPromote    bool
	}{
		{
			name: "new_issue clean no evidence (conv628)",
			score: 50, classification: ClassificationNewIssue,
			missing: []string{"product"}, refcase: "",
			wantConf: 65, wantPromote: true,
		},
		{
			name: "new_issue full evidence",
			score: 50, classification: ClassificationNewIssue,
			missing: []string{}, refcase: "REP-1001",
			wantConf: 93, wantPromote: true,
		},
		{
			name: "new_issue strong-junk conflict held",
			score: 27, reasons: []string{ReasonHasAttachments, ReasonAutoSubmitted},
			classification: ClassificationNewIssue, missing: []string{"product"},
			wantConf: 35, wantPromote: false,
		},
		{
			name: "not_actionable junk floored",
			score: 5, reasons: []string{ReasonListUnsubscribe},
			classification: ClassificationNotActionable, missing: []string{},
			wantConf: 5, wantPromote: false,
		},
		{
			name: "status_query with refcase never promotes",
			score: 50, classification: ClassificationStatusQuery,
			missing: []string{}, refcase: "REP-2002",
			wantConf: 68, wantPromote: false,
		},
		{
			name: "new_issue catalog veto borderline",
			score: 50, classification: ClassificationNewIssue,
			missing: []string{"product"}, catalog: boolPtr(false),
			wantConf: 55, wantPromote: true,
		},
		{
			name: "unknown classification default",
			score: 30, classification: "",
			missing: []string{"product"},
			wantConf: 30, wantPromote: false,
		},
		{
			name: "precedence_bulk is not strong junk",
			score: 44, reasons: []string{ReasonPrecedenceBulk},
			classification: ClassificationNewIssue, missing: []string{"product"},
			wantConf: 60, wantPromote: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			conf, trail := Confidence(c.score, c.reasons, c.classification, c.missing, c.catalog, c.refcase)
			if conf != c.wantConf {
				t.Errorf("conf = %d, want %d (trail %v)", conf, c.wantConf, trail)
			}
			if got := ConfidencePromotable(c.classification, conf); got != c.wantPromote {
				t.Errorf("promotable = %v, want %v", got, c.wantPromote)
			}
		})
	}
}
