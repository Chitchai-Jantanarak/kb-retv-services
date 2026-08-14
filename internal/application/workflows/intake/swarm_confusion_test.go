package intake_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/my/app/internal/application/workflows/intake"
)

// gateThreshold mirrors extractor.skipModelBelowScore: score <= it means the
// cheap tier-0 filter discards the mail without ever calling the model.
const gateThreshold = 20

func TestSwarmConfusionMatrix(t *testing.T) {
	type row struct {
		truth   string // "ticket" | "not"
		band    string // "clear" | "ambiguous"
		subject string
		sig     intake.Signals
	}

	body := func(s string, n int) string { return strings.Repeat(s+" ", n) }

	rows := []row{
		// ---- clear tickets ----
		{"ticket", "clear", "PuduBot ชั้น3 E-Stop ตามเคส REP-4106", intake.Signals{Sender: "somchai@grandhotel.co.th", Body: body("หุ่นยนต์ PuduBot ชั้น 3 หยุดวิ่ง ขึ้น error E-Stop ค้างหน้าลิฟต์ ตามเคส REP-4106", 2), ReferencedCase: "REP-4106", ThreadMatched: true, HasAttachments: true, SenderKnown: true}},
		{"ticket", "clear", "T300 เข้า dock ไม่ได้", intake.Signals{Sender: "facility@sirapark.com", Body: body("เครื่อง T300 ตัวที่ 2 เข้า dock ชาร์จไม่ได้ วนอยู่ lobby แบตหมด ขอช่างเข้าเช็ค", 2), ThreadMatched: true, HasAttachments: true, SenderKnown: true}},
		{"ticket", "clear", "หุ่นยนต์ไม่ออกลิฟต์ ชั้น 5", intake.Signals{Sender: "eng@centralmall.co.th", Body: body("หุ่นยนต์ส่งของไม่ยอมออกลิฟต์ที่ชั้น 5 ค้างมา 2 ชม แล้ว รบกวนแก้ด่วน", 2), SenderKnown: true, HasAttachments: true}},
		{"ticket", "clear", "แบตเตอรี่หุ่นยนต์เสื่อมเร็ว", intake.Signals{Sender: "ops@hospital.go.th", Body: body("แบตเตอรี่หุ่นยนต์ตัวที่ 4 เสื่อม ใช้ได้ไม่ถึงชั่วโมงก็ต้องชาร์จ ปกติอยู่ได้ทั้งวัน", 2), SenderKnown: true}},
		{"ticket", "clear", "robot fell at table 12, video attached", intake.Signals{Sender: "manager@zenrestaurant.co.th", Body: body("serving robot fell while passing table 12, dishes broke, it won't power on now, please send technician", 2), HasAttachments: true}},
		{"ticket", "clear", "Re: REP-4980 ยังไม่หาย ขอ escalate", intake.Signals{Sender: "somsak@bighotel.com", Body: body("ตามเคส REP-4980 ที่แจ้งไป อาการเดิมยังไม่หาย หุ่นยนต์ยังชนกำแพง ขอ escalate ด่วน", 2), ReferencedCase: "REP-4980", ThreadMatched: true, SenderKnown: true}},
		{"ticket", "clear", "map ผิด หุ่นวิ่งชนโต๊ะ", intake.Signals{Sender: "it@resort.co.th", Body: body("หลัง update map หุ่นยนต์วิ่งชนโต๊ะประจำ ตำแหน่งเพี้ยน ขอทีมมา re-map ให้หน่อย", 2), SenderKnown: true, HasAttachments: true}},
		{"ticket", "clear", "จอ error code 0x8 ค้าง", intake.Signals{Sender: "front@hotelx.com", Body: body("หน้าจอหุ่นยนต์ขึ้น error code 0x8 แล้วค้าง กดอะไรไม่ได้เลย restart แล้วก็ยังเป็น", 2), SenderKnown: true, ThreadMatched: true}},

		// ---- clear not-ticket ----
		{"not", "clear", "PuduTech Monthly Newsletter", intake.Signals{Sender: "no-reply@news.pudurobotics.com", Body: body("Check our new robots at https://pudu.com/a and https://pudu.com/b today", 5), ListUnsubscribe: true, Precedence: "bulk"}},
		{"not", "clear", "Your invoice #INV-2231 is ready", intake.Signals{Sender: "no-reply@billing.vendor.com", Body: "Your monthly invoice is now available in the portal.", AutoSubmitted: "auto-generated"}},
		{"not", "clear", "50% OFF accessories this week", intake.Signals{Sender: "promo@robotshop.com", Body: body("Buy now https://shop.example/x limited offer https://shop.example/y", 6), ListUnsubscribe: true}},
		{"not", "clear", "Delivery notification", intake.Signals{Sender: "auto@logistics.com", Body: "Your package has been delivered. Track at https://track.example/z", AutoSubmitted: "auto-replied"}},
		{"not", "clear", "We value your feedback - survey", intake.Signals{Sender: "no-reply@survey.com", Body: body("Please rate your experience https://survey.example/a it takes 2 minutes", 4), ListUnsubscribe: true, Precedence: "bulk"}},
		{"not", "clear", "Password reset requested", intake.Signals{Sender: "no-reply@accounts.com", Body: "Click https://reset.example/token to reset your password.", AutoSubmitted: "auto-generated"}},
		{"not", "clear", "You're invited: Robotics Webinar", intake.Signals{Sender: "events@webinar.com", Body: body("Join our free webinar https://webinar.example/join register now seats limited", 5), ListUnsubscribe: true}},
		{"not", "clear", "Weekly digest: 12 new posts", intake.Signals{Sender: "digest@community.com", Body: body("See the top posts https://c.example/1 https://c.example/2 this week", 6), ListUnsubscribe: true, Precedence: "bulk"}},

		// ---- ambiguous ----
		{"ticket", "ambiguous", "หุ่นยนต์มีปัญหา ช่วยด้วย", intake.Signals{Sender: "user@customer.com", Body: "หุ่นยนต์มีปัญหา ช่วยด้วยครับ"}},
		{"not", "ambiguous", "Out of office (auto-reply)", intake.Signals{Sender: "somchai@grandhotel.co.th", Body: "I am currently out of office until Monday. For urgent matters contact my team.", AutoSubmitted: "auto-replied", SenderKnown: true}},
		{"not", "ambiguous", "สอบถามราคาหุ่นยนต์รุ่นใหม่", intake.Signals{Sender: "buyer@newlead.com", Body: body("สนใจหุ่นยนต์เสิร์ฟรุ่นใหม่ ขอใบเสนอราคาและสเปคหน่อยครับ มีโปรโมชั่นไหม", 2)}},
		{"not", "ambiguous", "ขอบคุณที่แก้ REP-4106 ให้", intake.Signals{Sender: "somchai@grandhotel.co.th", Body: body("ขอบคุณทีมงานที่แก้เคส REP-4106 ให้เรียบร้อย หุ่นยนต์กลับมาใช้งานได้ปกติแล้ว", 2), ReferencedCase: "REP-4106", SenderKnown: true}},
		{"ticket", "ambiguous", "fwd newsletter + real question", intake.Signals{Sender: "ops@customer.co.th", Body: body("(forwarded) เห็นใน newsletter ว่ามีฟีเจอร์ใหม่ แต่หุ่นยนต์เราอัพเดทแล้ววิ่งไม่ตรงเส้น เป็นเพราะอะไร", 3), ListUnsubscribe: true}},
		{"ticket", "ambiguous", "image only, unknown sender", intake.Signals{Sender: "someone@gmail.com", Body: "", HasAttachments: true}},
		{"ticket", "ambiguous", "bulk precedence but genuine fault", intake.Signals{Sender: "maint@known.co.th", Body: body("หุ่นยนต์ตัวที่ 3 ล้อไม่หมุน มอเตอร์มีเสียงดัง ขอช่างเข้ามาดู", 2), Precedence: "bulk", SenderKnown: true}},
		{"not", "ambiguous", "ping test", intake.Signals{Sender: "admin@known.co.th", Body: "ping test 123", SenderKnown: true}},
	}

	type scored struct {
		row
		score int
		pred  string
	}
	out := make([]scored, 0, len(rows))
	for _, r := range rows {
		sc, _ := intake.Score(r.sig)
		pred := "not"
		if sc > gateThreshold {
			pred = "ticket"
		}
		out = append(out, scored{row: r, score: sc, pred: pred})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })

	var b strings.Builder
	b.WriteString("\n  score  truth   pred    band       hit  subject\n")
	b.WriteString("  -----  ------  ------  ---------  ---  -------\n")
	for _, s := range out {
		hit := "ok"
		if s.pred != s.truth {
			hit = "XX"
		}
		b.WriteString(fmt.Sprintf("   %3d   %-6s  %-6s  %-9s  %-3s  %s\n", s.score, s.truth, s.pred, s.band, hit, s.subject))
	}
	t.Log(b.String())

	// confusion matrix (positive class = ticket)
	var tp, fp, fn, tn int
	var cTP, cFP, cFN, cTN int // clear-band only
	for _, s := range out {
		switch {
		case s.truth == "ticket" && s.pred == "ticket":
			tp++
			if s.band == "clear" {
				cTP++
			}
		case s.truth == "not" && s.pred == "ticket":
			fp++
			if s.band == "clear" {
				cFP++
			}
		case s.truth == "ticket" && s.pred == "not":
			fn++
			if s.band == "clear" {
				cFN++
			}
		default:
			tn++
			if s.band == "clear" {
				cTN++
			}
		}
	}
	prec := ratio(tp, tp+fp)
	rec := ratio(tp, tp+fn)
	acc := ratio(tp+tn, tp+tn+fp+fn)

	t.Logf("\nCONFUSION MATRIX (all %d, positive=ticket, predictor=tier0 gate score>%d)\n"+
		"                 pred:ticket  pred:not\n"+
		"  truth:ticket        %2d         %2d      (TP / FN)\n"+
		"  truth:not           %2d         %2d      (FP / TN)\n"+
		"  precision=%.2f  recall=%.2f  accuracy=%.2f",
		len(out), gateThreshold, tp, fn, fp, tn, prec, rec, acc)

	t.Logf("clear-band only: TP=%d FP=%d FN=%d TN=%d  (ambiguous band carries all the hard errors)", cTP, cFP, cFN, cTN)

	// tier0 must not fail the EASY cases — clear tickets never gated out, clear junk always gated out.
	if cFN != 0 {
		t.Fatalf("tier0 gated out %d CLEAR ticket(s) — cheap filter killing real work", cFN)
	}
	if cFP != 0 {
		t.Fatalf("tier0 passed %d CLEAR junk(s) as ticket — should be caught before the model", cFP)
	}
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}
