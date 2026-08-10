package tools

import "testing"

func TestParseCaseRef(t *testing.T) {
	cases := []struct {
		text     string
		wantOK   bool
		wantNS   CaseRefNamespace
		wantID   int64
		wantCode string
	}{
		{"REP-4105", true, CaseRefRep, 4105, "REP-4105"},
		{"rep-4105", true, CaseRefRep, 4105, "REP-4105"},
		{"REP4105", true, CaseRefRep, 4105, "REP-4105"},
		{"rep4105", true, CaseRefRep, 4105, "REP-4105"},
		{"rep:4105", true, CaseRefRep, 4105, "REP-4105"},
		{"rep 4105", true, CaseRefRep, 4105, "REP-4105"},
		{"where is rep 4105", true, CaseRefRep, 4105, "REP-4105"},
		{"draft:45", true, CaseRefDraft, 45, "DRAFT-45"},
		{"draft45", true, CaseRefDraft, 45, "DRAFT-45"},
		{"DRAFT-45", true, CaseRefDraft, 45, "DRAFT-45"},
		{"draft 45", true, CaseRefDraft, 45, "DRAFT-45"},
		{"diagnose draft45", true, CaseRefDraft, 45, "DRAFT-45"},
		{"preparation", false, "", 0, ""},
		{"prep4105", false, "", 0, ""},
		{"rep", false, "", 0, ""},
		{"draft:", false, "", 0, ""},
		{"nothing here", false, "", 0, ""},
	}
	for _, c := range cases {
		got, ok := ParseCaseRef(c.text)
		if ok != c.wantOK {
			t.Fatalf("ParseCaseRef(%q) ok = %v, want %v", c.text, ok, c.wantOK)
		}
		if !ok {
			continue
		}
		if got.Namespace != c.wantNS || got.ID != c.wantID || got.Code != c.wantCode {
			t.Fatalf("ParseCaseRef(%q) = %+v, want {%s %d %s}", c.text, got, c.wantNS, c.wantID, c.wantCode)
		}
	}
}

func TestStripCaseRefs(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"status of REP-606", "status of "},
		{"give me 3 latest incoming cases", "give me 3 latest incoming cases"},
		{"check draft:45 please", "check  please"},
	}
	for _, c := range cases {
		if got := StripCaseRefs(c.text); got != c.want {
			t.Fatalf("StripCaseRefs(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}
