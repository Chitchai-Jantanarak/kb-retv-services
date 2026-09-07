package tools

import (
	"context"
	"fmt"
	"os"
	"sort"
	"testing"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
	"github.com/my/app/internal/shared/textnorm"
	"github.com/my/app/internal/shared/vec"
)

type acceptBarCase struct {
	group string
	text  string
	want  string
}

type acceptBarScore struct {
	c      acceptBarCase
	top    string
	s1     float64
	s2     float64
	second string
}

var acceptBarCases = []acceptBarCase{
	{"read", "แสดงรายงานล่าสุด", "f1_find_cases"},
	{"read", "เคสที่ยังไม่ปิดมีอะไรบ้าง", "f1_find_cases"},
	{"read", "show open cases", "f1_find_cases"},
	{"read", "หาเคสของสัปดาห์นี้", "f1_find_cases"},
	{"read", "สถานะเคส REP-4104", "f2_case_status"},
	{"read", "REP-4097 ถึงไหนแล้ว", "f2_case_status"},
	{"read", "track REP-4102", "f2_case_status"},
	{"read", "งานของ Tanayut มีกี่เคส", "f3_employee_status"},
	{"read", "Witsarut ดูแลเคสไหนอยู่", "f3_employee_status"},
	{"read", "ทีมงานล้นมือไหม", "f4_workload"},
	{"read", "team workload this week", "f4_workload"},
	{"read", "เคสของ Bella Bot มีอะไรบ้าง", "f5_product_cases"},
	{"read", "T300 มีปัญหาอะไรบ่อย", "f5_product_cases"},
	{"read", "ลูกค้า N-Health เป็นใคร", "f6_customer_service"},
	{"read", "ข้อมูลลูกค้าโรงพยาบาลกรุงเทพ", "f6_customer_service"},
	{"read", "หุ่นยนต์ไม่เข้าลิฟต์แก้ยังไง", "f7_knowledge"},
	{"read", "how to reset the robot map", "f7_knowledge"},
	{"read", "วิธีตั้งค่าจุดจอด", "f7_knowledge"},
	{"read", "อีเมลที่เข้ามาวันนี้", "f12_inbound_read"},
	{"read", "มีเมลใหม่ไหม", "f12_inbound_read"},
	{"read", "show inbox", "f12_inbound_read"},

	{"write", "ปิดเคส REP-4104", "f10_close_case"},
	{"write", "close REP-4097", "f10_close_case"},
	{"write", "มอบหมาย REP-4104 ให้ Tanayut", "f9_assign_case"},
	{"write", "assign REP-4102 to Witsarut", "f9_assign_case"},
	{"write", "อัปเดต REP-4104 เป็นเสร็จแล้ว", "f8_update_case"},
	{"write", "เปลี่ยนสถานะ REP-4097 เป็นกำลังทำ", "f8_update_case"},
	{"write", "สร้างเคสจากเมล 4821", "f13_promote_mail"},
	{"write", "promote email 4821 to a case", "f13_promote_mail"},

	{"notcap", "ลบเคส REP-4104", ""},
	{"notcap", "delete case REP-4097", ""},
	{"notcap", "ส่งอีเมลหาลูกค้า", ""},
	{"notcap", "ขอใบเสนอราคา", ""},
	{"notcap", "จองห้องประชุม", ""},
	{"notcap", "เงินเดือนออกวันไหน", ""},
	{"notcap", "คืนเงินลูกค้า", ""},
	{"notcap", "export รายงานเป็น excel", ""},
	{"notcap", "reset password ให้หน่อย", ""},
	{"notcap", "สั่งอะไหล่ใหม่", ""},
	{"notcap", "ขอลาพักร้อน", ""},
	{"notcap", "แก้ไขชื่อลูกค้า", ""},

	{"social", "สวัสดีครับ", ""},
	{"social", "ขอบคุณมาก", ""},
	{"social", "hello", ""},
	{"social", "ทดสอบ", ""},
	{"social", "อากาศวันนี้เป็นไง", ""},
	{"social", "เล่าเรื่องตลกให้ฟัง", ""},

	{"fragment", "ไม่เอา draft", "f1_find_cases"},
	{"fragment", "อันแรกสถานะอะไร", "f2_case_status"},
	{"fragment", "แล้วใครดูแล", "f2_case_status"},
	{"fragment", "promote เคสนี้", "f13_promote_mail"},
	{"fragment", "ปิดอันนั้นเลย", "f10_close_case"},
	{"fragment", "เอาเฉพาะ T300", "f5_product_cases"},
}

