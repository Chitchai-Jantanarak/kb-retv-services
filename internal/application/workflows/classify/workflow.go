package classify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/domain/ports"
)

type ReportRecord struct {
	ID        int64
	CompanyID int64
	Title     string
	Body      string
}

type Classification struct {
	ReportID        int64
	CompanyID       int64
	ProblemTypeID   int64
	ProblemTypeConf float64
	SeverityID      int64
	SeverityConf    float64
	SubjectNodeID   int64
	SubjectConf     float64
	SymptomNodeID   int64
	SymptomConf     float64
	Model           string
}

type ReportSource interface {
	UnclassifiedReports(ctx context.Context, companyID int64, limit int) ([]ReportRecord, error)
}

type Lookup interface {
	ProblemTypeIDByCode(ctx context.Context, code string) (int64, error)
	SeverityIDByCode(ctx context.Context, code string) (int64, error)
	KnownSymptoms(ctx context.Context, companyID int64, limit int) ([]string, error)
	UpsertSubject(ctx context.Context, companyID int64, name string) (int64, error)
	UpsertSymptom(ctx context.Context, companyID int64, name string, isNew bool) (int64, error)
}

type Sink interface {
	WriteClassification(ctx context.Context, c Classification) error
}

type ProviderForCompany func(ctx context.Context, companyID int64) (ports.LLMProvider, error)

type Workflow struct {
	registry *prompts.Registry
	source   ReportSource
	lookup   Lookup
	sink     Sink
	resolve  ProviderForCompany
	review   ports.ReviewQueueWriter
	model    string
}

type Config struct {
	Registry *prompts.Registry
	Source   ReportSource
	Lookup   Lookup
	Sink     Sink
	Resolve  ProviderForCompany
	Review   ports.ReviewQueueWriter
	Model    string
}

func New(cfg Config) (*Workflow, error) {
	if cfg.Registry == nil {
		return nil, errors.New("classify: registry is required")
	}
	if cfg.Source == nil {
		return nil, errors.New("classify: source is required")
	}
	if cfg.Lookup == nil {
		return nil, errors.New("classify: lookup is required")
	}
	if cfg.Sink == nil {
		return nil, errors.New("classify: sink is required")
	}
	if cfg.Resolve == nil {
		return nil, errors.New("classify: resolver is required")
	}
	return &Workflow{
		registry: cfg.Registry,
		source:   cfg.Source,
		lookup:   cfg.Lookup,
		sink:     cfg.Sink,
		resolve:  cfg.Resolve,
		review:   cfg.Review,
		model:    strings.TrimSpace(cfg.Model),
	}, nil
}

type Options struct {
	CompanyID int64
	Limit     int
}

type Result struct {
	Examined   int
	Classified int
	Failed     int
}

func (w *Workflow) Run(ctx context.Context, opts Options) (Result, error) {
	if opts.CompanyID <= 0 {
		return Result{}, errors.New("classify: company_id must be positive")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 25
	}

	reports, err := w.source.UnclassifiedReports(ctx, opts.CompanyID, limit)
	if err != nil {
		return Result{}, fmt.Errorf("classify: list reports: %w", err)
	}

	provider, err := w.resolve(ctx, opts.CompanyID)
	if err != nil {
		return Result{}, fmt.Errorf("classify: resolve provider: %w", err)
	}

	knownSymptoms, err := w.lookup.KnownSymptoms(ctx, opts.CompanyID, 100)
	if err != nil {
		return Result{}, fmt.Errorf("classify: load symptoms: %w", err)
	}
	symptomOptions := strings.Join(knownSymptoms, ", ")
	severityOptions := "critical, high, medium, low"

	res := Result{Examined: len(reports)}
	for _, r := range reports {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if r.CompanyID != opts.CompanyID {
			return res, fmt.Errorf("classify: report %d belongs to company %d, not %d", r.ID, r.CompanyID, opts.CompanyID)
		}

		c, classifyErr := w.classifyOne(ctx, provider, r, severityOptions, symptomOptions)
		if classifyErr != nil {
			log.Printf("classify: report %d company %d failed: %v", r.ID, r.CompanyID, classifyErr)
			res.Failed++
			continue
		}
		if err := w.sink.WriteClassification(ctx, c); err != nil {
			log.Printf("classify: report %d company %d write failed: %v", r.ID, r.CompanyID, err)
			res.Failed++
			continue
		}
		res.Classified++
	}
	return res, nil
}

type stage1Result struct {
	Subject     string  `json:"subject"`
	ProblemType string  `json:"problem_type"`
	Confidence  float64 `json:"confidence"`
	Rationale   string  `json:"rationale"`
}

type stage2Result struct {
	Severity     string  `json:"severity"`
	Symptom      string  `json:"symptom"`
	SymptomIsNew bool    `json:"symptom_is_new"`
	Confidence   float64 `json:"confidence"`
	Rationale    string  `json:"rationale"`
}

const (
	classifyMaxRetries = 2
	classifyRetryBase  = 800 * time.Millisecond
	classifyRetryMax   = 8 * time.Second
)

const maxReportRunes = 20000

var dataURIRe = regexp.MustCompile(`data:[a-zA-Z0-9/+.-]+;base64,[A-Za-z0-9+/=]+`)

func promptText(title, body string) string {
	text := strings.TrimSpace(title + "\n\n" + dataURIRe.ReplaceAllString(body, "[image]"))
	if runes := []rune(text); len(runes) > maxReportRunes {
		return string(runes[:maxReportRunes])
	}
	return text
}

