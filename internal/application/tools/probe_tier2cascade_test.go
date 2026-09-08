package tools

import (
	"context"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/my/app/internal/infra/llm/gemini"
)

type callCounter struct {
	inner ToolSelectorLike
	mu    sync.Mutex
	calls int
}

func (c *callCounter) Select(ctx context.Context, text string, perms []string) (Selection, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return c.inner.Select(ctx, text, perms)
}

type cascadeResult struct {
	c       acceptBarCase
	sel     Selection
	err     error
	latency time.Duration
}

func TestProbeCascade(t *testing.T) {
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
	tier2 := NewModelSelector(client, catalog)

	tier0Counter := &callCounter{inner: sel}
	tier2Counter := &callCounter{inner: tier2}
	cascade := NewCascade(tier0Counter, tier2Counter, 0.75, 0.15)

	cases := acceptBarCases
	granted := []string{"*"}
	results := make([]cascadeResult, len(cases))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, c := range cases {
		wg.Add(1)
		go func(i int, c acceptBarCase) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			start := time.Now()
			s, err := cascade.Select(callCtx, c.text, granted)
			results[i] = cascadeResult{c: c, sel: s, err: err, latency: time.Since(start)}
		}(i, c)
	}
	wg.Wait()

	answered, correct, wrong, falsefire, abstainOK, missed, invalid := 0, 0, 0, 0, 0, 0, 0
	var lat []time.Duration
	var wrongList, falseList, missList []string
	byGroup := map[string][2]int{}
	for _, r := range results {
		lat = append(lat, r.latency)
		g := byGroup[r.c.group]
		g[1]++
		switch {
		case r.err != nil:
			invalid++
		case !r.sel.Matched && r.c.want == "":
			abstainOK++
			g[0]++
		case !r.sel.Matched:
			missed++
			missList = append(missList, "\""+r.c.text+"\" want "+r.c.want)
		case r.c.want == "":
			answered++
			falsefire++
			falseList = append(falseList, "\""+r.c.text+"\"->"+r.sel.ToolID)
		case r.sel.ToolID == r.c.want:
			answered++
			correct++
			g[0]++
		default:
			answered++
			wrong++
			wrongList = append(wrongList, "\""+r.c.text+"\"->"+r.sel.ToolID+" want "+r.c.want)
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

	tier2Calls := tier2Counter.calls
	tier0Alone := len(cases) - tier2Calls

	t.Logf("")
	t.Logf("=== cascade (tier0=as-built selector, tier2=gemini-3.5-flash-lite, accept=0.75 margin=0.15) ===")
	t.Logf("  overall right (correct tool or correct abstain): %d/%d = %d%%", correct+abstainOK, len(results), overall)
	t.Logf("  answered=%d correct=%d wrong=%d falsefire=%d | abstained-correctly=%d missed(no_tool on capable)=%d invalid=%d | precision=%.0f%%",
		answered, correct, wrong, falsefire, abstainOK, missed, invalid, prec)
	t.Logf("  latency p50=%dms p90=%dms", p50.Milliseconds(), p90.Milliseconds())
	t.Logf("  coverage split: tier0 answered alone=%d/%d, fell through to tier2=%d/%d", tier0Alone, len(cases), tier2Calls, len(cases))
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
			t.Logf("  invalid: %q -> %v", r.c.text, r.err)
		}
	}
}
