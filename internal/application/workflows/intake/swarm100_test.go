package intake_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/my/app/internal/application/workflows/intake"
)

type swarmCase struct {
	truth   string // ticket | not
	band    string // clear | ambiguous
	subject string
	sig     intake.Signals
}

// buildTicketCases: substantial real fault reports crossed with realistic
// signal profiles. All are genuine tickets; a real fault report always has
// enough body to clear the thin-body penalty, so tier0 should never gate them.
func buildTicketCases() []swarmCase {
	bodies := []string{
		"หุ่นยนต์ PuduBot ที่ชั้น 3 หยุดวิ่งตั้งแต่เช้า ขึ้น error E-Stop ค้างอยู่หน้าลิฟต์ กดปลดล็อกก็ไม่ได้ รบกวนช่างเข้ามาดูด่วน",
		"เครื่อง T300 ตัวที่ 2 เข้า dock ชาร์จไม่ได้เลยตั้งแต่เมื่อคืน วนอยู่แถว lobby จนแบตหมด ขอทีมช่างเข้ามาเช็คหน่อยครับ",
		"หุ่นยนต์ส่งของไม่ยอมออกจากลิฟต์ที่ชั้น 5 ค้างมาสองชั่วโมงแล้ว ผู้ใช้ลิฟต์ติดขัดไปหมด รบกวนแก้ไขโดยเร็ว",
		"แบตเตอรี่หุ่นยนต์ตัวที่ 4 เสื่อมเร็วผิดปกติ ใช้งานได้ไม่ถึงชั่วโมงก็ต้องกลับไปชาร์จ ปกติอยู่ได้ทั้งวัน ขอเปลี่ยนแบต",
		"the serving robot fell over while passing table 12, several dishes broke and it will not power back on now, please dispatch a technician today",
		"หลังจากอัปเดตแผนที่ หุ่นยนต์วิ่งชนโต๊ะเป็นประจำ ตำแหน่งเพี้ยนไปจากเดิม ขอทีมเข้ามา re-map ใหม่ทั้งชั้น",
		"หน้าจอหุ่นยนต์ขึ้น error code 0x8 แล้วค้าง กดปุ่มอะไรก็ไม่ตอบสนอง restart ไปหลายรอบแล้วก็ยังเป็นเหมือนเดิม",
		"robot number 3 keeps beeping and the wheels lock up randomly in the middle of a delivery, it happened five times today, we need this fixed",
		"หุ่นยนต์ทำความสะอาดไม่ยอมกลับฐานชาร์จเอง ต้องเข็นกลับทุกครั้ง เซ็นเซอร์ด้านหน้าอาจมีปัญหา ขอช่างมาตรวจ",
		"ลูกค้าแจ้งว่าหุ่นยนต์เสิร์ฟพูดเสียงดังผิดปกติและล้อมีเสียงครืดคราด เหมือนมอเตอร์จะเสีย รบกวนเข้ามาดูก่อนพัง",
		"the delivery robot's lidar seems misaligned, it stops randomly and reports obstacles where there are none, blocking the corridor for guests",
		"หุ่นยนต์ที่โรงพยาบาลชั้น 2 จอฟ้าค้างตอนกลางดึก เปิดใหม่แล้วเชื่อมต่อ wifi ไม่ได้ ใช้งานไม่ได้เลยตอนนี้",
	}
	profiles := []struct {
		name   string
		known  bool
		thread bool
		attach bool
		ref    string
	}{
		{"known+thread+attach+ref", true, true, true, "REP-41%02d"},
		{"known+attach", true, false, true, ""},
		{"new+thread", false, true, false, ""},
		{"new+body-only", false, false, false, ""},
	}

	cases := make([]swarmCase, 0, len(bodies)*len(profiles))
	for i, body := range bodies {
		for _, p := range profiles {
			ref := ""
			if p.ref != "" {
				ref = fmt.Sprintf(p.ref, i)
			}
			cases = append(cases, swarmCase{
				truth: "ticket", band: "clear",
				subject: fmt.Sprintf("[T%02d %s]", i, p.name),
				sig: intake.Signals{
					Sender: "cust@known.co.th", Body: body,
					SenderKnown: p.known, ThreadMatched: p.thread,
					HasAttachments: p.attach, ReferencedCase: ref,
				},
			})
		}
	}
	return cases
}