func (w *Workflow) classifyOne(ctx context.Context, provider ports.LLMProvider, r ReportRecord, severityOptions, symptomOptions string) (Classification, error) {
	reportText := promptText(r.Title, r.Body)
	if reportText == "" {
		return Classification{}, errors.New("empty report body")
	}

	stage1, stage2, err := w.runStages(ctx, provider, reportText, severityOptions, symptomOptions)
	if err != nil {
		return Classification{}, err
	}

	problemTypeID, err := w.lookup.ProblemTypeIDByCode(ctx, normalizeCode(stage1.ProblemType))
	if err != nil {
		return Classification{}, fmt.Errorf("problem_type lookup: %w", err)
	}
	severityID, err := w.lookup.SeverityIDByCode(ctx, normalizeCode(stage2.Severity))
	if err != nil {
		return Classification{}, fmt.Errorf("severity lookup: %w", err)
	}

	subjectName := strings.TrimSpace(stage1.Subject)
	subjectID, err := w.lookup.UpsertSubject(ctx, r.CompanyID, subjectName)
	if err != nil {
		return Classification{}, fmt.Errorf("subject upsert: %w", err)
	}
	symptomName := strings.TrimSpace(stage2.Symptom)
	symptomID, err := w.lookup.UpsertSymptom(ctx, r.CompanyID, symptomName, stage2.SymptomIsNew)
	if err != nil {
		return Classification{}, fmt.Errorf("symptom upsert: %w", err)
	}

	if w.review != nil && stage2.SymptomIsNew && symptomName != "" {
		if err := w.review.Enqueue(ctx, ports.ReviewItem{
			CompanyID: r.CompanyID,
			Kind:      ports.ReviewKindSymptomPropose,
			Payload: map[string]any{
				"symptom_node_id": symptomID,
				"name":            symptomName,
				"source_report":   r.ID,
				"confidence":      stage2.Confidence,
			},
		}); err != nil {
			log.Printf("classify: report %d company %d symptom review enqueue failed: %v", r.ID, r.CompanyID, err)
		}
	}

	return Classification{
		ReportID:        r.ID,
		CompanyID:       r.CompanyID,
		ProblemTypeID:   problemTypeID,
		ProblemTypeConf: stage1.Confidence,
		SeverityID:      severityID,
		SeverityConf:    stage2.Confidence,
		SubjectNodeID:   subjectID,
		SubjectConf:     stage1.Confidence,
		SymptomNodeID:   symptomID,
		SymptomConf:     stage2.Confidence,
		Model:           w.model,
	}, nil
}

func (w *Workflow) runStages(ctx context.Context, provider ports.LLMProvider, reportText, severityOptions, symptomOptions string) (stage1Result, stage2Result, error) {
	var s1 stage1Result
	var s2 stage2Result
	var lastErr error

	for attempt := 0; attempt <= classifyMaxRetries; attempt++ {
		if attempt > 0 {
			wait := min(classifyRetryBase*time.Duration(1<<(attempt-1)), classifyRetryMax)
			select {
			case <-ctx.Done():
				return s1, s2, ctx.Err()
			case <-time.After(wait):
			}
			log.Printf("classify: stages retry attempt %d after: %v", attempt, lastErr)
		}

		var err error
		s1, s2, err = w.callStages(ctx, provider, reportText, severityOptions, symptomOptions)
		if err == nil {
			return s1, s2, nil
		}
		lastErr = err
	}
	return s1, s2, lastErr
}

func (w *Workflow) callStages(ctx context.Context, provider ports.LLMProvider, reportText, severityOptions, symptomOptions string) (stage1Result, stage2Result, error) {
	var s1 stage1Result
	var s2 stage2Result

	stage1Tmpl, err := w.registry.Get(prompts.NameClassifyStage1)
	if err != nil {
		return s1, s2, err
	}
	stage2Tmpl, err := w.registry.Get(prompts.NameClassifyStage2)
	if err != nil {
		return s1, s2, err
	}

	stage1Rendered, err := stage1Tmpl.Render(map[string]string{"report": reportText})
	if err != nil {
		return s1, s2, err
	}
	stage1Resp, err := provider.GenerateJSON(ctx, stage1Rendered)
	if err != nil {
		return s1, s2, fmt.Errorf("stage1 call: %w", err)
	}
	if err := json.Unmarshal([]byte(stage1Resp.Text), &s1); err != nil {
		return s1, s2, fmt.Errorf("stage1 parse: %w", err)
	}

	stage2Vars := map[string]string{
		"report":           reportText,
		"severity_options": severityOptions,
	}
	if symptomOptions != "" {
		stage2Vars["symptom_options"] = symptomOptions
	} else {
		stage2Vars["symptom_options"] = "(none yet)"
	}
	stage2Rendered, err := stage2Tmpl.Render(stage2Vars)
	if err != nil {
		return s1, s2, err
	}
	stage2Resp, err := provider.GenerateJSON(ctx, stage2Rendered)
	if err != nil {
		return s1, s2, fmt.Errorf("stage2 call: %w", err)
	}
	if err := json.Unmarshal([]byte(stage2Resp.Text), &s2); err != nil {
		return s1, s2, fmt.Errorf("stage2 parse: %w", err)
	}

	return s1, s2, nil
}

func normalizeCode(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
