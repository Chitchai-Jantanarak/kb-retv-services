package tools

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/my/app/internal/infra/llm/gemini"
	"github.com/my/app/internal/shared/ctxkey"
)

type cascadeFuzzCase struct {
	group      string
	raw        string
	rw         string
	want       string
	transcript string
}

const cascadeFuzzTranscriptEmployee = `User: งานของ Tanayut เป็นยังไง
Assistant: Tanayut มี 3 เคสที่กำลังทำ
`

// The 23 cases of the retired contextualize fuzz probe, now scored through the
// production Cascade (tier 0 at the safe bar, else tier 2 with the transcript
// window). Column "raw" is what the selector sees once contextualize is gone;
// column "rewrite" is what it saw while the rewrite step fed it.
var cascadeFuzzCases = []cascadeFuzzCase{
	{"followup-ellipsis", "ไม่เอา draft", "แสดงรายงานล่าสุด ไม่เอา draft", "f1_find_cases", fragTranscriptCases},
	{"followup-ellipsis", "เอาแบบปิดแล้ว", "แสดงเคสที่ปิดแล้ว", "f1_find_cases", fragTranscriptCases},
	{"followup-pronoun", "อันแรกสถานะอะไร", "สถานะของเคส REP-4104 คืออะไร", "f2_case_status", fragTranscriptCases},
	{"followup-pronoun", "แล้วเคสนั้นใครดูแล", "ใครรับผิดชอบเคส REP-4104", "f2_case_status", fragTranscriptCases},
	{"followup-pronoun", "promote เคสนี้", "สร้างเคสจากเมลที่ได้รับ 4821", "f13_promote_mail", fragTranscriptMail},
	{"codeswitch", "อยากได้ report ล่าสุดไม่เอา draft", "อยากได้รายงานล่าสุด ไม่เอาฉบับร่าง", "f1_find_cases", ""},
	{"codeswitch", "ขอ status ของ case นี้", "ขอสถานะของเคส REP-4104", "f2_case_status", fragTranscriptCases},
	{"codeswitch", "check งาน ของ employee คนนี้", "ตรวจสอบงานของพนักงาน Tanayut", "f3_employee_status", cascadeFuzzTranscriptEmployee},
	{"short-input", "หาเคส", "หาเคสทั้งหมด", "f1_find_cases", ""},
	{"short-input", "เมล", "แสดงอีเมลที่เข้ามา", "f12_inbound_read", ""},
	{"capable-write", "ปิดเคส REP-4104", "ปิดเคส REP-4104", "f10_close_case", ""},
	{"capable-write", "มอบหมายเคส REP-4104 ให้ Tanayut", "มอบหมายเคส REP-4104 ให้ Tanayut", "f9_assign_case", ""},
	{"capable-read", "งานของ Tanayut เป็นยังไง", "งานของพนักงาน Tanayut เป็นอย่างไร", "f3_employee_status", ""},
	{"capable-read", "หุ่นยนต์ไม่เข้าลิฟต์แก้ยังไง", "วิธีแก้ปัญหาหุ่นยนต์ไม่เข้าลิฟต์", "f7_knowledge", ""},
	{"NOT-capable", "ขอใบเสนอราคาหน่อย", "ขอใบเสนอราคาสำหรับลูกค้ารายนี้", "", ""},
	{"NOT-capable", "ลบเคสนี้ทิ้ง", "ลบเคส REP-4104 ออกจากระบบ", "", fragTranscriptCases},
	{"NOT-capable", "ส่งอีเมลหาลูกค้า", "ส่งอีเมลแจ้งลูกค้าเรื่องเคสนี้", "", fragTranscriptCases},
	{"NOT-capable", "จองห้องประชุมพรุ่งนี้", "จองห้องประชุมสำหรับพรุ่งนี้", "", ""},
	{"NOT-capable", "เงินเดือนออกวันไหน", "เงินเดือนพนักงานออกวันไหน", "", ""},
	{"ambiguous-verb", "อัปเดตสถานะเคส REP-4104", "อัปเดตสถานะเคส REP-4104 เป็นเสร็จแล้ว", "f8_update_case", ""},
	{"ambiguous-verb", "เช็คสถานะเคส REP-4104", "ตรวจสอบสถานะเคส REP-4104", "f2_case_status", ""},
	{"social", "สวัสดีครับ", "สวัสดีครับ", "", ""},
	{"social", "ขอบคุณมาก", "ขอบคุณมากครับ", "", ""},
}

