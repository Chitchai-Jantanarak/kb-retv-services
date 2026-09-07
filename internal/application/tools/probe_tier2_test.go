package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/llm/gemini"
)

type tier2Result struct {
	c       acceptBarCase
	toolID  string
	params  map[string]string
	raw     string
	latency time.Duration
	err     error
}

func tier2ToolList(catalog []Tool) string {
	var b strings.Builder
	for _, tl := range catalog {
		names := make([]string, 0, len(tl.Params))
		for _, p := range tl.Params {
			names = append(names, p.Name)
		}
		fmt.Fprintf(&b, "- %s: %s", tl.ID, tl.Description)
		if len(names) > 0 {
			fmt.Fprintf(&b, " (params: %s)", strings.Join(names, ", "))
		}
		b.WriteString("\n")
	}
	b.WriteString("- no_tool: the request needs none of the tools above, asks for something no tool can do, or is small talk\n")
	return b.String()
}

const tier2System = `You route one support-desk chat message to exactly one tool from the list, or to no_tool.
Pick no_tool when the request asks for an action no tool performs (delete, send, refund, quote, export, booking, HR), when it is small talk, or when you are not confident which tool applies.
Never invent case codes, names, or ids that are not in the message. Copy them exactly as written.
Reply with JSON only, no prose: {"tool_id": "<id or no_tool>", "params": {"<name>": "<value>"}}

Tools:
%s`

func runTier2(ctx context.Context, provider ports.LLMProvider, model, system string, cases []acceptBarCase, valid map[string]bool) []tier2Result {
	out := make([]tier2Result, len(cases))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	var think *int
	if !strings.HasPrefix(model, "gemini-3") {
		z := 0
		think = &z
	}
	for i, c := range cases {
		wg.Add(1)
		go func(i int, c acceptBarCase) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			start := time.Now()
			comp, err := provider.GenerateJSON(callCtx, ports.Prompt{
				System:      system,
				User:        "Message: " + c.text,
				MaxToks:     120,
				ThinkBudget: think,
				Temp:        0,
			})
			r := tier2Result{c: c, latency: time.Since(start), err: err}
			if err == nil {
				r.raw = strings.TrimSpace(comp.Text)
				var parsed struct {
					ToolID string            `json:"tool_id"`
					Params map[string]string `json:"params"`
				}
				if jerr := json.Unmarshal([]byte(r.raw), &parsed); jerr != nil {
					r.err = fmt.Errorf("bad json: %v", jerr)
				} else {
					r.toolID = strings.TrimSpace(parsed.ToolID)
					r.params = parsed.Params
					if r.toolID != "no_tool" && !valid[r.toolID] {
						r.err = fmt.Errorf("unknown tool %q", r.toolID)
					}
				}
			}
			out[i] = r
		}(i, c)
	}
	wg.Wait()
	return out
}

func TestProbeTier2(t *testing.T) {
	if os.Getenv("PROBE") == "" {
		t.Skip("set PROBE=1")
	}
	ctx, cfg, _, _, catalog := probeBoundSelector(t)
	if cfg.APIKeys.Gemini == "" {
		t.Skip("no gemini key")
	}
	valid := make(map[string]bool, len(catalog))
	for _, tl := range catalog {
		valid[tl.ID] = true
	}
	system := fmt.Sprintf(tier2System, tier2ToolList(catalog))
	t.Logf("declaration set: %d tools + no_tool, system prompt %d bytes", len(catalog), len(system))

	models := []string{"gemini-2.5-flash-lite", "gemini-2.5-flash"}
	if m := os.Getenv("TIER2_MODELS"); m != "" {
		models = strings.Split(m, ",")
	}

	for _, model := range models {
		provider, err := gemini.New(gemini.Config{APIKey: cfg.APIKeys.Gemini, Model: model})
		if err != nil {
			t.Fatalf("gemini %s: %v", model, err)
		}
		results := runTier2(ctx, provider, model, system, acceptBarCases, valid)

		answered, correct, wrong, falsefire, abstainOK, missed, invalid := 0, 0, 0, 0, 0, 0, 0
		var lat []time.Duration
		var wrongList, falseList, missList []string
		byGroup := map[string][2]int{}
		for _, r := range results {
			lat = append(lat, r.latency)
			g := byGroup[r.c.group]
			g[1]++
			if r.err != nil {
				invalid++
				byGroup[r.c.group] = g
				continue
			}
			switch {
			case r.toolID == "no_tool" && r.c.want == "":
				abstainOK++
				g[0]++
			case r.toolID == "no_tool":
				missed++
				missList = append(missList, fmt.Sprintf("%q want %s", r.c.text, r.c.want))
			case r.c.want == "":
				answered++
				falsefire++
				falseList = append(falseList, fmt.Sprintf("%q->%s", r.c.text, r.toolID))
			case r.toolID == r.c.want:
				answered++
				correct++
				g[0]++
			default:
				answered++
				wrong++
				wrongList = append(wrongList, fmt.Sprintf("%q->%s want %s", r.c.text, r.toolID, r.c.want))
			}
			byGroup[r.c.group] = g
		}
		sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
		p50, p90 := lat[len(lat)/2], lat[len(lat)*9/10]
		prec := 0.0
		if answered > 0 {
			prec = float64(correct) / float64(answered) * 100
		}
		overall := (correct + abstainOK) * 100 / len(results)

		t.Logf("")
		t.Logf("=== %s ===", model)
		t.Logf("  overall right (correct tool or correct abstain): %d/%d = %d%%", correct+abstainOK, len(results), overall)
		t.Logf("  answered=%d correct=%d wrong=%d falsefire=%d | abstained-correctly=%d missed(no_tool on capable)=%d invalid=%d | precision=%.0f%%",
			answered, correct, wrong, falsefire, abstainOK, missed, invalid, prec)
		t.Logf("  latency p50=%dms p90=%dms", p50.Milliseconds(), p90.Milliseconds())
		groups := []string{"read", "write", "notcap", "social", "fragment"}
		for _, g := range groups {
			v := byGroup[g]
			t.Logf("  %-9s %d/%d", g, v[0], v[1])
		}
		t.Logf("  wrong: %v", wrongList)
		t.Logf("  falsefire: %v", falseList)
		t.Logf("  missed: %v", missList)
		for _, r := range results {
			if r.err != nil {
				t.Logf("  invalid: %q -> %v raw=%q", r.c.text, r.err, r.raw)
			}
		}
		codeOK, codeTotal := 0, 0
		for _, r := range results {
			if r.err != nil || r.toolID == "no_tool" || !strings.Contains(r.c.text, "REP-") {
				continue
			}
			codeTotal++
			for _, v := range r.params {
				if strings.Contains(r.c.text, v) && strings.HasPrefix(v, "REP-") {
					codeOK++
					break
				}
			}
		}
		t.Logf("  case code copied verbatim into params: %d/%d", codeOK, codeTotal)
	}
}
