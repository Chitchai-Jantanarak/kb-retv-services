package intent

import (
	"context"
	"os"
	"testing"

	appconfig "github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
)

type zeroScorer struct{}

func (zeroScorer) TopScore(context.Context, string) (float64, error) { return 0, nil }

func TestProbeSocialBucket(t *testing.T) {
	if os.Getenv("PROBE") == "" {
		t.Skip("set PROBE=1")
	}
	cfg, err := appconfig.LoadFrom("/app/config.yaml")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	guard, th, _, _ := llmboot.GuardEmbedder(cfg, nil)
	if guard == nil {
		t.Fatal("guard embedder unavailable")
	}
	opts := []Option{}
	if th.HasValues {
		opts = append(opts, WithThresholds(th.Floor, th.Accept), WithMargin(th.Margin))
	}
	r, err := New(context.Background(), guard, zeroScorer{}, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cases := []struct {
		want Intent
		text string
	}{
		{Social, "test"},
		{Social, "hello"},
		{Social, "who are you?"},
		{Social, "what time is it?"},
		{Social, "สวัสดีครับ"},
		{Social, "ทดสอบ"},
		{Social, "ขอบคุณครับ"},
		{Social, "テスト"},
		{Social, "тест"},
		{OffDomain, "deliver some food to my house tonight"},
		{OffDomain, "แนะนำหนังตลกเรื่องใหม่หน่อย"},
		{CaseStatus, "check status of case REP-1234"},
		{CaseStatus, "ตรวจสอบสถานะเคส REP-1234"},
		{OpenCase, "open a new support case"},
		{Handoff, "talk to a human agent"},
	}

	ok := 0
	for _, c := range cases {
		d, err := r.Route(context.Background(), c.text)
		if err != nil {
			t.Fatalf("Route %q: %v", c.text, err)
		}
		mark := " "
		if d.Intent == c.want {
			ok++
		} else {
			mark = "x"
		}
		t.Logf("  %s %-42s want=%-16s got=%-16s conf=%.3f", mark, c.text, c.want, d.Intent, d.Confidence)
	}
	t.Logf("")
	t.Logf("router buckets: %d/%d", ok, len(cases))
}
