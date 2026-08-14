package decide

import (
	"testing"

	"github.com/my/app/internal/application/intent"
	"github.com/my/app/internal/application/skeleton"
)

func TestClassifyLadder(t *testing.T) {
	cases := []struct {
		name    string
		intent  intent.Intent
		tool    *skeleton.Response
		toolErr error
		want    Kind
		missing []string
	}{
		{"needs_param_err", intent.GeneralSupport, &skeleton.Response{}, skeleton.NeedsParams("case_code"), KindClarify, []string{"case_code"}},
		{"write_tool_err", intent.GeneralSupport, &skeleton.Response{Kind: "write"}, assertErr{}, KindToolFailed, nil},
		{"authoritative_tool_err", intent.CaseStatus, &skeleton.Response{}, assertErr{}, KindToolFailed, nil},
		{"read_tool_err_falls_to_generative", intent.GeneralSupport, &skeleton.Response{}, assertErr{}, KindGenerative, nil},
		{"matched", intent.GeneralSupport, &skeleton.Response{Matched: true}, nil, KindTool, nil},
		{"missing_params_selection", intent.GeneralSupport, &skeleton.Response{MissingParams: []string{"status"}}, nil, KindClarify, []string{"status"}},
		{"authoritative_unmatched", intent.CaseStatus, &skeleton.Response{}, nil, KindClarify, nil},
		{"authoritative_perm_filtered", intent.CaseStatus, &skeleton.Response{PermFiltered: true}, nil, KindPermissionDenied, nil},
		{"handoff_no_tool", intent.Handoff, nil, nil, KindHandoff, nil},
		{"social_no_tool", intent.Social, nil, nil, KindSocial, nil},
		{"generative_no_tool", intent.GeneralSupport, nil, nil, KindGenerative, nil},
		{"handoff_after_failed_read_tool", intent.Handoff, &skeleton.Response{}, assertErr{}, KindHandoff, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, missing := classify(intent.Decision{Intent: tc.intent}, tc.tool, tc.toolErr, 0)
			if kind != tc.want {
				t.Fatalf("classify() kind = %q, want %q", kind, tc.want)
			}
			if len(missing) != len(tc.missing) {
				t.Fatalf("classify() missing = %v, want %v", missing, tc.missing)
			}
			for i := range missing {
				if missing[i] != tc.missing[i] {
					t.Fatalf("classify() missing = %v, want %v", missing, tc.missing)
				}
			}
		})
	}
}

type assertErr struct{}

func (assertErr) Error() string { return "boom" }

func TestClassifyOffTopic(t *testing.T) {
	cases := []struct {
		name           string
		offMargin      float64
		offTopicMargin float64
		want           Kind
	}{
		{"margin unconfigured stays generative", 0.5, 0, KindGenerative},
		{"margin configured but below threshold stays generative", 0.1, 0.3, KindGenerative},
		{"margin configured and met promotes to off_topic", 0.3, 0.3, KindOffTopic},
		{"margin configured and exceeded promotes to off_topic", 0.9, 0.3, KindOffTopic},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := intent.Decision{Intent: intent.OffDomain, OffMargin: tc.offMargin}
			kind, _ := classify(decision, nil, nil, tc.offTopicMargin)
			if kind != tc.want {
				t.Fatalf("classify() kind = %q, want %q", kind, tc.want)
			}
		})
	}
}
