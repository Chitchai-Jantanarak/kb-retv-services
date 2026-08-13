package tools

import (
	"context"
	"os"
	"sort"
	"testing"

	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
	vecmath "github.com/my/app/internal/shared/vec"
)

type gateCase struct {
	query string
	want  string
}

func TestProbeKnowledgeMiss(t *testing.T) {
	if os.Getenv("PROBE") == "" {
		t.Skip("set PROBE=1")
	}
	cfg, err := config.LoadFrom("/app/config.yaml")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	catalog, err := Load(os.DirFS("/app/config/tools"), nil)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	guard, _, _, gerr := llmboot.GuardEmbedder(cfg, nil)
	if guard == nil {
		t.Fatalf("guard embedder unavailable: %v (dir=%q provider=%q)", gerr, cfg.Chat.GuardEmbedderAssetDir, cfg.Chat.GuardEmbedderProvider)
	}
	ctx := context.Background()
	sel, err := NewSelector(ctx, guard, catalog, WithNameSource(probeGazetteer()))
	if err != nil {
		t.Fatalf("selector: %v", err)
	}

	cases := []gateCase{
		{"ช่วยหาวิธีการแก้ไขปัญหาจากเครื่องบูตไม่ติด", "f7_knowledge"},
		{"หุ่นยนต์บูตไม่ติด แก้ยังไง", "f7_knowledge"},
		{"เคยเจอปัญหาลิฟต์ไม่เปิดไหม แก้ยังไง", "f7_knowledge"},
		{"how do i fix a machine that will not boot", "f7_knowledge"},
		{"what the latest report", "f1_find_cases"},
		{"หาเคสที่ยังรออยู่", "f1_find_cases"},
		{"รายงานล่าสุดที่ต้องดู", "f1_find_cases"},
		{"show me tickets waiting", "f1_find_cases"},
		{"ปัญหาใกล้กับ rep4105", "f7_knowledge"},
		{"หาเคสที่คล้ายกับ REP-4105", "f7_knowledge"},
		{"status of rep-4106", "f2_case_status"},
		{"เคส REP-4106 ตอนนี้เป็นยังไง", "f2_case_status"},
		{"cases for bella bot", "f5_product_cases"},
		{"ปัญหาของ T300 มีอะไรบ้าง", "f5_product_cases"},
		{"เปิดเคสใหม่ให้หน่อย หุ่นยนต์เสีย", "open_case"},
		{"อยากแจ้งปัญหาใหม่", "open_case"},
		{"แจ้งซ่อมหุ่นยนต์ตัวนี้หน่อย", "open_case"},
		{"ขอดูเคสทั้งหมดที่ยังไม่เสร็จ", "f1_find_cases"},
		{"ปิดเคส REP-4106 ให้หน่อย", "f10_close_case"},
		{"what is somchai prasert working on", "f3_employee_status"},
		{"who is free right now", "f4_workload"},
		{"workload of the support team", "f4_workload"},
	}

	granted := []string{"*"}
	pass, fail := 0, 0
	for _, c := range cases {
		got, err := sel.Select(ctx, c.query, granted)
		if err != nil {
			t.Fatalf("select %q: %v", c.query, err)
		}
		mark := "FAIL"
		if got.Matched && got.ToolID == c.want {
			mark = "ok"
			pass++
		} else {
			fail++
		}
		picked := got.ToolID
		if !got.Matched {
			picked = "(none) near=" + got.ToolID
		}
		t.Logf("%-4s %-46s want=%-18s got=%-18s %.3f", mark, c.query, c.want, picked, got.Score)
		if mark == "FAIL" {
			for _, r := range rankTools(sel, c.query, ctx) {
				t.Logf("        %-20s %.3f accept=%.2f req=%d", r.id, r.score, r.accept, r.req)
			}
		}
	}
	t.Logf("gate: %d/%d", pass, pass+fail)
}

type rankRow struct {
	id     string
	score  float64
	accept float64
	req    int
}

func rankTools(s *Selector, q string, ctx context.Context) []rankRow {
	vec := vecmath.Normalize(mustEmbedOne(ctx, s, q))
	rows := make([]rankRow, 0, len(s.tools))
	for _, tv := range s.tools {
		best := 0.0
		for _, v := range tv.vecs {
			if d := vecmath.Dot(vec, v); d > best {
				best = d
			}
		}
		rows = append(rows, rankRow{tv.id, best, tv.accept, requiredCount(tv.params)})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].score > rows[j].score })
	if len(rows) > 4 {
		rows = rows[:4]
	}
	return rows
}
