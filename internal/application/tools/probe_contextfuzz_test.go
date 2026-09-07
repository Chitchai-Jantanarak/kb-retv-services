package tools

import (
	"context"
	"os"
	"testing"

	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
)

type fuzzCase struct {
	group string
	raw   string
	rw    string
	want  string
}

var contextFuzzBound = map[string]bool{
	"knowledge": true, "reports.update": true, "reports.close": true,
	"reports.assign": true, "reports.find": true, "reports.track": true,
	"reports.byProduct": true, "employee.status": true, "workload": true,
	"customer.profile": true, "inbound.read": true, "promote.mail": true,
}

func TestProbeContextFuzz(t *testing.T) {
	if os.Getenv("PROBE") == "" {
		t.Skip("set PROBE=1")
	}
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
	granted := []string{"*"}
	t.Logf("bound catalog tools=%d (of %d)", len(catalog), len(full))

	cases := []fuzzCase{
		{"followup-ellipsis", "ไม่เอา draft", "แสดงรายงานล่าสุด ไม่เอา draft", "f1_find_cases"},
		{"followup-ellipsis", "เอาแบบปิดแล้ว", "แสดงเคสที่ปิดแล้ว", "f1_find_cases"},
		{"followup-pronoun", "อันแรกสถานะอะไร", "สถานะของเคส REP-4104 คืออะไร", "f2_case_status"},
		{"followup-pronoun", "แล้วเคสนั้นใครดูแล", "ใครรับผิดชอบเคส REP-4104", "f2_case_status"},
		{"followup-pronoun", "promote เคสนี้", "สร้างเคสจากเมลที่ได้รับ 4821", "f13_promote_mail"},
		{"codeswitch", "อยากได้ report ล่าสุดไม่เอา draft", "อยากได้รายงานล่าสุด ไม่เอาฉบับร่าง", "f1_find_cases"},
		{"codeswitch", "ขอ status ของ case นี้", "ขอสถานะของเคส REP-4104", "f2_case_status"},
		{"codeswitch", "check งาน ของ employee คนนี้", "ตรวจสอบงานของพนักงาน Tanayut", "f3_employee_status"},
		{"short-input", "หาเคส", "หาเคสทั้งหมด", "f1_find_cases"},
		{"short-input", "เมล", "แสดงอีเมลที่เข้ามา", "f12_inbound_read"},
		{"capable-write", "ปิดเคส REP-4104", "ปิดเคส REP-4104", "f10_close_case"},
		{"capable-write", "มอบหมายเคส REP-4104 ให้ Tanayut", "มอบหมายเคส REP-4104 ให้ Tanayut", "f9_assign_case"},
		{"capable-read", "งานของ Tanayut เป็นยังไง", "งานของพนักงาน Tanayut เป็นอย่างไร", "f3_employee_status"},
		{"capable-read", "หุ่นยนต์ไม่เข้าลิฟต์แก้ยังไง", "วิธีแก้ปัญหาหุ่นยนต์ไม่เข้าลิฟต์", "f7_knowledge"},
		{"NOT-capable", "ขอใบเสนอราคาหน่อย", "ขอใบเสนอราคาสำหรับลูกค้ารายนี้", ""},
		{"NOT-capable", "ลบเคสนี้ทิ้ง", "ลบเคส REP-4104 ออกจากระบบ", ""},
		{"NOT-capable", "ส่งอีเมลหาลูกค้า", "ส่งอีเมลแจ้งลูกค้าเรื่องเคสนี้", ""},
		{"NOT-capable", "จองห้องประชุมพรุ่งนี้", "จองห้องประชุมสำหรับพรุ่งนี้", ""},
		{"NOT-capable", "เงินเดือนออกวันไหน", "เงินเดือนพนักงานออกวันไหน", ""},
		{"ambiguous-verb", "อัปเดตสถานะเคส REP-4104", "อัปเดตสถานะเคส REP-4104 เป็นเสร็จแล้ว", "f8_update_case"},
		{"ambiguous-verb", "เช็คสถานะเคส REP-4104", "ตรวจสอบสถานะเคส REP-4104", "f2_case_status"},
		{"social", "สวัสดีครับ", "สวัสดีครับ", ""},
		{"social", "ขอบคุณมาก", "ขอบคุณมากครับ", ""},
	}

	score := func(text string) Selection {
		s, err := sel.Select(ctx, text, granted)
		if err != nil {
			t.Fatalf("select %q: %v", text, err)
		}
		return s
	}
	verdict := func(s Selection, want string) string {
		got := ""
		if s.Matched {
			got = s.ToolID
		}
		switch {
		case got == want:
			return "OK"
		case want == "" && got != "":
			return "FALSE-FIRE"
		case want != "" && got == "":
			return "MISS"
		default:
			return "WRONG"
		}
	}

	var rawOK, rwOK, gained, lost int
	t.Logf("")
	t.Logf("%-18s %-34s %-16s %-10s | %-16s %-10s", "group", "raw text", "raw tool", "raw", "rewritten tool", "rw")
	for _, c := range cases {
		a, b := score(c.raw), score(c.rw)
		va, vb := verdict(a, c.want), verdict(b, c.want)
		if va == "OK" {
			rawOK++
		}
		if vb == "OK" {
			rwOK++
		}
		if va != "OK" && vb == "OK" {
			gained++
		}
		if va == "OK" && vb != "OK" {
			lost++
		}
		at, bt := "-", "-"
		if a.Matched {
			at = a.ToolID
		}
		if b.Matched {
			bt = b.ToolID
		}
		t.Logf("%-18s %-34s %-16s %-4s %.3f | %-16s %-4s %.3f  want=%s",
			c.group, c.raw, at, va, a.Score, bt, vb, b.Score, c.want)
	}
	t.Logf("")
	t.Logf("TOTAL %d cases | raw correct=%d  rewritten correct=%d | gained=%d lost=%d",
		len(cases), rawOK, rwOK, gained, lost)
}
