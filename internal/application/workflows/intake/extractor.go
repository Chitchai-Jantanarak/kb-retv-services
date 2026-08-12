package intake

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/domain/ports"
)

var intakeMailDelimiter = regexp.MustCompile(`(?i)\[\s*(?:begin|end)\s+mail\b[^\]]*\]`)

const (
	maxMessageChars = 8000
	maxProductHints = 30
)

const FieldClassification = "classification"
const FieldCatalogRelated = "catalog_related"

var skipModelBelowScore = 20

const (
	ClassificationNewIssue      = "new_issue"
	ClassificationFollowUp      = "follow_up"
	ClassificationStatusQuery   = "status_query"
	ClassificationNotActionable = "not_actionable"
	ClassificationUnclear       = "unclear"
)

var validClassifications = map[string]struct{}{
	ClassificationNewIssue:      {},
	ClassificationFollowUp:      {},
	ClassificationStatusQuery:   {},
	ClassificationNotActionable: {},
	ClassificationUnclear:       {},
}

type Result struct {
	Fields         map[string]string
	Missing        []string
	Status         string
	Score          int
	Reasons        []string
	Classification string
	CatalogRelated *bool
}

type ProviderForCompany func(ctx context.Context, companyID int64) (ports.LLMProvider, error)

type ProductSource interface {
	Products(ctx context.Context, companyID int64) ([]string, error)
}

type Extractor struct {
	tmpl     prompts.Template
	resolve  ProviderForCompany
	specs    SpecResolver
	products ProductSource
}

type Option func(*Extractor)

func WithSpecResolver(specs SpecResolver) Option {
	return func(e *Extractor) {
		if specs != nil {
			e.specs = specs
		}
	}
}

func WithProducts(products ProductSource) Option {
	return func(e *Extractor) {
		if products != nil {
			e.products = products
		}
	}
}

func NewExtractor(registry *prompts.Registry, resolve ProviderForCompany, opts ...Option) (*Extractor, error) {
	if registry == nil {
		return nil, fmt.Errorf("intake: prompt registry is required")
	}
	if resolve == nil {
		return nil, fmt.Errorf("intake: provider resolver is required")
	}
	tmpl, err := registry.Get(prompts.NameIntakeExtract)
	if err != nil {
		return nil, fmt.Errorf("intake: %w", err)
	}
	e := &Extractor{tmpl: tmpl, resolve: resolve}
	for _, opt := range opts {
		opt(e)
	}
	return e, nil
}

func (e *Extractor) Extract(ctx context.Context, companyID int64, sig Signals) (Result, error) {
	if companyID <= 0 {
		return Result{}, fmt.Errorf("intake: company_id must be positive")
	}

	score, reasons := Score(sig)
	if score <= skipModelBelowScore {
		return Result{
			Status:         StatusUnknown,
			Score:          score,
			Reasons:        reasons,
			Classification: ClassificationNotActionable,
		}, nil
	}

	spec := DefaultSpec()
	if e.specs != nil {
		resolved, err := e.specs.SpecFor(ctx, companyID)
		if err != nil {
			return Result{}, fmt.Errorf("intake: resolve spec: %w", err)
		}
		spec = resolved.Normalized()
	}

	message := composeMessage(sig.Subject, sig.Body)
	if message == "" {
		return emptyResult(spec), nil
	}

	provider, err := e.resolve(ctx, companyID)
	if errors.Is(err, ports.ErrAIDisabled) {
		return Result{Status: StatusUnknown}, nil
	}
	if err != nil {
		return Result{}, fmt.Errorf("intake: resolve provider: %w", err)
	}

	products, err := e.productList(ctx, companyID)
	if err != nil {
		return Result{}, err
	}

	prompt, err := e.tmpl.Render(map[string]string{
		"fields":   fieldsSection(spec),
		"products": productsSection(products),
		"message":  message,
	})
	if err != nil {
		return Result{}, fmt.Errorf("intake: render prompt: %w", err)
	}
	prompt.Images = sig.Images

	completion, err := provider.GenerateJSON(ctx, prompt)
	if err != nil {
		return Result{}, err
	}

	decoded := decodeJSONObject(completion.Text)
	fields := parseFields(decoded, spec)
	classification := parseClassification(decoded)
	catalogRelated := parseCatalogRelated(decoded)
	if !catalogRelated {
		classification = ClassificationNotActionable
	}
	var catalogRelatedPtr *bool
	if len(sig.Images) > 0 {
		catalogRelatedPtr = &catalogRelated
	}

	productChecked := len(products) > 0
	if productChecked && unknownProduct(fields[FieldProduct], products) {
		delete(fields, FieldProduct)
	}

	missing := spec.Missing(fields)
	status := spec.StatusFor(fields)
	if status == StatusReady && !productChecked {
		status = StatusUnverified
	}

	return Result{
		Fields:         fields,
		Missing:        missing,
		Status:         status,
		Score:          score,
		Reasons:        reasons,
		Classification: classification,
		CatalogRelated: catalogRelatedPtr,
	}, nil
}

