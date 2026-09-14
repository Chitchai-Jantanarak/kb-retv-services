package llm

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/llm/llmerr"
	"github.com/my/app/internal/shared/ctxkey"
)

type routeTestProvider struct {
	err    error
	calls  int
	stream <-chan ports.Completion
	wait   bool
}

func (p *routeTestProvider) Generate(ctx context.Context, _ ports.Prompt) (ports.Completion, error) {
	p.calls++
	if p.wait {
		<-ctx.Done()
		return ports.Completion{}, ctx.Err()
	}
	return ports.Completion{Text: "answer"}, p.err
}

func (p *routeTestProvider) GenerateJSON(ctx context.Context, prompt ports.Prompt) (ports.Completion, error) {
	return p.Generate(ctx, prompt)
}

func (p *routeTestProvider) Stream(context.Context, ports.Prompt) (<-chan ports.Completion, error) {
	p.calls++
	return p.stream, p.err
}

type routeTestSink struct {
	mu       sync.Mutex
	attempts []RouteAttempt
}

func (s *routeTestSink) RecordAttempt(_ context.Context, attempt RouteAttempt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts = append(s.attempts, attempt)
	return nil
}

func testRoute(providers ...*routeTestProvider) *routedProvider {
	r := &routedProvider{companyID: 7, task: "chat", config: RouteConfig{TimeoutMS: 1000, AttemptTimeoutMS: 100, MaxAttempts: 3, FallbackOn: []string{"timeout", "rate_limit", "unavailable"}}}
	for i, provider := range providers {
		r.candidates = append(r.candidates, routedCandidate{provider: provider, connectionID: int64(i + 1), vendor: "test", model: "selected"})
	}
	return r
}

func TestRouteReachesThirdCandidateAndRecordsEveryAttempt(t *testing.T) {
	first := &routeTestProvider{err: &llmerr.ProviderError{Status: 429, Message: "secret"}}
	second := &routeTestProvider{err: &llmerr.ProviderError{Status: 503}}
	third := &routeTestProvider{}
	r := testRoute(first, second, third)
	sink := &routeTestSink{}
	r.sink = sink
	out, err := r.GenerateJSON(context.Background(), ports.Prompt{})
	if err != nil || out.Vendor != "test" || out.Model != "selected" || third.calls != 1 {
		t.Fatalf("result=%+v err=%v calls=%d", out, err, third.calls)
	}
	if len(sink.attempts) != 3 || sink.attempts[0].Status != "rate_limit" || sink.attempts[1].Status != "unavailable" || sink.attempts[2].Status != "ok" {
		t.Fatalf("attempts=%+v", sink.attempts)
	}
	if sink.attempts[0].RequestID != sink.attempts[2].RequestID {
		t.Fatal("attempts must share request ID")
	}
}

func TestRouteStopsOnRejectedRequests(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 422} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			first, second := &routeTestProvider{err: &llmerr.ProviderError{Status: status, Message: "private-key"}}, &routeTestProvider{}
			_, err := testRoute(first, second).Generate(context.Background(), ports.Prompt{})
			if err == nil || second.calls != 0 || strings.Contains(err.Error(), "private-key") {
				t.Fatalf("err=%v calls=%d", err, second.calls)
			}
		})
	}
}

func TestRouteHonorsAttemptBudgetAndCancellation(t *testing.T) {
	first, second := &routeTestProvider{err: &llmerr.ProviderError{Status: 429}}, &routeTestProvider{}
	r := testRoute(first, second)
	r.config.MaxAttempts = 1
	if _, err := r.Generate(context.Background(), ports.Prompt{}); err == nil || second.calls != 0 {
		t.Fatal("attempt budget ignored")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	first.calls = 0
	if _, err := r.Generate(ctx, ports.Prompt{}); err == nil || first.calls != 0 {
		t.Fatal("canceled request reached provider")
	}
	r = testRoute(&routeTestProvider{wait: true}, second)
	r.config.AttemptTimeoutMS = 5
	if _, err := r.Generate(context.Background(), ports.Prompt{}); err != nil || second.calls != 1 {
		t.Fatalf("attempt timeout did not fall back: %v", err)
	}
}

func TestRouteStreamingFallsBackOnlyBeforeOpening(t *testing.T) {
	chunks := make(chan ports.Completion, 1)
	chunks <- ports.Completion{Text: "partial"}
	close(chunks)
	first := &routeTestProvider{err: &llmerr.ProviderError{Status: 503}}
	second, third := &routeTestProvider{stream: chunks}, &routeTestProvider{}
	stream, err := testRoute(first, second, third).Stream(context.Background(), ports.Prompt{})
	if err != nil {
		t.Fatal(err)
	}
	var text string
	for chunk := range stream {
		text += chunk.Text
	}
	if text != "partial" || third.calls != 0 {
		t.Fatal("opened stream was replayed")
	}
}

func TestRouteStreamStopsWhenConsumerCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	chunks := make(chan ports.Completion, 1)
	chunks <- ports.Completion{Text: "partial"}
	stream, err := testRoute(&routeTestProvider{stream: chunks}).Stream(ctx, ports.Prompt{})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-stream:
	case <-time.After(time.Second):
		t.Fatal("stream did not stop")
	}
}

type routeTestLookup struct {
	config      RouteConfig
	found       bool
	connections map[int64]ProviderConnection
}

func (r routeTestLookup) RouteFor(context.Context, int64, string) (RouteConfig, bool, error) {
	return r.config, r.found, nil
}