var cascadeFuzzWriteTools = map[string]bool{
	"f8_update_case": true, "f9_assign_case": true, "f10_close_case": true, "f13_promote_mail": true,
}

func TestProbeCascadeFuzz(t *testing.T) {
	if os.Getenv("PROBE") == "" {
		t.Skip("set PROBE=1")
	}
	ctx, cfg, _, sel, catalog := probeBoundSelector(t)
	if cfg.APIKeys.Gemini == "" {
		t.Skip("no gemini key")
	}
	client, err := gemini.New(gemini.Config{APIKey: cfg.APIKeys.Gemini, Model: "gemini-3.5-flash-lite"})
	if err != nil {
		t.Fatalf("gemini: %v", err)
	}
	cascade := NewCascade(sel, NewModelSelector(client, catalog), 0.75, 0.15)
	granted := []string{"*"}

	type row struct {
		raw, rw Selection
		rawErr  error
		rwErr   error
	}
	rows := make([]row, len(cascadeFuzzCases))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	run := func(text, transcript string) (Selection, error) {
		sem <- struct{}{}
		defer func() { <-sem }()
		callCtx := ctx
		if transcript != "" {
			callCtx = ctxkey.WithTranscript(ctx, transcript)
		}
		callCtx, cancel := context.WithTimeout(callCtx, 25*time.Second)
		defer cancel()
		return cascade.Select(callCtx, text, granted)
	}
	for i, c := range cascadeFuzzCases {
		wg.Add(1)
		go func(i int, c cascadeFuzzCase) {
			defer wg.Done()
			rows[i].raw, rows[i].rawErr = run(c.raw, c.transcript)
			rows[i].rw, rows[i].rwErr = run(c.rw, c.transcript)
		}(i, c)
	}
	wg.Wait()

	verdict := func(s Selection, err error, want string) string {
		got := selLabel(s.ToolID, s.Matched, err)
		switch {
		case got == want, want == "" && got == "no_tool":
			return "OK"
		case want == "" && got != "no_tool" && err == nil:
			return "FALSE-FIRE"
		case want != "" && (got == "no_tool" || err != nil):
			return "MISS"
		default:
			return "WRONG"
		}
	}
	writeMisfire := func(s Selection, err error, want string) bool {
		got := selLabel(s.ToolID, s.Matched, err)
		return err == nil && got != want && cascadeFuzzWriteTools[got]
	}

	var rawOK, rwOK, rawWrite, rwWrite int
	t.Logf("")
	t.Logf("%-18s %-34s %-18s %-10s | %-18s %-10s", "group", "raw text", "raw+ctx tool", "raw", "rewrite+ctx tool", "rw")
	for i, c := range cascadeFuzzCases {
		r := rows[i]
		rawV, rwV := verdict(r.raw, r.rawErr, c.want), verdict(r.rw, r.rwErr, c.want)
		if rawV == "OK" {
			rawOK++
		}
		if rwV == "OK" {
			rwOK++
		}
		if writeMisfire(r.raw, r.rawErr, c.want) {
			rawWrite++
		}
		if writeMisfire(r.rw, r.rwErr, c.want) {
			rwWrite++
		}
		t.Logf("%-18s %-34s %-18s %-10s | %-18s %-10s", c.group, c.raw,
			selLabel(r.raw.ToolID, r.raw.Matched, r.rawErr), rawV,
			selLabel(r.rw.ToolID, r.rw.Matched, r.rwErr), rwV)
	}
	t.Logf("")
	t.Logf("raw+transcript: %d/%d correct, %d write-tool misfires | rewrite+transcript: %d/%d correct, %d write-tool misfires",
		rawOK, len(cascadeFuzzCases), rawWrite, rwOK, len(cascadeFuzzCases), rwWrite)
}
