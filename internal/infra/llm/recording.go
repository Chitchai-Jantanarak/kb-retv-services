package llm

import (
	"context"
	"errors"
	"time"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/llm/llmerr"
	"github.com/my/app/internal/shared/ctxkey"
)

type RequestLog struct {
	CompanyID    int64
	Vendor       string
	Model        string
	Op           string
	Status       string
	HTTPStatus   int
	LatencyMs    int
	InputTokens  int
	OutputTokens int
	Error        string
}

type RequestSink interface {
	Record(ctx context.Context, l RequestLog) error
}

type recording struct {
	inner  ports.LLMProvider
	vendor string
	model  string
	sink   RequestSink
}

func NewRecording(inner ports.LLMProvider, vendor, model string, sink RequestSink) ports.LLMProvider {
	if inner == nil {
		return nil
	}
	return &recording{inner: inner, vendor: vendor, model: model, sink: sink}
}

func (r *recording) Generate(ctx context.Context, p ports.Prompt) (ports.Completion, error) {
	start := time.Now()
	out, err := r.inner.Generate(ctx, p)
	r.record(ctx, "generate", start, out, err)
	return out, err
}

func (r *recording) GenerateJSON(ctx context.Context, p ports.Prompt) (ports.Completion, error) {
	start := time.Now()
	out, err := r.inner.GenerateJSON(ctx, p)
	r.record(ctx, "json", start, out, err)
	return out, err
}

func (r *recording) Stream(ctx context.Context, p ports.Prompt) (<-chan ports.Completion, error) {
	start := time.Now()
	ch, err := r.inner.Stream(ctx, p)
	if err != nil {
		r.record(ctx, "stream", start, ports.Completion{}, err)
		return nil, err
	}

	out := make(chan ports.Completion)
	go func() {
		defer close(out)
		var last ports.Completion
		for chunk := range ch {
			if chunk.Vendor != "" || chunk.Model != "" || chunk.Usage != (ports.TokenUsage{}) {
				last = chunk
			}
			out <- chunk
		}
		r.record(ctx, "stream", start, last, nil)
	}()
	return out, nil
}

func (r *recording) record(ctx context.Context, op string, start time.Time, out ports.Completion, err error) {
	companyID, _ := ctxkey.CompanyID(ctx)

	vendor := r.vendor
	if out.Vendor != "" {
		vendor = out.Vendor
	}
	model := r.model
	if out.Model != "" {
		model = out.Model
	}

	status := "ok"
	httpStatus := 0
	errMsg := ""
	if err != nil {
		status = "error"
		errMsg = truncateRunes(err.Error(), 255)
		var pe *llmerr.ProviderError
		if errors.As(err, &pe) {
			httpStatus = pe.Status
		}
	}

	l := RequestLog{
		CompanyID:    companyID,
		Vendor:       vendor,
		Model:        model,
		Op:           op,
		Status:       status,
		HTTPStatus:   httpStatus,
		LatencyMs:    int(time.Since(start).Milliseconds()),
		InputTokens:  out.Usage.Input,
		OutputTokens: out.Usage.Output,
		Error:        errMsg,
	}

	if r.sink == nil {
		return
	}
	base := context.WithoutCancel(ctx)
	go func() {
		sinkCtx, cancel := context.WithTimeout(base, 3*time.Second)
		defer cancel()
		_ = r.sink.Record(sinkCtx, l)
	}()
}

func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
