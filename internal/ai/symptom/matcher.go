package symptom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/my/app/internal/domain/ports"
)

const defaultThreshold = 0.70

type Symptom struct {
	ID   int64
	Name string
}

type Catalog interface {
	Symptoms(ctx context.Context, companyID int64) ([]Symptom, error)
}

type ProviderForCompany func(ctx context.Context, companyID int64) (ports.LLMProvider, error)

type Match struct {
	SymptomID  int64
	Name       string
	Confidence float64
}

type Config struct {
	Catalog   Catalog
	Resolve   ProviderForCompany
	Threshold float64
}

type Matcher struct {
	catalog   Catalog
	resolve   ProviderForCompany
	threshold float64
}

func New(cfg Config) (*Matcher, error) {
	if cfg.Catalog == nil {
		return nil, errors.New("symptom: catalog is required")
	}
	if cfg.Resolve == nil {
		return nil, errors.New("symptom: resolver is required")
	}
	threshold := cfg.Threshold
	if threshold <= 0 {
		threshold = defaultThreshold
	}
	return &Matcher{catalog: cfg.Catalog, resolve: cfg.Resolve, threshold: threshold}, nil
}

func (m *Matcher) Match(ctx context.Context, companyID int64, message string) (Match, error) {
	message = strings.TrimSpace(message)
	if companyID <= 0 || message == "" {
		return Match{}, nil
	}

	symptoms, err := m.catalog.Symptoms(ctx, companyID)
	if err != nil {
		return Match{}, err
	}

	byID := make(map[int64]string, len(symptoms))
	var list strings.Builder
	for _, s := range symptoms {
		name := strings.TrimSpace(s.Name)
		if s.ID <= 0 || name == "" {
			continue
		}
		byID[s.ID] = name
		fmt.Fprintf(&list, "%d: %s\n", s.ID, name)
	}
	if len(byID) == 0 {
		return Match{}, nil
	}

	provider, err := m.resolve(ctx, companyID)
	if err != nil {
		return Match{}, err
	}

	prompt := ports.Prompt{
		System: "You match a customer support message to the single best-fitting known symptom. " +
			"Choose only from the provided list. Respond with JSON " +
			`{"symptom_id": <id or 0>, "confidence": <0..1>}. Use 0 when none fits.`,
		User: "Known symptoms (id: name):\n" + list.String() + "\nMessage:\n" + message,
	}
	resp, err := provider.GenerateJSON(ctx, prompt)
	if err != nil {
		return Match{}, fmt.Errorf("symptom match: %w", err)
	}

	var out struct {
		SymptomID  int64   `json:"symptom_id"`
		Confidence float64 `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(resp.Text), &out); err != nil {
		return Match{}, fmt.Errorf("symptom match parse: %w", err)
	}

	name, ok := byID[out.SymptomID]
	if !ok || out.SymptomID <= 0 || out.Confidence < m.threshold {
		return Match{}, nil
	}
	return Match{SymptomID: out.SymptomID, Name: name, Confidence: out.Confidence}, nil
}