func probeBoundSelector(t *testing.T) (context.Context, config.Config, ports.EmbeddingProvider, *Selector, []Tool) {
	t.Helper()
	cfg, err := config.LoadFrom("/app/config.yaml")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	full, err := Load(os.DirFS("/app/config/tools"), nil)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	catalog := make([]Tool, 0, len(full))
	for _, tl := range full {
		if contextFuzzBound[tl.Handler] {
			catalog = append(catalog, tl)
		}
	}
	guard, _, _, _ := llmboot.GuardEmbedder(cfg, nil)
	if guard == nil {
		t.Fatal("guard embedder unavailable")
	}
	ctx := context.Background()
	sel, err := NewSelector(ctx, guard, catalog, WithNameSource(probeGazetteer()))
	if err != nil {
		t.Fatalf("selector: %v", err)
	}
	return ctx, cfg, guard, sel, catalog
}

func TestProbeAcceptBar(t *testing.T) {
	if os.Getenv("PROBE") == "" {
		t.Skip("set PROBE=1")
	}
	ctx, _, guard, sel, _ := probeBoundSelector(t)
	cases := acceptBarCases

	scored := make([]acceptBarScore, 0, len(cases))
	for _, c := range cases {
		text := normalizeQuery(textnorm.NormalizeLoanwords(c.text))
		vecs, err := guard.Embed(ctx, []string{text})
		if err != nil {
			t.Fatalf("embed %q: %v", c.text, err)
		}
		q := vec.Normalize(vecs[0])
		type ts struct {
			id string
			s  float64
		}
		all := make([]ts, 0, len(sel.tools))
		for _, tv := range sel.tools {
			best := 0.0
			for _, v := range tv.vecs {
				if d := vec.Dot(q, v); d > best {
					best = d
				}
			}
			all = append(all, ts{tv.id, best})
		}
		sort.Slice(all, func(i, j int) bool { return all[i].s > all[j].s })
		scored = append(scored, acceptBarScore{c: c, top: all[0].id, s1: all[0].s, s2: all[1].s, second: all[1].id})
	}

	t.Logf("")
	t.Logf("%-9s %-36s %-18s %6s %6s %6s  want", "group", "text", "top", "s1", "s2", "margin")
	for _, r := range scored {
		t.Logf("%-9s %-36s %-18s %.3f %.3f %.3f  %s", r.c.group, r.c.text, r.top, r.s1, r.s2, r.s1-r.s2, r.c.want)
	}

	accepts := []float64{0.55, 0.60, 0.65, 0.70, 0.75, 0.80, 0.85, 0.90, 0.95}
	margins := []float64{0.00, 0.05, 0.10, 0.15}

	t.Logf("")
	t.Logf("Tier 0 answers alone when s1 >= accept AND (s1 - s2) >= margin. Everything else defers to tier 2.")
	t.Logf("%-7s %-7s %9s %8s %6s %10s %9s %10s", "accept", "margin", "answered", "correct", "wrong", "falsefire", "coverage", "precision")
	for _, a := range accepts {
		for _, m := range margins {
			answered, correct, wrong, falsefire := 0, 0, 0, 0
			for _, r := range scored {
				if r.s1 < a || (r.s1-r.s2) < m {
					continue
				}
				answered++
				switch {
				case r.c.want == "":
					falsefire++
				case r.top == r.c.want:
					correct++
				default:
					wrong++
				}
			}
			prec := 0.0
			if answered > 0 {
				prec = float64(correct) / float64(answered) * 100
			}
			t.Logf("%-7.2f %-7.2f %9d %8d %6d %10d %8.0f%% %9.0f%%",
				a, m, answered, correct, wrong, falsefire, float64(answered)/float64(len(scored))*100, prec)
		}
	}

	t.Logf("")
	t.Logf("As-built selector (real Select: params, gazetteer, per-tool accept/floor, reject margin) on the same %d cases:", len(scored))
	granted := []string{"*"}
	sAnswered, sCorrect, sWrong, sFalse := 0, 0, 0, 0
	var sWrongList, sFalseList []string
	for _, r := range scored {
		s, err := sel.Select(ctx, r.c.text, granted)
		if err != nil {
			t.Fatalf("select %q: %v", r.c.text, err)
		}
		if !s.Matched {
			continue
		}
		sAnswered++
		switch {
		case r.c.want == "":
			sFalse++
			sFalseList = append(sFalseList, fmt.Sprintf("%q->%s(%.2f)", r.c.text, s.ToolID, s.Score))
		case s.ToolID == r.c.want:
			sCorrect++
		default:
			sWrong++
			sWrongList = append(sWrongList, fmt.Sprintf("%q->%s(%.2f) want %s", r.c.text, s.ToolID, s.Score, r.c.want))
		}
	}
	sPrec := 0.0
	if sAnswered > 0 {
		sPrec = float64(sCorrect) / float64(sAnswered) * 100
	}
	t.Logf("  answered=%d correct=%d wrong=%d falsefire=%d coverage=%.0f%% precision=%.0f%%",
		sAnswered, sCorrect, sWrong, sFalse, float64(sAnswered)/float64(len(scored))*100, sPrec)
	t.Logf("  wrong: %v", sWrongList)
	t.Logf("  falsefire: %v", sFalseList)
}
