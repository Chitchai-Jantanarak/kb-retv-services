package llm

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
)

type RouteCandidate struct {
	ConnectionID int64  `json:"connection_id"`
	Model        string `json:"model"`
}

type RouteConfig struct {
	Candidates             []RouteCandidate `json:"candidates"`
	AllowModelSubstitution bool             `json:"allow_model_substitution"`
	TimeoutMS              int              `json:"timeout_ms"`
	AttemptTimeoutMS       int              `json:"attempt_timeout_ms"`
	MaxAttempts            int              `json:"max_attempts"`
	FallbackOn             []string         `json:"fallback_on"`
	Version                int              `json:"-"`
}

type ProviderModel struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Capabilities map[string]bool `json:"capabilities"`
}

type ProviderConnection struct {
	ID                int64
	Provider          string
	APIKey            string `json:"-"`
	CredentialVersion int
	Active            bool
	Models            []ProviderModel
	RefreshedAt       *time.Time
	ModelsError       string
}

type RouteLookup interface {
	RouteFor(context.Context, int64, string) (RouteConfig, bool, error)
	ConnectionFor(context.Context, int64, int64) (ProviderConnection, error)
}

type RouteAttempt struct {
	CompanyID, ConnectionID int64
	RequestID, Task         string
	Provider, Model         string
	Operation, Status       string
	Version, Attempt        int
	LatencyMS               int
	Usage                   ports.TokenUsage
}

type RouteAttemptSink interface {
	RecordAttempt(context.Context, RouteAttempt) error
}

type modelSelectionKey struct{}

type ModelSelection struct {
	Model        string
	ConnectionID int64
}

func WithModelSelection(ctx context.Context, selection ModelSelection) context.Context {
	return context.WithValue(ctx, modelSelectionKey{}, selection)
}

func (r *CompanyResolver) WithRoutes(lookup RouteLookup, sink RouteAttemptSink) *CompanyResolver {
	r.routes = lookup
	r.attemptSink = sink
	return r
}

func (r *CompanyResolver) ForTask(task string) func(context.Context, int64) (ports.LLMProvider, error) {
	return func(ctx context.Context, companyID int64) (ports.LLMProvider, error) {
		return r.ResolveTask(ctx, companyID, task)
	}
}

func (r *CompanyResolver) LegacyCacheAllowed(ctx context.Context, companyID int64, task string) bool {
	if r.routes == nil {
		return true
	}
	_, found, err := r.routes.RouteFor(ctx, companyID, task)
	return err == nil && !found
}

func (r *CompanyResolver) ResolveTask(ctx context.Context, companyID int64, task string) (ports.LLMProvider, error) {
	if cid, ok := ctxkey.CompanyID(ctx); !ok || cid != companyID || cid <= 0 {
		return nil, apperr.New(apperr.CodeForbidden, "company context does not match")
	}
	selection, _ := ctx.Value(modelSelectionKey{}).(ModelSelection)
	if r.routes == nil {
		return r.resolveLegacySelection(ctx, companyID, selection)
	}
	config, found, err := r.routes.RouteFor(ctx, companyID, task)
	if err != nil {
		return nil, apperr.New(apperr.CodeUnavailable, "AI routing configuration unavailable")
	}
	if !found {
		return r.resolveLegacySelection(ctx, companyID, selection)
	}
	if _, err := r.lookupAgent(ctx, companyID); err != nil {
		return nil, err
	}
	if !config.valid() {
		return nil, apperr.New(apperr.CodeUnavailable, "invalid AI routing policy")
	}
	candidates, err := r.selectCandidates(ctx, companyID, config, selection)
	if err != nil {
		return nil, err
	}
	p := &routedProvider{config: config, companyID: companyID, task: task, sink: r.attemptSink}
	for _, candidate := range candidates {
		p.candidates = append(p.candidates, routedCandidate{
			connectionID: candidate.ConnectionID, vendor: "routing", model: candidate.Model,
			load: func(ctx context.Context) (ports.LLMProvider, string, error) {
				return r.loadCandidate(ctx, companyID, candidate)
			},
		})
	}
	var provider ports.LLMProvider = p
	if r.sink != nil {
		provider = NewRecording(provider, "routing", candidates[0].Model, r.sink)
	}
	return provider, nil
}

