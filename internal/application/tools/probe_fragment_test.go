package tools

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/my/app/internal/infra/llm/gemini"
	"github.com/my/app/internal/shared/ctxkey"
)

type fragmentCase struct {
	text       string
	want       string
	transcript string
}

const fragTranscriptCases = `User: แสดงรายงานล่าสุด
Assistant: พบ 10 เคส ล่าสุดคือ REP-4104
REP-4104 | ทดสอบ routing | waiting
REP-4103 | หุ่นยนต์ไม่เข้าลิฟต์ | doing
REP-4102 | จอดผิดจุด | waiting
`

const fragTranscriptMail = `User: อีเมลที่เข้ามาวันนี้
Assistant: พบ 3 ฉบับ ล่าสุดคือ ai:email:11208:4821
ai:email:11208:4821 | หุ่นยนต์ค้าง | new
`

var fragmentCases = []fragmentCase{
	{"อันแรกสถานะอะไร", "f2_case_status", fragTranscriptCases},
	{"แล้วใครดูแล", "f2_case_status", fragTranscriptCases},
	{"ปิดอันนั้นเลย", "f10_close_case", fragTranscriptCases},
	{"ไม่เอา draft", "f1_find_cases", fragTranscriptCases},
	{"เอาเฉพาะ T300", "f5_product_cases", fragTranscriptCases},
	{"promote เคสนี้", "f13_promote_mail", fragTranscriptMail},
}

// The four permanent misses in probe_tier2cascade_test.go are all context-less
// fragments. This measures the same fragments twice, once bare and once with the
// transcript window the production path now attaches, so the transcript's effect
// is a measured delta rather than an assumption.
func TestProbeFragmentTranscript(t *testing.T) {
	if os.Getenv("PROBE") == "" {
		t.Skip("set PROBE=1")
	}
	ctx, cfg, _, _, catalog := probeBoundSelector(t)
	if cfg.APIKeys.Gemini == "" {
		t.Skip("no gemini key")
	}
	model := "gemini-3.5-flash-lite"
	if m := os.Getenv("TIER2_MODELS"); m != "" {
		model = m
	}
	client, err := gemini.New(gemini.Config{APIKey: cfg.APIKeys.Gemini, Model: model})
	if err != nil {
		t.Fatalf("gemini: %v", err)
	}
	sel := NewModelSelector(client, catalog)
	granted := []string{"*"}

	bareHits, ctxHits := 0, 0
	t.Logf("")
	t.Logf("%-22s %-20s %-20s %s", "fragment", "bare", "with transcript", "want")
	for _, c := range fragmentCases {
		callCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		bare, bareErr := sel.Select(callCtx, c.text, granted)
		cancel()

		withCtx := ctxkey.WithTranscript(ctx, c.transcript)
		callCtx2, cancel2 := context.WithTimeout(withCtx, 25*time.Second)
		got, gotErr := sel.Select(callCtx2, c.text, granted)
		cancel2()

		bareID := selLabel(bare.ToolID, bare.Matched, bareErr)
		gotID := selLabel(got.ToolID, got.Matched, gotErr)
		if bareID == c.want {
			bareHits++
		}
		if gotID == c.want {
			ctxHits++
		}
		t.Logf("%-22s %-20s %-20s %s", c.text, bareID, gotID, c.want)
		if len(got.Params) > 0 {
			t.Logf("%-22s params: %v", "", got.Params)
		}
	}
	t.Logf("")
	t.Logf("resolved: bare %d/%d, with transcript %d/%d", bareHits, len(fragmentCases), ctxHits, len(fragmentCases))
	if ctxHits <= bareHits {
		t.Errorf("transcript did not help: bare=%d withTranscript=%d", bareHits, ctxHits)
	}
}

func selLabel(id string, matched bool, err error) string {
	if err != nil {
		return "error"
	}
	if !matched {
		return "no_tool"
	}
	return id
}
