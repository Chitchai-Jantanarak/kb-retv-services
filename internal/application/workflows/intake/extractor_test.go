package intake_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/application/workflows/intake"
	"github.com/my/app/internal/domain/ports"
)

type fakeProvider struct {
	json   string
	err    error
	prompt ports.Prompt
}

func (f *fakeProvider) Generate(context.Context, ports.Prompt) (ports.Completion, error) {
	return ports.Completion{}, errors.New("not used")
}

func (f *fakeProvider) GenerateJSON(_ context.Context, p ports.Prompt) (ports.Completion, error) {
	f.prompt = p
	if f.err != nil {
		return ports.Completion{}, f.err
	}
	return ports.Completion{Text: f.json}, nil
}

func (f *fakeProvider) Stream(context.Context, ports.Prompt) (<-chan ports.Completion, error) {
	return nil, errors.New("not used")
}

type fakeSpecs struct {
	spec intake.Spec
	err  error
}

func (f fakeSpecs) SpecFor(context.Context, int64) (intake.Spec, error) { return f.spec, f.err }

type fakeProducts struct {
	products []string
	err      error
}

func (f fakeProducts) Products(context.Context, int64) ([]string, error) {
	return f.products, f.err
}

func newExtractor(t *testing.T, provider ports.LLMProvider, opts ...intake.Option) *intake.Extractor {
	t.Helper()
	registry, err := prompts.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	resolve := func(context.Context, int64) (ports.LLMProvider, error) { return provider, nil }
	ext, err := intake.NewExtractor(registry, resolve, opts...)
	if err != nil {
		t.Fatalf("NewExtractor() error = %v", err)
	}
	return ext
}

func TestExtractMissingProductIsIncomplete(t *testing.T) {
	provider := &fakeProvider{json: `{"problem_detail":"printer stops mid-job","product":"","contact":"a@b.com"}`}
	ext := newExtractor(t, provider)

	got, err := ext.Extract(context.Background(), 7, "Printer down", "It stops mid-job every morning.")
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got.Status != intake.StatusIncomplete {
		t.Fatalf("Status = %q, want incomplete", got.Status)
	}
	if !reflect.DeepEqual(got.Missing, []string{"product"}) {
		t.Fatalf("Missing = %v, want [product]", got.Missing)
	}
	if got.Fields["problem_detail"] != "printer stops mid-job" {
		t.Fatalf("Fields = %v", got.Fields)
	}
	if got.Fields["contact"] != "a@b.com" {
		t.Fatalf("optional field dropped: %v", got.Fields)
	}
}

func TestExtractAllRequiredPresentIsReady(t *testing.T) {
	provider := &fakeProvider{json: "```json\n{\"problem_detail\":\"robot stuck at dock\",\"product\":\"Bella Bot\"}\n```"}
	ext := newExtractor(t, provider)

	got, err := ext.Extract(context.Background(), 7, "Robot stuck", "The robot will not leave the dock.")
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got.Status != intake.StatusReady {
		t.Fatalf("Status = %q, want ready", got.Status)
	}
	if len(got.Missing) != 0 {
		t.Fatalf("Missing = %v, want empty", got.Missing)
	}
}

func TestExtractPromptCarriesMessageAndKnownProducts(t *testing.T) {
	provider := &fakeProvider{json: `{"problem_detail":"x","product":"Bella Bot"}`}
	ext := newExtractor(t, provider, intake.WithProducts(fakeProducts{products: []string{"Bella Bot", "Kettybot"}}))

	if _, err := ext.Extract(context.Background(), 7, "Robot stuck", "will not leave the dock"); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if !strings.Contains(provider.prompt.User, "will not leave the dock") {
		t.Fatalf("prompt user missing body:\n%s", provider.prompt.User)
	}
	if !strings.Contains(provider.prompt.User, "Robot stuck") {
		t.Fatalf("prompt user missing subject:\n%s", provider.prompt.User)
	}
	if !strings.Contains(provider.prompt.System, "Bella Bot") {
		t.Fatalf("prompt system missing product hints:\n%s", provider.prompt.System)
	}
	if !strings.Contains(provider.prompt.System, "problem_detail") {
		t.Fatalf("prompt system missing field spec:\n%s", provider.prompt.System)
	}
}

func TestExtractProductOutsideTaxonomyIsTreatedAsMissing(t *testing.T) {
	provider := &fakeProvider{json: `{"problem_detail":"screen dead","product":"Toaster 9000"}`}
	ext := newExtractor(t, provider, intake.WithProducts(fakeProducts{products: []string{"Bella Bot"}}))

	got, err := ext.Extract(context.Background(), 7, "", "the screen is dead")
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got.Status != intake.StatusIncomplete {
		t.Fatalf("Status = %q, want incomplete", got.Status)
	}
	if !reflect.DeepEqual(got.Missing, []string{"product"}) {
		t.Fatalf("Missing = %v, want [product]", got.Missing)
	}
	if _, ok := got.Fields["product"]; ok {
		t.Fatalf("unknown product must not be kept: %v", got.Fields)
	}
}

func TestExtractUsesPerCompanySpec(t *testing.T) {
	provider := &fakeProvider{json: `{"problem_detail":"x","product":"Bella Bot","site":""}`}
	ext := newExtractor(t, provider, intake.WithSpecResolver(fakeSpecs{spec: intake.Spec{
		Required: []string{"problem_detail", "site"},
		Optional: []string{"contact"},
	}}))

	got, err := ext.Extract(context.Background(), 7, "", "something broke")
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if !reflect.DeepEqual(got.Missing, []string{"site"}) {
		t.Fatalf("Missing = %v, want [site]", got.Missing)
	}
	if _, ok := got.Fields["product"]; ok {
		t.Fatalf("field outside the company spec must be dropped: %v", got.Fields)
	}
}

func TestExtractEmptyMessageSkipsTheLLM(t *testing.T) {
	provider := &fakeProvider{json: `{"problem_detail":"never asked"}`}
	ext := newExtractor(t, provider)

	got, err := ext.Extract(context.Background(), 7, "  ", "\n")
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if provider.prompt.User != "" {
		t.Fatal("provider must not be called for an empty message")
	}
	if got.Status != intake.StatusIncomplete {
		t.Fatalf("Status = %q, want incomplete", got.Status)
	}
	if !reflect.DeepEqual(got.Missing, []string{"problem_detail", "product"}) {
		t.Fatalf("Missing = %v", got.Missing)
	}
}

func TestExtractUnparsableJSONYieldsIncompleteNotError(t *testing.T) {
	provider := &fakeProvider{json: "sorry, I cannot help with that"}
	ext := newExtractor(t, provider)

	got, err := ext.Extract(context.Background(), 7, "Subject", "body")
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got.Status != intake.StatusIncomplete {
		t.Fatalf("Status = %q, want incomplete", got.Status)
	}
	if len(got.Fields) != 0 {
		t.Fatalf("Fields = %v, want empty", got.Fields)
	}
}

func TestExtractProviderErrorPropagates(t *testing.T) {
	provider := &fakeProvider{err: errors.New("boom")}
	ext := newExtractor(t, provider)

	if _, err := ext.Extract(context.Background(), 7, "s", "b"); err == nil {
		t.Fatal("Extract() error = nil, want provider error")
	}
}