func (c RouteConfig) valid() bool {
	return c.TimeoutMS >= 1000 && c.TimeoutMS <= 120000 && c.AttemptTimeoutMS >= 1000 &&
		c.AttemptTimeoutMS <= c.TimeoutMS && c.MaxAttempts >= 1 && c.MaxAttempts <= 10 &&
		len(c.Candidates) > 0 && len(c.Candidates) <= 10
}

func (r *CompanyResolver) selectCandidates(ctx context.Context, companyID int64, config RouteConfig, selection ModelSelection) ([]RouteCandidate, error) {
	candidates := slices.Clone(config.Candidates)
	if selection.Model == "" && selection.ConnectionID == 0 {
		return candidates, nil
	}
	for index, candidate := range candidates {
		if selection.ConnectionID != 0 && candidate.ConnectionID != selection.ConnectionID {
			continue
		}
		connection, err := r.routes.ConnectionFor(ctx, companyID, candidate.ConnectionID)
		if err != nil {
			return nil, apperr.New(apperr.CodeUnavailable, "AI connection unavailable")
		}
		if selection.Model != "" {
			candidate.Model = selection.Model
		}
		if !connection.Active || !slices.ContainsFunc(connection.Models, func(m ProviderModel) bool { return m.ID == candidate.Model && generationModel(m) }) {
			continue
		}
		if !config.AllowModelSubstitution {
			return []RouteCandidate{candidate}, nil
		}
		rest := append(slices.Clone(candidates[:index]), candidates[index+1:]...)
		return append([]RouteCandidate{candidate}, rest...), nil
	}
	return nil, apperr.New(apperr.CodeInvalidInput, "selected model is not allowed by the task route")
}

func (r *CompanyResolver) loadCandidate(ctx context.Context, companyID int64, candidate RouteCandidate) (ports.LLMProvider, string, error) {
	connection, err := r.routes.ConnectionFor(ctx, companyID, candidate.ConnectionID)
	if err != nil {
		return nil, "routing", apperr.New(apperr.CodeUnavailable, "AI connection unavailable")
	}
	if !connection.Active {
		return nil, connection.Provider, errRouteConnectionDisabled
	}
	index := slices.IndexFunc(connection.Models, func(m ProviderModel) bool { return m.ID == candidate.Model })
	if index < 0 {
		return nil, connection.Provider, apperr.New(apperr.CodeUnavailable, "model catalog requires refresh or route update")
	}
	if supported, known := connection.Models[index].Capabilities["generate"]; known && !supported {
		return nil, connection.Provider, apperr.New(apperr.CodeInvalidInput, "selected model does not support generation")
	}
	provider, err := r.buildConnection(connection, candidate.Model)
	if err != nil {
		return nil, connection.Provider, apperr.New(apperr.CodeUnavailable, "AI connection is not configured")
	}
	return provider, connection.Provider, nil
}

func (r *CompanyResolver) resolveLegacySelection(ctx context.Context, companyID int64, selection ModelSelection) (ports.LLMProvider, error) {
	if selection.Model != "" || selection.ConnectionID != 0 {
		return nil, apperr.New(apperr.CodeInvalidInput, "model selection requires an active task route")
	}
	return r.ResolveFor(ctx, companyID)
}

func (r *CompanyResolver) buildConnection(connection ProviderConnection, model string) (ports.LLMProvider, error) {
	if connection.APIKey == "" && connection.Provider != VendorLocal {
		return nil, errors.New("connection credential missing")
	}
	// NOTE: Explicit connections must never inherit a platform credential.
	settings := Settings{Vendor: connection.Provider, Model: model, LocalURL: r.defaults.LocalURL}
	applyVendorKey(&settings, connection.Provider, connection.APIKey)
	return Resolve(settings)
}
