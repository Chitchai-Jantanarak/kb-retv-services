package intake_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/application/workflows/intake"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/llm/gemini"
)

// buildOutOfTopicCases: mail with a real body but no relation to robots or
// support at all. tier0 lets most through (base score), so only tier1 can
// reject them. All are not-tickets.
func buildOutOfTopicCases() []swarmCase {
	mk := func(subj, body string) swarmCase {
		return swarmCase{truth: "not", band: "offtopic", subject: subj, sig: intake.Signals{Sender: "someone@random.com", Subject: subj, Body: body}}
	}
	return []swarmCase{
		mk("สูตรผัดไทย", "รบกวนขอสูตรผัดไทยแบบต้นตำรับหน่อยครับ ใส่อะไรบ้าง เคี่ยวน้ำมะขามยังไงให้อร่อย อยากทำเองที่บ้าน"),
		mk("weather tomorrow", "hey do you know if it will rain tomorrow in Bangkok? planning a trip and want to pack right"),
		mk("Q3 budget meeting notes", "attaching the Q3 budget review notes, please look at the marketing line items before the board call on Friday"),
		mk("ร้านอาหารแนะนำ", "แถวสยามมีร้านอาหารญี่ปุ่นอร่อยๆ แนะนำไหมครับ งบประมาณคนละพันบาท ไปกันสี่คน"),
		mk("python homework help", "can you help me debug this python script for my university assignment, it keeps throwing an index error on line 12"),
		mk("หวยงวดนี้", "งวดนี้เลขเด็ดออกอะไร มีใบ้มาบ้างไหม อยากได้เลขท้ายสองตัวสามตัว"),
		mk("team standup 10am", "reminder that the team standup is moved to 10am tomorrow in room 3, please update your calendars accordingly"),
		mk("gym membership renewal", "your gym membership is up for renewal next week, would you like to keep the same plan or upgrade to premium"),
		mk("travel England", "แนะนำสถานที่ท่องเที่ยวในอังกฤษหน่อยครับ ไปหน้าหนาวเหมาะไหม ควรเตรียมอะไรบ้าง"),
		mk("joke of the day", "why did the chicken cross the road? just wanted to share a laugh with the team today, have a good one everyone"),
	}
}

func TestCompareTiers(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE=1 to hit the real LLM")
	}
	key := os.Getenv("APIKEYS_GEMINI")
	if key == "" {
		t.Skip("APIKEYS_GEMINI not set")
	}
	client, err := gemini.New(gemini.Config{APIKey: key, Model: os.Getenv("LLM_DEFAULT_MODEL")})
	if err != nil {
		t.Fatalf("gemini: %v", err)
	}
	reg, err := prompts.NewRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	ext, err := intake.NewExtractor(reg, func(ctx context.Context, cid int64) (ports.LLMProvider, error) { return client, nil })
	if err != nil {
		t.Fatalf("extractor: %v", err)
	}
	actionable := map[string]bool{
		intake.ClassificationNewIssue:    true,
		intake.ClassificationFollowUp:    true,
		intake.ClassificationStatusQuery: true,
	}

	cases := make([]swarmCase, 0, 120)
	cases = append(cases, buildTicketCases()...)
	cases = append(cases, buildJunkCases()...)
	cases = append(cases, buildAmbiguousCases()...)
	cases = append(cases, buildOutOfTopicCases()...)

	type mtx struct{ tp, fp, fn, tn int }
	add := func(m *mtx, truth, pred string) {
		switch {
		case truth == "ticket" && pred == "ticket":
			m.tp++
		case truth == "not" && pred == "ticket":
			m.fp++
		case truth == "ticket" && pred == "not":
			m.fn++
		default:
			m.tn++
		}
	}
	var t0, tc mtx
	flipsFixed, flipsBroke := 0, 0 // tier1 vs tier0 on the same case
	var brokeList, offtopicMiss []string

	ctx := context.Background()
	for _, c := range cases {
		sc, _ := intake.Score(c.sig)
		pred0 := "not"
		if sc > gateThreshold {
			pred0 = "ticket"
		}
		res, err := ext.Extract(ctx, 1, c.sig)
		if err != nil {
			t.Fatalf("extract %q: %v", c.subject, err)
		}
		predC := "not"
		if actionable[res.Classification] {
			predC = "ticket"
		}
		add(&t0, c.truth, pred0)
		add(&tc, c.truth, predC)

		if pred0 != predC {
			correct0 := pred0 == c.truth
			correctC := predC == c.truth
			if !correct0 && correctC {
				flipsFixed++
			} else if correct0 && !correctC {
				flipsBroke++
				brokeList = append(brokeList, c.subject)
			}
		}
		if c.band == "offtopic" && predC == "ticket" {
			offtopicMiss = append(offtopicMiss, c.subject+" -> "+res.Classification)
		}
	}

	show := func(name string, m mtx) string {
		n := m.tp + m.fp + m.fn + m.tn
		return fmt.Sprintf("%s (n=%d)\n"+
			"                 pred:ticket  pred:not\n"+
			"  truth:ticket       %3d        %3d\n"+
			"  truth:not          %3d        %3d\n"+
			"  precision=%.3f recall=%.3f accuracy=%.3f f1=%.3f",
			name, n, m.tp, m.fn, m.fp, m.tn,
			ratio(m.tp, m.tp+m.fp), ratio(m.tp, m.tp+m.fn), ratio(m.tp+m.tn, n), f1(m.tp, m.fp, m.fn))
	}

	t.Logf("\n=== TIER0 only (gate) ===\n%s", show("tier0", t0))
	t.Logf("\n=== TIER0 + TIER1 (Gemini) ===\n%s", show("tier0+1", tc))
	t.Logf("\ntier1 delta vs tier0: fixed=%d broke=%d", flipsFixed, flipsBroke)
	if len(brokeList) > 0 {
		t.Logf("tier1 BROKE (was right, now wrong): %v", brokeList)
	}
	if len(offtopicMiss) > 0 {
		t.Logf("off-topic leaked as ticket (%d): %v", len(offtopicMiss), offtopicMiss)
	} else {
		t.Logf("off-topic: all rejected by pipeline")
	}
}
