package memgraph

import "testing"

func TestCheckCypherIdent(t *testing.T) {
	valid := []string{"Report", "KbArticle", "SOLVES", "_x", "a1_B2"}
	for _, v := range valid {
		if err := checkCypherIdent("label", v); err != nil {
			t.Errorf("checkCypherIdent(%q) = %v, want nil", v, err)
		}
	}

	invalid := []string{
		"",
		"1Report",
		"Report Article",
		"Report`",
		"Report) DETACH DELETE (n",
		"Report{company_id:0}",
		"Report-Article",
	}
	for _, v := range invalid {
		if err := checkCypherIdent("label", v); err == nil {
			t.Errorf("checkCypherIdent(%q) = nil, want error", v)
		}
	}
}
