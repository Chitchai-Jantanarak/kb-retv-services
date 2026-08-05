package tools

import (
	"context"
	"testing"
)

func TestMatchLookupNameExactContainment(t *testing.T) {
	names := []string{"WiFi", "Monitor WIFI SSID", "Grafana"}
	if got := matchLookupName(names, "cases for wifi please"); got != "WiFi" {
		t.Fatalf("got %q, want WiFi", got)
	}
}

func TestMatchLookupNamePrefersLongestExact(t *testing.T) {
	names := []string{"WiFi", "Monitor WIFI SSID"}
	if got := matchLookupName(names, "ปัญหา monitor wifi ssid ที่สยาม"); got != "Monitor WIFI SSID" {
		t.Fatalf("got %q, want Monitor WIFI SSID", got)
	}
}

func TestMatchLookupNameTokenFallback(t *testing.T) {
	names := []string{"Siam Michelin HQ", "Kantary Hotel"}
	if got := matchLookupName(names, "customer profile for michelin"); got != "Siam Michelin HQ" {
		t.Fatalf("got %q, want Siam Michelin HQ", got)
	}
}

func TestMatchLookupNameAmbiguousTokenTieReturnsEmpty(t *testing.T) {
	names := []string{"Siam Michelin HQ", "Siam Paragon"}
	if got := matchLookupName(names, "cases at siam"); got != "" {
		t.Fatalf("got %q, want empty on ambiguous tie", got)
	}
}

func TestMatchLookupNameNoMatch(t *testing.T) {
	names := []string{"WiFi", "Grafana"}
	if got := matchLookupName(names, "the internet thing is broken"); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

type flakyEmbedder struct {
	inner    fakeEmbedder
	failures int
	calls    int
}

func (f *flakyEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	f.calls++
	if f.calls <= f.failures {
		return nil, context.DeadlineExceeded
	}
	return f.inner.Embed(ctx, texts)
}

func TestSelectRetriesEmbedOnce(t *testing.T) {
	flaky := &flakyEmbedder{inner: emb(), failures: 1}
	s, err := NewSelector(context.Background(), emb(), twoTools())
	if err != nil {
		t.Fatalf("NewSelector: %v", err)
	}
	s.emb = flaky

	got, err := s.Select(context.Background(), "show me cases", []string{"report.view"})
	if err != nil {
		t.Fatalf("Select after one embed failure: %v", err)
	}
	if got.ToolID != "f1" || !got.Matched {
		t.Fatalf("got %+v, want f1 matched", got)
	}

	flaky = &flakyEmbedder{inner: emb(), failures: 2}
	s.emb = flaky
	if _, err := s.Select(context.Background(), "show me cases", []string{"report.view"}); err == nil {
		t.Fatal("want error when embed fails twice")
	}
}

type fakeNames struct {
	byLookup map[string][]string
	calls    int
}

func (f *fakeNames) Names(_ context.Context, lookup string) ([]string, error) {
	f.calls++
	return f.byLookup[lookup], nil
}

func lookupTools() []Tool {
	return []Tool{
		{ID: "f5_product", Anchors: []string{"cases for product"}, Thresholds: Thresholds{Accept: 0.55}, RBAC: RBAC{RequiresPermission: "report.view"},
			Params: []Param{{Name: "product", Required: true, Lookup: "nodes.title"}}},
		{ID: "f1_find", Anchors: []string{"latest cases"}, Thresholds: Thresholds{Accept: 0.55}, RBAC: RBAC{RequiresPermission: "report.view"}},
	}
}

func lookupEmb() fakeEmbedder {
	return fakeEmbedder{table: map[string][]float32{
		"cases for product": {1, 0},
		"latest cases":      {0.7, 0.71},
		"cases for wifi":    {0.98, 0.2},
	}}
}

func TestSelectResolvesLookupParam(t *testing.T) {
	src := &fakeNames{byLookup: map[string][]string{"nodes.title": {"WiFi", "Grafana"}}}
	s, err := NewSelector(context.Background(), lookupEmb(), lookupTools(), WithNameSource(src))
	if err != nil {
		t.Fatalf("NewSelector: %v", err)
	}

	got, err := s.Select(context.Background(), "cases for wifi", []string{"report.view"})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if got.ToolID != "f5_product" || !got.Matched {
		t.Fatalf("want f5_product matched, got %+v", got)
	}
	if got.Params["product"] != "WiFi" {
		t.Fatalf("params = %v, want canonical WiFi", got.Params)
	}
}

func TestSelectExcludesLookupToolWhenNoNameFound(t *testing.T) {
	src := &fakeNames{byLookup: map[string][]string{"nodes.title": {"Grafana"}}}
	s, err := NewSelector(context.Background(), lookupEmb(), lookupTools(), WithNameSource(src))
	if err != nil {
		t.Fatalf("NewSelector: %v", err)
	}

	got, err := s.Select(context.Background(), "cases for wifi", []string{"report.view"})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if got.ToolID != "f1_find" || !got.Matched {
		t.Fatalf("want fallback to f1_find when product unresolvable, got %+v", got)
	}
}

func TestSelectLookupToolWithoutSourceIsUnsatisfiable(t *testing.T) {
	s, err := NewSelector(context.Background(), lookupEmb(), lookupTools())
	if err != nil {
		t.Fatalf("NewSelector: %v", err)
	}

	got, err := s.Select(context.Background(), "cases for wifi", []string{"report.view"})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if got.ToolID != "f1_find" {
		t.Fatalf("want f1_find when no name source wired, got %+v", got)
	}
}
