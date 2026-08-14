package intake_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/my/app/internal/application/workflows/intake"
)

func TestRankingDemo(t *testing.T) {
	type sample struct {
		kind    string
		subject string
		sig     intake.Signals
	}

	samples := []sample{
		{
			kind:    "ticket",
			subject: "PuduBot ชั้น 3 ขึ้น error E-Stop (ตามเคส REP-4106)",
			sig: intake.Signals{
				Sender:         "somchai@grandhotel.co.th",
				Body:           "หุ่นยนต์ PuduBot ที่ชั้น 3 หยุดวิ่งตั้งแต่เช้า ขึ้น error E-Stop ค้างที่หน้าลิฟต์ ตามเคสเดิม REP-4106 ที่แจ้งไว้ รบกวนช่วยดูด่วนครับ",
				ReferencedCase: "REP-4106",
				ThreadMatched:  true,
				HasAttachments: true,
				SenderKnown:    true,
			},
		},
		{
			kind:    "ticket",
			subject: "T300 เข้า dock ไม่ได้ ค้างที่ lobby",
			sig: intake.Signals{
				Sender:        "facility@sirapark.com",
				Body:          "เครื่อง T300 ตัวที่ 2 เข้า dock ชาร์จไม่ได้เลยตั้งแต่เมื่อคืน วนอยู่แถว lobby แล้วแบตหมด ขอทีมช่างเข้ามาเช็คหน่อยครับ มีรูปหน้าจอ error แนบมาด้วย",
				ThreadMatched: true,
				HasAttachments: true,
				SenderKnown:   true,
			},
		},
		{
			kind:    "ticket",
			subject: "หุ่นยนต์เสิร์ฟล้มที่โต๊ะ 12",
			sig: intake.Signals{
				Sender:         "manager@zenrestaurant.co.th",
				Body:           "หุ่นยนต์เสิร์ฟอาหารล้มขณะวิ่งผ่านโต๊ะ 12 จานแตกเสียหาย ตอนนี้เปิดไม่ติดแล้ว รบกวนส่งช่างมาดูด่วน แนบคลิปตอนล้มมาให้",
				HasAttachments: true,
			},
		},
		{
			kind:    "ticket",
			subject: "Robot No.1 หยุดทำงาน",
			sig: intake.Signals{
				Sender: "ops@customer.co.th",
				Body:   "16.23 น. Robot No.1 หยุดทำงาน No.12",
			},
		},
		{
			kind:    "not-ticket",
			subject: "PuduTech Monthly Newsletter — New Features!",
			sig: intake.Signals{
				Sender:          "no-reply@news.pudurobotics.com",
				Body:            strings.Repeat("Check our new robots at https://pudu.com/a and https://pudu.com/b today! ", 5),
				ListUnsubscribe: true,
				Precedence:      "bulk",
			},
		},
		{
			kind:    "not-ticket",
			subject: "Your invoice #INV-2231 is ready",
			sig: intake.Signals{
				Sender:        "no-reply@billing.vendor.com",
				Body:          "Your monthly invoice is now available in the portal.",
				AutoSubmitted: "auto-generated",
			},
		},
		{
			kind:    "not-ticket",
			subject: "🎉 50% OFF robot accessories this week only",
			sig: intake.Signals{
				Sender:          "promo@robotshop.com",
				Body:            strings.Repeat("Buy now https://shop.example/x limited offer https://shop.example/y ", 6),
				ListUnsubscribe: true,
			},
		},
	}

	type scored struct {
		sample
		score   int
		reasons []string
	}

	ranked := make([]scored, 0, len(samples))
	for _, s := range samples {
		sc, rs := intake.Score(s.sig)
		ranked = append(ranked, scored{sample: s, score: sc, reasons: rs})
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })

	var b strings.Builder
	b.WriteString("\n  #  score  kind        reasons                                              subject\n")
	b.WriteString("  -- -----  ----------  ---------------------------------------------------  -------\n")
	for i, r := range ranked {
		b.WriteString(fmt.Sprintf("  %2d  %3d   %-10s  %-51s  %s\n",
			i+1, r.score, r.kind, strings.Join(r.reasons, ","), r.subject))
	}
	t.Log(b.String())

	minTicket, maxJunk := 100, 0
	for _, r := range ranked {
		if r.kind == "ticket" && r.score < minTicket {
			minTicket = r.score
		}
		if r.kind == "not-ticket" && r.score > maxJunk {
			maxJunk = r.score
		}
	}
	t.Logf("lowest ticket=%d  highest not-ticket=%d  gap=%d", minTicket, maxJunk, minTicket-maxJunk)
	if minTicket <= maxJunk {
		t.Fatalf("ranking overlap: a not-ticket (%d) scored >= a ticket (%d)", maxJunk, minTicket)
	}
}
