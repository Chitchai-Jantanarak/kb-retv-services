package mysql

import (
	"errors"
	"reflect"
	"testing"
)

func TestParseIntentKeywordsExtractsLowercasedTrimmedList(t *testing.T) {
	got := parseIntentKeywords(`{"intent_keywords":["Robot Stuck","  wheel  ","STOP"]}`)
	want := []string{"robot stuck", "wheel", "stop"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseIntentKeywords() = %v, want %v", got, want)
	}
}

func TestParseIntentKeywordsDropsEmptyEntries(t *testing.T) {
	got := parseIntentKeywords(`{"intent_keywords":["", "  ", "stuck"]}`)
	want := []string{"stuck"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseIntentKeywords() = %v, want %v", got, want)
	}
}

func TestParseIntentKeywordsEmptyStringReturnsNil(t *testing.T) {
	if got := parseIntentKeywords(""); got != nil {
		t.Fatalf("parseIntentKeywords(\"\") = %v, want nil", got)
	}
	if got := parseIntentKeywords("   "); got != nil {
		t.Fatalf("parseIntentKeywords(whitespace) = %v, want nil", got)
	}
}

func TestParseIntentKeywordsNullJSONReturnsNil(t *testing.T) {
	if got := parseIntentKeywords(`{"intent_keywords": null}`); got != nil {
		t.Fatalf("parseIntentKeywords(null field) = %v, want nil", got)
	}
	if got := parseIntentKeywords(`null`); got != nil {
		t.Fatalf("parseIntentKeywords(null) = %v, want nil", got)
	}
}

func TestParseIntentKeywordsMissingKeyReturnsNil(t *testing.T) {
	if got := parseIntentKeywords(`{"other_setting":true}`); got != nil {
		t.Fatalf("parseIntentKeywords(missing key) = %v, want nil", got)
	}
}

func TestParseIntentKeywordsMalformedJSONReturnsNil(t *testing.T) {
	if got := parseIntentKeywords(`not json`); got != nil {
		t.Fatalf("parseIntentKeywords(malformed) = %v, want nil", got)
	}
}

func TestIsUnknownColumnErrMatchesMySQLMessage(t *testing.T) {
	err := errors.New("Error 1054: Unknown column 'intake_settings' in 'field list'")
	if !isUnknownColumnErr(err) {
		t.Fatalf("isUnknownColumnErr() = false, want true for %v", err)
	}
}

func TestIsUnknownColumnErrRejectsOtherErrors(t *testing.T) {
	err := errors.New("connection refused")
	if isUnknownColumnErr(err) {
		t.Fatalf("isUnknownColumnErr() = true, want false for %v", err)
	}
}

func TestParsePromoteThreshold(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int
	}{
		{"valid threshold", `{"auto_promote_threshold":70}`, 70},
		{"missing key", `{"intent_keywords":["a"]}`, 0},
		{"empty string", "", 0},
		{"malformed json", "not json", 0},
		{"above ceiling", `{"auto_promote_threshold":120}`, 0},
		{"negative", `{"auto_promote_threshold":-5}`, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parsePromoteThreshold(c.raw); got != c.want {
				t.Errorf("parsePromoteThreshold(%q) = %d, want %d", c.raw, got, c.want)
			}
		})
	}
}
