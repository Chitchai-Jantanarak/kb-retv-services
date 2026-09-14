package llm

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/llm/llmerr"
	apperr "github.com/my/app/internal/shared/errors"
)

type routedCandidate struct {
	provider      ports.LLMProvider
	load          func(context.Context) (ports.LLMProvider, string, error)
	connectionID  int64
	vendor, model string
}

type routedProvider struct {
	config     RouteConfig
	candidates []routedCandidate
	companyID  int64
	task       string
	sink       RouteAttemptSink
}

var errRouteConnectionDisabled = errors.New("AI connection disabled")

func (c *routedCandidate) resolve(ctx context.Context) error {
	if c.load == nil {
		return nil
	}
	provider, vendor, err := c.load(ctx)
	c.provider, c.vendor = provider, vendor
	return err
}

func (r *routedProvider) Generate(ctx context.Context, prompt ports.Prompt) (ports.Completion, error) {
	return r.call(ctx, "generate", prompt)
}

func (r *routedProvider) GenerateJSON(ctx context.Context, prompt ports.Prompt) (ports.Completion, error) {
	return r.call(ctx, "json", prompt)
}

func (r *routedProvider) call(ctx context.Context, operation string, prompt ports.Prompt) (ports.Completion, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(r.config.TimeoutMS)*time.Millisecond)
	defer cancel()
	requestID := uuid.NewString()
	attempts := 0
	for index, candidate := range r.candidates {
		if attempts >= r.config.MaxAttempts {
			break
		}
		if err := ctx.Err(); err != nil {
			return ports.Completion{}, routeError(err)
		}
		callCtx, stop := context.WithTimeout(ctx, time.Duration(r.config.AttemptTimeoutMS)*time.Millisecond)
		started := time.Now()
		var out ports.Completion
		err := candidate.resolve(callCtx)
		if errors.Is(err, errRouteConnectionDisabled) {
			stop()
			r.record(ctx, requestID, index+1, candidate, operation, started, out, err)
			continue
		}
		attempts++
		if err == nil && operation == "json" {
			out, err = candidate.provider.GenerateJSON(callCtx, prompt)
		} else if err == nil {
			out, err = candidate.provider.Generate(callCtx, prompt)
		}
		stop()
		r.record(ctx, requestID, index+1, candidate, operation, started, out, err)
		if err == nil {
			return candidateCompletion(candidate, out), nil
		}
		if ctx.Err() != nil || !slices.Contains(r.config.FallbackOn, routeFailure(err)) {
			return ports.Completion{}, routeError(err)
		}
	}
	return ports.Completion{}, apperr.New(apperr.CodeUnavailable, "AI task route exhausted")
}

func (r *routedProvider) Stream(ctx context.Context, prompt ports.Prompt) (<-chan ports.Completion, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(r.config.TimeoutMS)*time.Millisecond)
	requestID := uuid.NewString()
	attempts := 0
	for index, candidate := range r.candidates {
		if attempts >= r.config.MaxAttempts || ctx.Err() != nil {
			break
		}
		callCtx, stop := context.WithTimeout(ctx, time.Duration(r.config.AttemptTimeoutMS)*time.Millisecond)
		started := time.Now()
		var stream <-chan ports.Completion
		err := candidate.resolve(callCtx)
		if errors.Is(err, errRouteConnectionDisabled) {
			stop()
			r.record(ctx, requestID, index+1, candidate, "stream", started, ports.Completion{}, err)
			continue
		}
		attempts++
		if err == nil {
			stream, err = candidate.provider.Stream(callCtx, prompt)
		}
		if err != nil {
			stop()
			r.record(ctx, requestID, index+1, candidate, "stream", started, ports.Completion{}, err)
			if ctx.Err() != nil || !slices.Contains(r.config.FallbackOn, routeFailure(err)) {
				cancel()
				return nil, routeError(err)
			}
			continue
		}
		output := make(chan ports.Completion)
		// NOTE: Once a stream opens, never replay it on another model.
		go func() {
			defer close(output)
			defer cancel()
			defer stop()
			var last ports.Completion
			defer func() { r.record(ctx, requestID, index+1, candidate, "stream", started, last, callCtx.Err()) }()
			for {
				select {
				case <-callCtx.Done():
					return
				case chunk, ok := <-stream:
					if !ok {
						return
					}
					last = candidateCompletion(candidate, chunk)
					select {
					case output <- last:
					case <-callCtx.Done():
						return
					}
				}
			}
		}()
		return output, nil
	}
	err := ctx.Err()
	cancel()
	if err != nil {
		return nil, routeError(err)
	}
	return nil, apperr.New(apperr.CodeUnavailable, "AI task route exhausted")
}

func candidateCompletion(candidate routedCandidate, out ports.Completion) ports.Completion {
	if out.Model == "" {
		out.Model = candidate.model
	}
	if out.Vendor == "" {
		out.Vendor = candidate.vendor
	}
	return out
}

func routeFailure(err error) string {
	if errors.Is(err, errRouteConnectionDisabled) {
		return "disabled"
	}
	if err == nil {
		return "ok"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var pe *llmerr.ProviderError
	if errors.As(err, &pe) {
		if pe.Status == 429 {
			return "rate_limit"
		}
		if pe.Status == 408 || pe.Status == 504 {
			return "timeout"
		}
		if pe.Status >= 500 && pe.Status <= 599 {
			return "unavailable"
		}
		return "rejected"
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "timeout"
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return "unavailable"
	}
	return "error"
}

func routeError(err error) error {
	code := apperr.CodeUpstreamFailed
	if routeFailure(err) == "rate_limit" {
		code = apperr.CodeRateLimited
	}
	if errors.Is(err, context.Canceled) || routeFailure(err) == "timeout" {
		code = apperr.CodeUnavailable
	}
	return apperr.New(code, "AI provider request failed")
}

func (r *routedProvider) record(ctx context.Context, requestID string, attempt int, candidate routedCandidate, op string, started time.Time, out ports.Completion, err error) {
	if r.sink == nil {
		return
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	recordErr := r.sink.RecordAttempt(recordCtx, RouteAttempt{
		CompanyID: r.companyID, ConnectionID: candidate.connectionID,
		RequestID: requestID, Task: r.task, Version: r.config.Version, Attempt: attempt,
		Provider: candidate.vendor, Model: candidate.model, Operation: op,
		Status: routeFailure(err), LatencyMS: int(time.Since(started).Milliseconds()), Usage: out.Usage,
	})
	if recordErr != nil {
		slog.Error("AI route attempt could not be recorded", "company_id", r.companyID, "request_id", requestID)
	}
}