// buildJunkCases: bulk/auto mail crossed with profiles that always carry at
// least one strong machine-origin signal (unsubscribe or auto-submitted),
// which is what real bulk mail looks like. tier0 should gate all of these.
func buildJunkCases() []swarmCase {
	bodies := []string{
		"Check out our newest robots and features this month, read more at https://news.example/a and https://news.example/b",
		"Your monthly invoice INV-%d is now available for download in the billing portal, no action is required",
		"Limited time offer, fifty percent off all robot accessories this week only, shop now at https://shop.example/x",
		"Your package has been delivered successfully, track the details at https://track.example/z anytime",
		"We value your feedback, please rate your recent experience with a short two minute survey https://survey.example/a",
		"A password reset was requested for your account, click https://reset.example/token to continue or ignore this",
		"You are invited to our free robotics webinar next Tuesday, register now at https://webinar.example/join seats are limited",
		"Here is your weekly community digest with twelve new posts, see the highlights at https://c.example/1 today",
		"Thanks for subscribing, here are five tips to get the most out of your account this week, read at https://tips.example/t",
		"Your subscription will renew automatically next month, manage your plan anytime in the account settings page online",
	}
	profiles := []struct {
		name    string
		unsub   bool
		auto    string
		noreply bool
		bulk    bool
	}{
		{"unsub+bulk", true, "", false, true},
		{"auto+noreply", false, "auto-generated", true, false},
		{"unsub+links", true, "", false, false},
		{"auto+bulk", false, "auto-replied", false, true},
	}

	sender := func(noreply bool) string {
		if noreply {
			return "no-reply@bulk.example.com"
		}
		return "news@bulk.example.com"
	}

	cases := make([]swarmCase, 0, len(bodies)*len(profiles))
	for i, raw := range bodies {
		body := raw
		if strings.Contains(raw, "%d") {
			body = fmt.Sprintf(raw, 2200+i)
		}
		for _, p := range profiles {
			cases = append(cases, swarmCase{
				truth: "not", band: "clear",
				subject: fmt.Sprintf("[J%02d %s]", i, p.name),
				sig: intake.Signals{
					Sender: sender(p.noreply), Body: body,
					ListUnsubscribe: p.unsub, AutoSubmitted: p.auto, Precedence: boolBulk(p.bulk),
				},
			})
		}
	}
	return cases
}

func boolBulk(b bool) string {
	if b {
		return "bulk"
	}
	return ""
}

// buildAmbiguousCases: the hard middle where tier0 evidence and true intent
// diverge — exactly what the tier1 model exists to resolve.
func buildAmbiguousCases() []swarmCase {
	return []swarmCase{
		{"not", "ambiguous", "thanks for fixing REP-4106", intake.Signals{Sender: "cust@known.co.th", Body: strings.Repeat("ขอบคุณทีมงานที่แก้เคส REP-4106 ให้เรียบร้อย หุ่นยนต์กลับมาใช้งานได้ปกติแล้วครับ ", 2), ReferencedCase: "REP-4106", SenderKnown: true}},
		{"not", "ambiguous", "sales inquiry pricing", intake.Signals{Sender: "buyer@newlead.com", Body: strings.Repeat("สนใจหุ่นยนต์เสิร์ฟรุ่นใหม่ ขอใบเสนอราคาและสเปคเพิ่มเติม มีโปรโมชั่นช่วงนี้ไหมครับ ", 2)}},
		{"ticket", "ambiguous", "thin vague fault", intake.Signals{Sender: "user@customer.com", Body: "หุ่นยนต์มีปัญหา ช่วยด้วยครับ"}},
		{"not", "ambiguous", "ping test known", intake.Signals{Sender: "admin@known.co.th", Body: "ping test 123", SenderKnown: true}},
		{"not", "ambiguous", "out of office known", intake.Signals{Sender: "cust@known.co.th", Body: "I am out of office until Monday, for urgent matters please contact my team", AutoSubmitted: "auto-replied", SenderKnown: true}},
		{"ticket", "ambiguous", "fwd newsletter + real question", intake.Signals{Sender: "ops@customer.co.th", Body: strings.Repeat("เห็นใน newsletter ว่ามีฟีเจอร์ใหม่ แต่หุ่นยนต์เราอัปเดตแล้ววิ่งไม่ตรงเส้น เป็นเพราะอะไร ", 2), ListUnsubscribe: true}},
		{"ticket", "ambiguous", "image only unknown", intake.Signals{Sender: "someone@gmail.com", Body: "", HasAttachments: true}},
		{"not", "ambiguous", "general question known", intake.Signals{Sender: "cust@known.co.th", Body: strings.Repeat("อยากทราบว่าหุ่นยนต์รองรับภาษาอังกฤษด้วยไหม และตั้งเวลาทำงานอัตโนมัติได้หรือเปล่า ", 2), SenderKnown: true}},
		{"ticket", "ambiguous", "short real fault", intake.Signals{Sender: "ops@customer.co.th", Body: "16.23 น. Robot No.1 หยุดทำงาน No.12"}},
		{"not", "ambiguous", "forwarded receipt", intake.Signals{Sender: "auto@vendor.com", Body: "Please find attached your receipt for the recent purchase, no reply needed", AutoSubmitted: "auto-generated"}},
		{"ticket", "ambiguous", "newsletter-style but real issue", intake.Signals{Sender: "cust@known.co.th", Body: strings.Repeat("หุ่นยนต์เราอัปเดตเวอร์ชันใหม่แล้วแบตหมดไวมาก จากเดิมอยู่ทั้งวันเหลือไม่กี่ชั่วโมง ", 2), ListUnsubscribe: true, SenderKnown: true}},
		{"ticket", "ambiguous", "bulk precedence genuine fault", intake.Signals{Sender: "maint@known.co.th", Body: strings.Repeat("หุ่นยนต์ตัวที่ 3 ล้อไม่หมุน มอเตอร์มีเสียงดังผิดปกติ ขอช่างเข้ามาดูด่วน ", 2), Precedence: "bulk", SenderKnown: true}},
	}
}

