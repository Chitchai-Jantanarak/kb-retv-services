//go:build tokenizers

package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/my/app/internal/ai/embeddings"
)

type selGateCase struct {
	text     string
	wantTool string
	note     string
}

var boundHandlers = map[string]bool{
	"knowledge":         true,
	"reports.update":    true,
	"reports.close":     true,
	"reports.assign":    true,
	"reports.find":      true,
	"reports.track":     true,
	"reports.byProduct": true,
	"employee.status":   true,
	"workload":          true,
	"customer.profile":  true,
	"inbound.read":      true,
	"promote.mail":      true,
}

var knownUnbound = map[string]bool{
	"reports.create": true,
}

func gateCatalogDir() string {
	if dir := os.Getenv("TOOL_CONFIG_DIR"); dir != "" {
		return dir
	}
	return filepath.Join("..", "..", "..", "config", "tools")
}

func TestCatalogBindingDrift(t *testing.T) {
	catalog, err := Load(os.DirFS(gateCatalogDir()), nil)
	if err != nil {
		t.Skipf("tool catalog unavailable: %v", err)
	}
	for _, tl := range catalog {
		if !boundHandlers[tl.Handler] && !knownUnbound[tl.Handler] {
			t.Errorf("tool %s handler %q is neither bound nor known-unbound; sync boundHandlers with cmd/api/toolbroker.go",
				tl.ID, tl.Handler)
		}
	}
}

func selGateSelector(t *testing.T) (*Selector, map[string]string) {
	t.Helper()

	assetDir := os.Getenv("GUARD_ASSET_DIR")
	if assetDir == "" {
		assetDir = filepath.Join("..", "..", "..", "assets", "guard-embedder")
	}
	emb, err := embeddings.NewStaticLocalProvider(assetDir)
	if err != nil {
		t.Skipf("guard asset unavailable: %v", err)
	}

	catalog, err := Load(os.DirFS(gateCatalogDir()), nil)
	if err != nil {
		t.Skipf("tool catalog unavailable: %v", err)
	}

	bound := make([]Tool, 0, len(catalog))
	kinds := make(map[string]string, len(catalog))
	for _, tl := range catalog {
		if !boundHandlers[tl.Handler] {
			continue
		}
		bound = append(bound, tl)
		kinds[tl.ID] = tl.Kind
	}

	sel, err := NewSelector(context.Background(), emb, bound, WithNameSource(probeGazetteer()))
	if err != nil {
		t.Fatalf("selector: %v", err)
	}
	return sel, kinds
}

func runSelGate(t *testing.T, cases []selGateCase) {
	t.Helper()
	sel, kinds := selGateSelector(t)
	granted := []string{"*"}

	for _, c := range cases {
		got, err := sel.Select(context.Background(), c.text, granted)
		if err != nil {
			t.Fatalf("Select(%q): %v", c.text, err)
		}

		switch c.wantTool {
		case "":
			if got.Matched {
				t.Errorf("Select(%q) matched %s @ %.4f, want no match (%s)",
					c.text, got.ToolID, got.Score, c.note)
			}
		default:
			if !got.Matched {
				t.Errorf("Select(%q) no match (top %s @ %.4f), want %s (%s)",
					c.text, got.ToolID, got.Score, c.wantTool, c.note)
				continue
			}
			if got.ToolID != c.wantTool {
				t.Errorf("Select(%q) matched %s @ %.4f, want %s (%s)",
					c.text, got.ToolID, got.Score, c.wantTool, c.note)
			}
		}

		if got.Matched && kinds[got.ToolID] == "write" {
			t.Errorf("SAFETY Select(%q) selected WRITE tool %s @ %.4f on a read-intent utterance",
				c.text, got.ToolID, got.Score)
		}
	}
}

func TestSelectorGate(t *testing.T) {
	runSelGate(t, []selGateCase{
		{"ขอข้อมูล report", "f1_find_cases", "th+en partial, 0.7415"},
		{"รายงานล่าสุด", "f1_find_cases", "verbatim th anchor, 1.0000"},
		{"หาเคส", "f1_find_cases", "verbatim th anchor, 1.0000"},
		{"latest cases i should look at", "f1_find_cases", "verbatim en anchor, 1.0000"},
		{"show me the latest reports excluding drafts", "f1_find_cases", "en, negation, 0.6542"},
		{"อยากได้รายงานล่าสุด", "f1_find_cases", "pure th, 0.7938"},
		{"อยากได้รายงานล่าสุด ไม่เอาฉบับร่าง", "f1_find_cases", "pure th + negation, 0.6397"},

		{"สวัสดีครับ", "", "social, top was f7 @ 0.1569"},
		{"given my data", "", "contentless, top was f12 @ 0.3595"},
		{"ไม่เอา draft", "", "bare follow-up unscoreable, top was f12 @ 0.2660"},
	})
}

func TestSelectorGateCodeSwitch(t *testing.T) {
	runSelGate(t, []selGateCase{
		{"อยากได้ report ล่าสุดไม่เอา draft", "f1_find_cases", "code-switch, was 0.4910 vs accept 0.55"},
		{"ขอ report ล่าสุด", "f1_find_cases", "code-switch, short"},
		{"หา case ที่ยังไม่ปิด", "f1_find_cases", "code-switch, was f7_knowledge @ 0.6053"},
	})
}
