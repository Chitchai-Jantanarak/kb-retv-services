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

// TestLiveTier1 runs the REAL extractor (tier0 gate + tier1 Gemini classify)
// on cases chosen to PASS the tier0 gate, so the model actually runs. It
// measures whether tier1 corrects the content-blind calls tier0 cannot make.
// Requires LIVE=1 and a working APIKEYS_GEMINI in the environment.
func TestLiveTier1(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE=1 to hit the real LLM")
	}
	key := os.Getenv("APIKEYS_GEMINI")
	if key == "" {
		t.Skip("APIKEYS_GEMINI not set")
	}
	client, err := gemini.New(gemini.Config{APIKey: key, Model: os.Getenv("LLM_DEFAULT_MODEL")})
	if err != nil {
		t.Fatalf("gemini client: %v", err)
	}
	reg, err := prompts.NewRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	ext, err := intake.NewExtractor(reg, func(ctx context.Context, cid int64) (ports.LLMProvider, error) {
		return client, nil
	})
	if err != nil {
		t.Fatalf("extractor: %v", err)
	}

	actionable := map[string]bool{
		intake.ClassificationNewIssue:    true,
		intake.ClassificationFollowUp:    true,
		intake.ClassificationStatusQuery: true,
	}

	type tc struct {
		truth   string
		subject string
		sig     intake.Signals
	}
	rep := func(s string) string { return s + " " + s }
	cases := []tc{
		{"ticket", "PuduBot E-Stop", intake.Signals{Sender: "cust@known.co.th", Subject: "หุ่นยนต์ error", Body: rep("หุ่นยนต์ PuduBot ชั้น 3 หยุดวิ่ง ขึ้น error E-Stop ค้างหน้าลิฟต์ กดปลดล็อกไม่ได้ ขอช่างด่วน"), ReferencedCase: "REP-4106", SenderKnown: true, HasAttachments: true}},
		{"ticket", "T300 dock", intake.Signals{Sender: "cust@known.co.th", Subject: "T300 charge", Body: rep("เครื่อง T300 เข้า dock ชาร์จไม่ได้ วนอยู่ lobby จนแบตหมด ขอทีมช่างเข้าเช็ค"), ThreadMatched: true, SenderKnown: true}},
		{"ticket", "battery degrade", intake.Signals{Sender: "cust@known.co.th", Subject: "แบตเสื่อม", Body: rep("แบตเตอรี่หุ่นยนต์ตัวที่ 4 เสื่อมเร็ว ใช้ไม่ถึงชั่วโมงต้องชาร์จ ปกติอยู่ทั้งวัน ขอเปลี่ยน"), SenderKnown: true}},
		{"ticket", "escalate REP-4980", intake.Signals{Sender: "cust@known.co.th", Subject: "Re: REP-4980", Body: rep("ตามเคส REP-4980 อาการเดิมยังไม่หาย หุ่นยนต์ยังชนกำแพง ขอ escalate ด่วน"), ReferencedCase: "REP-4980", ThreadMatched: true, SenderKnown: true}},
		{"ticket", "status query", intake.Signals{Sender: "cust@known.co.th", Subject: "สอบถามเคส", Body: rep("อยากทราบว่าเคส REP-4980 ที่แจ้งซ่อมหุ่นยนต์ไป ตอนนี้ดำเนินการถึงขั้นไหนแล้วครับ"), ReferencedCase: "REP-4980", SenderKnown: true}},
		{"ticket", "thin real fault", intake.Signals{Sender: "u@customer.com", Subject: "ช่วยด้วย", Body: "หุ่นยนต์เสิร์ฟล้มที่โต๊ะ 12 เปิดไม่ติดแล้ว ช่วยด้วยครับ ขอช่างด่วน"}},

		{"not", "thanks for fix", intake.Signals{Sender: "cust@known.co.th", Subject: "ขอบคุณ", Body: rep("ขอบคุณทีมงานที่แก้เคส REP-4106 ให้เรียบร้อย หุ่นยนต์กลับมาใช้งานได้ปกติแล้วครับ"), ReferencedCase: "REP-4106", SenderKnown: true}},
		{"not", "sales pricing", intake.Signals{Sender: "buyer@lead.com", Subject: "สอบถามราคา", Body: rep("สนใจหุ่นยนต์เสิร์ฟรุ่นใหม่ ขอใบเสนอราคาและสเปค มีโปรโมชั่นช่วงนี้ไหมครับ")}},
		{"not", "out of office", intake.Signals{Sender: "cust@known.co.th", Subject: "Auto-Reply", Body: rep("I am out of office until Monday, for urgent matters please contact my team, this is an automatic reply"), AutoSubmitted: "auto-replied", SenderKnown: true}},
		{"not", "ping test", intake.Signals{Sender: "admin@known.co.th", Subject: "test", Body: "ping test 123 ทดสอบระบบ ไม่มีอะไร", SenderKnown: true}},
	}

	ctx := context.Background()
	var tp, fp, fn, tn int
	fmt.Printf("\n  truth   pred(class)        subject\n  ------  -----------------  -------\n")
	for i, c := range cases {
		res, err := ext.Extract(ctx, 1, c.sig)
		if err != nil {
			if i == 0 {
				t.Skipf("first live call failed (key/quota?): %v", err)
			}
			t.Fatalf("case %q extract: %v", c.subject, err)
		}
		pred := "not"
		if actionable[res.Classification] {
			pred = "ticket"
		}
		hit := "ok"
		if pred != c.truth {
			hit = "XX"
		}
		fmt.Printf("  %-6s  %-17s  %s  %s\n", c.truth, res.Classification, hit, c.subject)
		switch {
		case c.truth == "ticket" && pred == "ticket":
			tp++
		case c.truth == "not" && pred == "ticket":
			fp++
		case c.truth == "ticket" && pred == "not":
			fn++
		default:
			tn++
		}
	}

	prec, rec := ratio(tp, tp+fp), ratio(tp, tp+fn)
	t.Logf("\nTIER1 (Gemini) on gate-passers, n=%d\n"+
		"                 pred:ticket  pred:not\n"+
		"  truth:ticket       %2d         %2d\n"+
		"  truth:not          %2d         %2d\n"+
		"  precision=%.2f recall=%.2f accuracy=%.2f",
		len(cases), tp, fn, fp, tn, prec, rec, ratio(tp+tn, len(cases)))
}