func TestSwarm100ConfusionMatrix(t *testing.T) {
	cases := make([]swarmCase, 0, 100)
	cases = append(cases, buildTicketCases()...)
	cases = append(cases, buildJunkCases()...)
	cases = append(cases, buildAmbiguousCases()...)

	type scored struct {
		swarmCase
		score int
		pred  string
	}
	out := make([]scored, 0, len(cases))
	for _, c := range cases {
		sc, _ := intake.Score(c.sig)
		pred := "not"
		if sc > gateThreshold {
			pred = "ticket"
		}
		out = append(out, scored{swarmCase: c, score: sc, pred: pred})
	}

	var tp, fp, fn, tn int
	var cFP, cFN int
	bandErr := map[string]int{}
	for _, s := range out {
		switch {
		case s.truth == "ticket" && s.pred == "ticket":
			tp++
		case s.truth == "not" && s.pred == "ticket":
			fp++
			if s.band == "clear" {
				cFP++
			} else {
				bandErr["FP:"+s.subject]++
			}
		case s.truth == "ticket" && s.pred == "not":
			fn++
			if s.band == "clear" {
				cFN++
			} else {
				bandErr["FN:"+s.subject]++
			}
		default:
			tn++
		}
	}

	// only surface the ambiguous-band misses (the interesting ones)
	errs := make([]string, 0, len(bandErr))
	for k := range bandErr {
		errs = append(errs, k)
	}
	sort.Strings(errs)

	t.Logf("\nSWARM n=%d  (predictor = tier0 gate, ticket iff score > %d)\n"+
		"                 pred:ticket  pred:not\n"+
		"  truth:ticket       %3d        %3d     (TP / FN)\n"+
		"  truth:not          %3d        %3d     (FP / TN)\n"+
		"  precision=%.3f  recall=%.3f  accuracy=%.3f  f1=%.3f",
		len(out), gateThreshold, tp, fn, fp, tn,
		ratio(tp, tp+fp), ratio(tp, tp+fn), ratio(tp+tn, len(out)), f1(tp, fp, fn))

	t.Logf("clear-band errors: FP=%d FN=%d  (must be 0 — tier0 owns the easy cases)", cFP, cFN)
	t.Logf("ambiguous-band misses (%d) = the tier1 model's job:\n  %s", len(errs), strings.Join(errs, "\n  "))

	if cFP != 0 || cFN != 0 {
		t.Fatalf("tier0 failed a CLEAR case: FP=%d FN=%d", cFP, cFN)
	}
}

func f1(tp, fp, fn int) float64 {
	p, r := ratio(tp, tp+fp), ratio(tp, tp+fn)
	if p+r == 0 {
		return 0
	}
	return 2 * p * r / (p + r)
}