func (r routeTestLookup) ConnectionFor(_ context.Context, _ int64, id int64) (ProviderConnection, error) {
	c, found := r.connections[id]
	if !found {
		return c, errors.New("missing")
	}
	return c, nil
}

func TestTaskSelectionUsesDiscoveredModelsAndPreservesChoice(t *testing.T) {
	lookup := routeTestLookup{found: true, config: RouteConfig{Candidates: []RouteCandidate{{1, "default"}, {2, "backup"}}, TimeoutMS: 5000, AttemptTimeoutMS: 1000, MaxAttempts: 2}, connections: map[int64]ProviderConnection{
		1: {ID: 1, Provider: VendorOpenAI, APIKey: "test", Active: true, Models: []ProviderModel{{ID: "default"}, {ID: "discovered"}}},
		2: {ID: 2, Provider: VendorOpenAI, APIKey: "test", Active: true, Models: []ProviderModel{{ID: "backup"}}},
	}}
	r, err := NewCompanyResolver(stubLookup{ok: true}, Settings{Vendor: VendorOpenAI, OpenAIKey: "platform-key"})
	if err != nil {
		t.Fatal(err)
	}
	r.WithRoutes(lookup, nil)
	ctx := ctxkey.WithCompanyID(context.Background(), 7)
	selected := WithModelSelection(ctx, ModelSelection{Model: "discovered", ConnectionID: 1})
	provider, err := r.ResolveTask(selected, 7, "chat")
	if err != nil {
		t.Fatal(err)
	}
	routed := provider.(*routedProvider)
	if len(routed.candidates) != 1 || routed.candidates[0].model != "discovered" {
		t.Fatalf("candidates=%+v", routed.candidates)
	}
	if _, err := r.ResolveTask(selected, 8, "chat"); err == nil {
		t.Fatal("cross-company resolution accepted")
	}
	if _, err := r.ResolveTask(WithModelSelection(ctx, ModelSelection{Model: "invented"}), 7, "chat"); err == nil {
		t.Fatal("unknown model accepted")
	}
	lookup.config.AllowModelSubstitution = true
	r.WithRoutes(lookup, nil)
	provider, err = r.ResolveTask(selected, 7, "chat")
	if err != nil || len(provider.(*routedProvider).candidates) != 2 {
		t.Fatalf("substitution=%v", err)
	}
	if _, err := r.buildConnection(ProviderConnection{Provider: VendorOpenAI}, "model"); err == nil {
		t.Fatal("platform key inherited")
	}
}

func TestLegacyBackupConfigurationSurvivesLookup(t *testing.T) {
	backup := &BackupConfig{Vendor: VendorGemini, Model: "backup", APIKey: "backup-key"}
	r, err := NewCompanyResolver(stubLookup{ok: true, agent: AgentConfig{Backup: backup, Failover: "quota"}}, Settings{Vendor: VendorOpenAI})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := r.lookupAgent(context.Background(), 7)
	if err != nil || cfg.Backup != backup || cfg.Failover != "quota" {
		t.Fatalf("config=%+v err=%v", cfg, err)
	}
}

func TestRouteLoadsOnlyAttemptedProvidersAndSkipsDisabledConnections(t *testing.T) {
	primary := &routeTestProvider{}
	route := testRoute(primary, &routeTestProvider{})
	route.candidates[1].load = func(context.Context) (ports.LLMProvider, string, error) {
		t.Error("unused backup was loaded")
		return nil, "backup", errors.New("broken backup")
	}
	if _, err := route.Generate(context.Background(), ports.Prompt{}); err != nil {
		t.Fatal(err)
	}
	route = testRoute(&routeTestProvider{}, primary)
	route.config.MaxAttempts = 1
	route.candidates[0].load = func(context.Context) (ports.LLMProvider, string, error) {
		return nil, "disabled", errRouteConnectionDisabled
	}
	if _, err := route.Generate(context.Background(), ports.Prompt{}); err != nil {
		t.Fatal(err)
	}
	if primary.calls != 2 {
		t.Fatal("disabled connection consumed upstream attempt budget")
	}
}

func TestChatModelChoicesUseAllowedConnectionsAndHideCredentials(t *testing.T) {
	lookup := routeTestLookup{found: true, config: RouteConfig{
		Version: 4, Candidates: []RouteCandidate{{1, "default"}, {2, "backup"}}, TimeoutMS: 5000, AttemptTimeoutMS: 1000, MaxAttempts: 2,
	}, connections: map[int64]ProviderConnection{
		1: {ID: 1, Provider: VendorOpenAI, APIKey: "private-key", Active: true, Models: []ProviderModel{{ID: "default"}, {ID: "discovered"}, {ID: "embedding", Capabilities: map[string]bool{"generate": false}}}},
		2: {ID: 2, Provider: VendorOpenAI, Active: false, Models: []ProviderModel{{ID: "backup"}}},
	}}
	resolver, err := NewCompanyResolver(stubLookup{ok: true}, Settings{Vendor: VendorOpenAI})
	if err != nil {
		t.Fatal(err)
	}
	resolver.WithRoutes(lookup, nil)
	models, err := resolver.TaskModels(ctxkey.WithCompanyID(context.Background(), 7), 7, "chat")
	if err != nil || models.RouteVersion != 4 || len(models.Models) != 2 || !models.Models[0].Default {
		t.Fatalf("models=%+v err=%v", models, err)
	}
	raw, err := json.Marshal(models)
	if err != nil || strings.Contains(string(raw), "private-key") {
		t.Fatal("model choices exposed credential")
	}
}
