package tools

import "testing"

func TestResolveReference(t *testing.T) {
	three := []string{"REP-4106", "REP-4105", "REP-4104"}
	one := []string{"REP-4106"}

	tests := []struct {
		name       string
		text       string
		candidates []string
		want       string
		wantOK     bool
	}{
		{"explicit code wins over everything", "อัพเดท REP-4105 หน่อย", three, "REP-4105", true},
		{"explicit code not in candidates still wins", "close REP-9999", three, "REP-9999", true},

		{"recency th", "อัพเดทอันล่าสุดเป็นกำลังดำเนินการ", three, "REP-4106", true},
		{"recency en", "update the latest one", three, "REP-4106", true},

		{"ordinal word th", "เอาอันแรก", three, "REP-4106", true},
		{"ordinal word en", "the second one please", three, "REP-4105", true},
		{"ordinal digit th", "ตัวที่ 3", three, "REP-4104", true},
		{"ordinal digit en", "number 2", three, "REP-4105", true},
		{"ordinal out of range", "ตัวที่ 9", three, "", false},

		{"singleton with anaphora th", "ปิดอันนี้เลย", one, "REP-4106", true},
		{"singleton with anaphora en", "close it", one, "REP-4106", true},

		{"singleton without any cue stays unresolved", "อัพเดทเป็นกำลังดำเนินการ", one, "", false},
		{"many candidates bare anaphora is ambiguous", "ปิดอันนี้", three, "", false},
		{"no candidates", "อันล่าสุด", nil, "", false},
		{"unrelated text", "ขอดูภาระงานทีม", three, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ResolveReference(tt.text, tt.candidates)
			if ok != tt.wantOK || got != tt.want {
				t.Fatalf("ResolveReference(%q, %v) = (%q, %v), want (%q, %v)",
					tt.text, tt.candidates, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestCiteCandidatesFromTranscript(t *testing.T) {
	turns := []string{
		"พบ 3 เคส\nREP-4106|robot stuck|waiting\nREP-4105|elevator|success",
		"ok",
		"พบ 1 เคส\nREP-9001|newest|waiting",
	}
	got := CiteCandidatesFromTurns(turns, `REP-\d+`)
	want := []string{"REP-9001"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("CiteCandidatesFromTurns = %v, want %v (most recent turn only)", got, want)
	}
}

func TestCiteCandidatesSkipsTurnsWithoutMatches(t *testing.T) {
	turns := []string{"REP-4106|a|waiting", "ไม่พบเคสที่ตรงกับคำค้น"}
	got := CiteCandidatesFromTurns(turns, `REP-\d+`)
	if len(got) != 1 || got[0] != "REP-4106" {
		t.Fatalf("CiteCandidatesFromTurns = %v, want the last turn that actually cited something", got)
	}
}
