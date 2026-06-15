package reply

import (
	"context"
	"testing"
)

type fakeAnchorer struct {
	id     int64
	conf   float64
	called int
}

func (f *fakeAnchorer) Anchor(ctx context.Context, companyID int64, message string) (int64, float64) {
	f.called++
	return f.id, f.conf
}

func TestApplySymptomAnchorInjectsAIAnchor(t *testing.T) {
	anchorer := &fakeAnchorer{id: 42, conf: 0.88}
	in := map[string]string{"channel": "web"}

	out := applySymptomAnchor(context.Background(), anchorer, 7, "ups won't back up", in)

	if out["ai_symptom_node_id"] != "42" {
		t.Fatalf("ai_symptom_node_id = %q, want 42", out["ai_symptom_node_id"])
	}
	if out["ai_symptom_conf"] != "0.88" {
		t.Fatalf("ai_symptom_conf = %q, want 0.88", out["ai_symptom_conf"])
	}
	if out["channel"] != "web" {
		t.Fatalf("existing metadata lost: %v", out)
	}
}

func TestApplySymptomAnchorRespectsLaravelOverride(t *testing.T) {
	anchorer := &fakeAnchorer{id: 42, conf: 0.88}
	in := map[string]string{"symptom_node_id": "9"}

	out := applySymptomAnchor(context.Background(), anchorer, 7, "msg", in)

	if anchorer.called != 0 {
		t.Fatalf("anchorer called %d times, want 0 when Laravel supplied symptom_node_id", anchorer.called)
	}
	if _, ok := out["ai_symptom_node_id"]; ok {
		t.Fatalf("must not inject ai anchor over a confirmed symptom: %v", out)
	}
}

func TestApplySymptomAnchorNoMatchLeavesMetadata(t *testing.T) {
	anchorer := &fakeAnchorer{id: 0, conf: 0}
	in := map[string]string{"channel": "line"}

	out := applySymptomAnchor(context.Background(), anchorer, 7, "msg", in)

	if _, ok := out["ai_symptom_node_id"]; ok {
		t.Fatalf("no match must not inject anchor: %v", out)
	}
}

func TestApplySymptomAnchorNilAnchorerPassesThrough(t *testing.T) {
	in := map[string]string{"a": "b"}
	out := applySymptomAnchor(context.Background(), nil, 7, "msg", in)
	if out["a"] != "b" || len(out) != 1 {
		t.Fatalf("nil anchorer should pass metadata through unchanged: %v", out)
	}
}

func TestApplySymptomAnchorDoesNotMutateInput(t *testing.T) {
	anchorer := &fakeAnchorer{id: 5, conf: 0.9}
	in := map[string]string{"channel": "web"}

	_ = applySymptomAnchor(context.Background(), anchorer, 7, "msg", in)

	if _, ok := in["ai_symptom_node_id"]; ok {
		t.Fatalf("input map was mutated: %v", in)
	}
}