func (e *Extractor) productList(ctx context.Context, companyID int64) ([]string, error) {
	if e.products == nil {
		return nil, nil
	}
	products, err := e.products.Products(ctx, companyID)
	if err != nil {
		return nil, fmt.Errorf("intake: load products: %w", err)
	}
	if len(products) > maxProductHints {
		products = products[:maxProductHints]
	}
	return products, nil
}

func emptyResult(spec Spec) Result {
	fields := map[string]string{}
	return Result{
		Fields:  fields,
		Missing: spec.Missing(fields),
		Status:  spec.StatusFor(fields),
	}
}

func composeMessage(subject, body string) string {
	subject = intakeMailDelimiter.ReplaceAllString(strings.TrimSpace(subject), " ")
	body = intakeMailDelimiter.ReplaceAllString(strings.TrimSpace(body), " ")
	var b strings.Builder
	if subject != "" {
		fmt.Fprintf(&b, "Subject: %s\n\n", subject)
	}
	b.WriteString(body)
	message := strings.TrimSpace(b.String())
	if len(message) > maxMessageChars {
		message = message[:maxMessageChars]
	}
	return message
}

func fieldsSection(spec Spec) string {
	normalized := spec.Normalized()
	var b strings.Builder
	fmt.Fprintf(&b, "required: %s", strings.Join(normalized.Required, ", "))
	if len(normalized.Optional) > 0 {
		fmt.Fprintf(&b, "; optional: %s", strings.Join(normalized.Optional, ", "))
	}
	return b.String()
}

func productsSection(products []string) string {
	if len(products) == 0 {
		return ""
	}
	return "\nKnown products for this company: " + strings.Join(products, ", ") +
		". Use one of them verbatim when the message names a product; leave product empty when none of them matches."
}

func decodeJSONObject(raw string) map[string]any {
	txt := strings.TrimSpace(raw)
	txt = strings.TrimPrefix(txt, "```json")
	txt = strings.TrimPrefix(txt, "```")
	txt = strings.TrimSuffix(txt, "```")
	txt = strings.TrimSpace(txt)

	decoded := map[string]any{}
	if err := json.Unmarshal([]byte(txt), &decoded); err == nil {
		return decoded
	}

	start, end := strings.Index(txt, "{"), strings.LastIndex(txt, "}")
	if start < 0 || end <= start {
		return map[string]any{}
	}
	if err := json.Unmarshal([]byte(txt[start:end+1]), &decoded); err != nil {
		return map[string]any{}
	}
	return decoded
}

func parseFields(decoded map[string]any, spec Spec) map[string]string {
	fields := make(map[string]string, len(decoded))
	for _, key := range spec.Fields() {
		value, ok := decoded[key]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		fields[key] = text
	}
	return fields
}

func parseClassification(decoded map[string]any) string {
	value, ok := decoded[FieldClassification]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	text = strings.ToLower(strings.TrimSpace(text))
	if _, valid := validClassifications[text]; !valid {
		return ""
	}
	return text
}

func parseCatalogRelated(decoded map[string]any) bool {
	value, ok := decoded[FieldCatalogRelated]
	if !ok {
		return true
	}
	related, ok := value.(bool)
	if !ok {
		return true
	}
	return related
}

func unknownProduct(product string, products []string) bool {
	product = strings.TrimSpace(product)
	if product == "" || len(products) == 0 {
		return false
	}
	for _, known := range products {
		if strings.EqualFold(strings.TrimSpace(known), product) {
			return false
		}
	}
	return true
}
