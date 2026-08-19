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
	calls  int
}

func (f *fakeProvider) Generate(context.Context, ports.Prompt) (ports.Completion, error) {
	return ports.Completion{}, errors.New("not used")
}

func (f *fakeProvider) GenerateJSON(_ context.Context, p ports.Prompt) (ports.Completion, error) {
	f.calls++
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

func TestExtractDisabledAIReturnsUnknownWithoutVendorCall(t *testing.T) {
	registry, err := prompts.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	provider := &fakeProvider{}
	resolve := func(context.Context, int64) (ports.LLMProvider, error) {
		return nil, ports.ErrAIDisabled
	}
	ext, err := intake.NewExtractor(registry, resolve)
	if err != nil {
		t.Fatalf("NewExtractor() error = %v", err)
	}

	got, err := ext.Extract(context.Background(), 7, intake.Signals{Sender: "cust@x.com", Subject: "Printer down", Body: "It stops mid-job every morning."})
	if err != nil {
		t.Fatalf("Extract() error = %v, want nil", err)
	}
	if got.Status != intake.StatusUnknown {
		t.Fatalf("Status = %q, want unknown", got.Status)
	}
	if provider.calls != 0 {
		t.Fatal("provider was called while ai is disabled")
	}
}

func TestExtractMissingProductIsIncomplete(t *testing.T) {
	provider := &fakeProvider{json: `{"problem_detail":"printer stops mid-job","product":"","contact":"a@b.com"}`}
	ext := newExtractor(t, provider)

	got, err := ext.Extract(context.Background(), 7, intake.Signals{Sender: "cust@x.com", Subject: "Printer down", Body: "It stops mid-job every morning."})
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
	ext := newExtractor(t, provider, intake.WithProducts(fakeProducts{products: []string{"Bella Bot"}}))

	got, err := ext.Extract(context.Background(), 7, intake.Signals{Sender: "cust@x.com", Subject: "Robot stuck", Body: "The robot will not leave the dock."})
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

	if _, err := ext.Extract(context.Background(), 7, intake.Signals{Sender: "cust@x.com", Subject: "Robot stuck", Body: "will not leave the dock"}); err != nil {
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

	got, err := ext.Extract(context.Background(), 7, intake.Signals{Sender: "cust@x.com", Subject: "", Body: "the screen is dead"})
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

	got, err := ext.Extract(context.Background(), 7, intake.Signals{Sender: "cust@x.com", Subject: "", Body: "something broke"})
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

	got, err := ext.Extract(context.Background(), 7, intake.Signals{Sender: "cust@x.com", Subject: "  ", Body: "\n"})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if provider.calls != 0 {
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

	got, err := ext.Extract(context.Background(), 7, intake.Signals{Sender: "cust@x.com", Subject: "Subject", Body: "body"})
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

	if _, err := ext.Extract(context.Background(), 7, intake.Signals{Sender: "cust@x.com", Subject: "s", Body: "b"}); err == nil {
		t.Fatal("Extract() error = nil, want provider error")
	}
}

func TestExtractWithoutProductTaxonomyIsUnverified(t *testing.T) {
	provider := &fakeProvider{json: `{"problem_detail":"repo has a long function","product":"some/repo"}`}
	ext := newExtractor(t, provider)

	got, err := ext.Extract(context.Background(), 7, intake.Signals{Sender: "no-reply@example.com", Subject: "Suggestions", Body: "Jules found improvements for your repos and listed them below in detail for review."})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got.Status != intake.StatusUnverified {
		t.Fatalf("Status = %q, want unverified", got.Status)
	}
	if got.Score <= 0 || got.Score >= 100 {
		t.Fatalf("Score = %d, want strictly between 0 and 100", got.Score)
	}
	if !containsReason(got.Reasons, intake.ReasonSenderNoReply) {
		t.Fatalf("Reasons = %v", got.Reasons)
	}
}

func TestExtractParsesClassificationAndReasoningFromModelOutput(t *testing.T) {
	provider := &fakeProvider{json: `{"classification":"new_issue","reasoning":"The message reports that the robot cannot leave the dock.","problem_detail":"robot stuck at dock","product":"Bella Bot"}`}
	ext := newExtractor(t, provider, intake.WithProducts(fakeProducts{products: []string{"Bella Bot"}}))

	got, err := ext.Extract(context.Background(), 7, intake.Signals{Sender: "cust@x.com", Subject: "Robot stuck", Body: "The robot will not leave the dock."})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got.Classification != intake.ClassificationNewIssue {
		t.Fatalf("Classification = %q, want %q", got.Classification, intake.ClassificationNewIssue)
	}
	if got.Reasoning != "The message reports that the robot cannot leave the dock." {
		t.Fatalf("Reasoning = %q", got.Reasoning)
	}
}

func TestExtractDropsUnrecognizedClassification(t *testing.T) {
	provider := &fakeProvider{json: `{"classification":"spam","problem_detail":"x"}`}
	ext := newExtractor(t, provider)

	got, err := ext.Extract(context.Background(), 7, intake.Signals{Sender: "cust@x.com", Subject: "s", Body: "b"})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got.Classification != "" {
		t.Fatalf("Classification = %q, want empty for an unrecognized value", got.Classification)
	}
}

func TestExtractSkipsProviderForBulkMail(t *testing.T) {
	provider := &fakeProvider{json: `{"problem_detail":"never asked"}`}
	ext := newExtractor(t, provider)

	got, err := ext.Extract(context.Background(), 7, intake.Signals{
		Sender:          "no-reply@newsletter.com",
		Subject:         "You might like these products",
		Body:            "Click here to unsubscribe from our mailing list at any time.",
		ListUnsubscribe: true,
		AutoSubmitted:   "auto-generated",
		Precedence:      "bulk",
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0 for obvious bulk mail", provider.calls)
	}
	if got.Classification != intake.ClassificationNotActionable {
		t.Fatalf("Classification = %q, want %q", got.Classification, intake.ClassificationNotActionable)
	}
	if got.Status != intake.StatusUnknown {
		t.Fatalf("Status = %q, want unknown", got.Status)
	}
	if got.Reasoning != "" {
		t.Fatalf("Reasoning = %q, want empty when semantic analysis is skipped", got.Reasoning)
	}
}

func TestExtractSkipModelPathStillCarriesReferencedCase(t *testing.T) {
	provider := &fakeProvider{json: `{"problem_detail":"never asked"}`}
	ext := newExtractor(t, provider)

	got, err := ext.Extract(context.Background(), 7, intake.Signals{
		Sender:          "no-reply@newsletter.com",
		Subject:         "You might like these products",
		Body:            "Click here to unsubscribe from our mailing list at any time.",
		ListUnsubscribe: true,
		AutoSubmitted:   "auto-generated",
		Precedence:      "bulk",
		ReferencedCase:  "REP-4106",
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0 for obvious bulk mail even when it cites a case", provider.calls)
	}
	if got.ReferencedCase != "REP-4106" {
		t.Fatalf("ReferencedCase = %q, want REP-4106", got.ReferencedCase)
	}
}

func TestExtractPlainShortCustomerMailCallsProvider(t *testing.T) {
	provider := &fakeProvider{json: `{"problem_detail":"stuck","product":"Bella Bot"}`}
	ext := newExtractor(t, provider, intake.WithProducts(fakeProducts{products: []string{"Bella Bot"}}))

	if _, err := ext.Extract(context.Background(), 7, intake.Signals{
		Sender:  "somchai@customer.co.th",
		Subject: "Robot stuck",
		Body:    "The robot will not leave the dock.",
	}); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1 for a plain customer mail", provider.calls)
	}
}

func TestExtractReferencedCaseStillCallsProviderDespiteThinBody(t *testing.T) {
	provider := &fakeProvider{json: `{"problem_detail":"ok"}`}
	ext := newExtractor(t, provider)

	got, err := ext.Extract(context.Background(), 7, intake.Signals{
		Sender:         "somchai@customer.co.th",
		Subject:        "Re: REP-4106",
		Body:           "ok",
		ReferencedCase: "REP-4106",
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1 when a case is referenced even with a thin body", provider.calls)
	}
	if got.ReferencedCase != "REP-4106" {
		t.Fatalf("ReferencedCase = %q, want REP-4106", got.ReferencedCase)
	}
}

func TestExtractPromptCarriesImages(t *testing.T) {
	provider := &fakeProvider{json: `{"problem_detail":"screen cracked"}`}
	ext := newExtractor(t, provider)

	images := []ports.PromptImage{
		{MIMEType: "image/jpeg", Data: []byte("first")},
		{MIMEType: "image/png", Data: []byte("second")},
	}
	if _, err := ext.Extract(context.Background(), 7, intake.Signals{Sender: "cust@x.com", Subject: "Screen broken", Body: "see attached", Images: images}); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(provider.prompt.Images) != 2 {
		t.Fatalf("prompt Images = %v, want 2 entries", provider.prompt.Images)
	}
}

func TestExtractCatalogRelatedFalseForcesNotActionable(t *testing.T) {
	provider := &fakeProvider{json: `{"classification":"new_issue","problem_detail":"a cat in the photo","catalog_related":false}`}
	ext := newExtractor(t, provider)

	got, err := ext.Extract(context.Background(), 7, intake.Signals{Sender: "cust@x.com", Subject: "Photo", Body: "see attached photo"})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got.Classification != intake.ClassificationNotActionable {
		t.Fatalf("Classification = %q, want %q", got.Classification, intake.ClassificationNotActionable)
	}
}

func TestExtractCatalogRelatedTrueKeepsModelClassification(t *testing.T) {
	provider := &fakeProvider{json: `{"classification":"new_issue","problem_detail":"robot stuck at dock","catalog_related":true}`}
	ext := newExtractor(t, provider, intake.WithProducts(fakeProducts{products: []string{"Bella Bot"}}))

	got, err := ext.Extract(context.Background(), 7, intake.Signals{Sender: "cust@x.com", Subject: "Robot stuck", Body: "The robot will not leave the dock."})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got.Classification != intake.ClassificationNewIssue {
		t.Fatalf("Classification = %q, want %q", got.Classification, intake.ClassificationNewIssue)
	}
}

func TestExtractCatalogRelatedMissingKeepsModelClassification(t *testing.T) {
	provider := &fakeProvider{json: `{"classification":"new_issue","problem_detail":"robot stuck at dock"}`}
	ext := newExtractor(t, provider, intake.WithProducts(fakeProducts{products: []string{"Bella Bot"}}))

	got, err := ext.Extract(context.Background(), 7, intake.Signals{Sender: "cust@x.com", Subject: "Robot stuck", Body: "The robot will not leave the dock."})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got.Classification != intake.ClassificationNewIssue {
		t.Fatalf("Classification = %q, want %q (fail open on missing catalog_related)", got.Classification, intake.ClassificationNewIssue)
	}
}

func TestExtractSkipModelPathLeavesCatalogRelatedNil(t *testing.T) {
	provider := &fakeProvider{json: `{"problem_detail":"never asked"}`}
	ext := newExtractor(t, provider)

	got, err := ext.Extract(context.Background(), 7, intake.Signals{
		Sender:          "no-reply@newsletter.com",
		Subject:         "You might like these products",
		Body:            "Click here to unsubscribe from our mailing list at any time.",
		ListUnsubscribe: true,
		AutoSubmitted:   "auto-generated",
		Precedence:      "bulk",
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got.Classification != intake.ClassificationNotActionable {
		t.Fatalf("Classification = %q, want %q", got.Classification, intake.ClassificationNotActionable)
	}
	if got.CatalogRelated != nil {
		t.Fatalf("CatalogRelated = %v, want nil on the skip-model path", got.CatalogRelated)
	}
}

func TestExtractModelFalseCatalogRelatedYieldsNonNilFalse(t *testing.T) {
	provider := &fakeProvider{json: `{"classification":"new_issue","problem_detail":"a cat in the photo","catalog_related":false}`}
	ext := newExtractor(t, provider)

	images := []ports.PromptImage{{MIMEType: "image/jpeg", Data: []byte("photo")}}
	got, err := ext.Extract(context.Background(), 7, intake.Signals{Sender: "cust@x.com", Subject: "Photo", Body: "see attached photo", Images: images})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got.CatalogRelated == nil {
		t.Fatal("CatalogRelated = nil, want non-nil")
	}
	if *got.CatalogRelated != false {
		t.Fatalf("CatalogRelated = %v, want false", *got.CatalogRelated)
	}
}

func TestExtractModelOmittingCatalogRelatedYieldsNonNilTrue(t *testing.T) {
	provider := &fakeProvider{json: `{"classification":"new_issue","problem_detail":"robot stuck at dock"}`}
	ext := newExtractor(t, provider, intake.WithProducts(fakeProducts{products: []string{"Bella Bot"}}))

	images := []ports.PromptImage{{MIMEType: "image/jpeg", Data: []byte("photo")}}
	got, err := ext.Extract(context.Background(), 7, intake.Signals{Sender: "cust@x.com", Subject: "Robot stuck", Body: "The robot will not leave the dock.", Images: images})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got.CatalogRelated == nil {
		t.Fatal("CatalogRelated = nil, want non-nil (fail-open still records what happened)")
	}
	if *got.CatalogRelated != true {
		t.Fatalf("CatalogRelated = %v, want true", *got.CatalogRelated)
	}
}

func containsReason(reasons []string, want string) bool {
	for _, r := range reasons {
		if r == want {
			return true
		}
	}
	return false
}
