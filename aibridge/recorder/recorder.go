package recorder

import (
	"context"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"

	"go.opentelemetry.io/otel/trace"

	"github.com/coder/aibridge/metrics"
	"github.com/coder/aibridge/tracing"
)

var (
	_ Recorder = &WrappedRecorder{}
	_ Recorder = &AsyncRecorder{}
)

// WrappedRecorder is a convenience struct which implements RecorderClient and resolves a client before calling each method.
// It also sets the start/creation time of each record.
type WrappedRecorder struct {
	logger   slog.Logger
	tracer   trace.Tracer
	clientFn func() (Recorder, error)
}

func (r *WrappedRecorder) RecordInterception(ctx context.Context, req *InterceptionRecord) (outErr error) {
	ctx, span := r.tracer.Start(ctx, "Intercept.RecordInterception", trace.WithAttributes(tracing.InterceptionAttributesFromContext(ctx)...))
	defer tracing.EndSpanErr(span, &outErr)

	client, err := r.clientFn()
	if err != nil {
		return xerrors.Errorf("acquire client: %w", err)
	}

	req.StartedAt = time.Now()
	if err = client.RecordInterception(ctx, req); err == nil {
		return nil
	}

	r.logger.Warn(ctx, "failed to record interception", slog.Error(err), slog.F("interception_id", req.ID))
	return err
}

func (r *WrappedRecorder) RecordInterceptionEnded(ctx context.Context, req *InterceptionRecordEnded) (outErr error) {
	ctx, span := r.tracer.Start(ctx, "Intercept.RecordInterceptionEnded", trace.WithAttributes(tracing.InterceptionAttributesFromContext(ctx)...))
	defer tracing.EndSpanErr(span, &outErr)

	client, err := r.clientFn()
	if err != nil {
		return xerrors.Errorf("acquire client: %w", err)
	}

	req.EndedAt = time.Now().UTC()
	if err = client.RecordInterceptionEnded(ctx, req); err == nil {
		return nil
	}

	r.logger.Warn(ctx, "failed to record that interception ended", slog.Error(err), slog.F("interception_id", req.ID))
	return err
}

func (r *WrappedRecorder) RecordPromptUsage(ctx context.Context, req *PromptUsageRecord) (outErr error) {
	ctx, span := r.tracer.Start(ctx, "Intercept.RecordPromptUsage", trace.WithAttributes(tracing.InterceptionAttributesFromContext(ctx)...))
	defer tracing.EndSpanErr(span, &outErr)

	client, err := r.clientFn()
	if err != nil {
		return xerrors.Errorf("acquire client: %w", err)
	}

	req.CreatedAt = time.Now()
	if err = client.RecordPromptUsage(ctx, req); err == nil {
		return nil
	}

	r.logger.Warn(ctx, "failed to record prompt usage", slog.Error(err), slog.F("interception_id", req.InterceptionID))
	return err
}

func (r *WrappedRecorder) RecordTokenUsage(ctx context.Context, req *TokenUsageRecord) (outErr error) {
	ctx, span := r.tracer.Start(ctx, "Intercept.RecordTokenUsage", trace.WithAttributes(tracing.InterceptionAttributesFromContext(ctx)...))
	defer tracing.EndSpanErr(span, &outErr)

	client, err := r.clientFn()
	if err != nil {
		return xerrors.Errorf("acquire client: %w", err)
	}

	req.CreatedAt = time.Now()
	if err = client.RecordTokenUsage(ctx, req); err == nil {
		return nil
	}

	r.logger.Warn(ctx, "failed to record token usage", slog.Error(err), slog.F("interception_id", req.InterceptionID))
	return err
}

func (r *WrappedRecorder) RecordToolUsage(ctx context.Context, req *ToolUsageRecord) (outErr error) {
	ctx, span := r.tracer.Start(ctx, "Intercept.RecordToolUsage", trace.WithAttributes(tracing.InterceptionAttributesFromContext(ctx)...))
	defer tracing.EndSpanErr(span, &outErr)

	client, err := r.clientFn()
	if err != nil {
		return xerrors.Errorf("acquire client: %w", err)
	}

	req.CreatedAt = time.Now()
	if err = client.RecordToolUsage(ctx, req); err == nil {
		return nil
	}

	r.logger.Warn(ctx, "failed to record tool usage", slog.Error(err), slog.F("interception_id", req.InterceptionID))
	return err
}

func (r *WrappedRecorder) RecordModelThought(ctx context.Context, req *ModelThoughtRecord) (outErr error) {
	ctx, span := r.tracer.Start(ctx, "Intercept.RecordModelThought", trace.WithAttributes(tracing.InterceptionAttributesFromContext(ctx)...))
	defer tracing.EndSpanErr(span, &outErr)

	client, err := r.clientFn()
	if err != nil {
		return xerrors.Errorf("acquire client: %w", err)
	}

	req.CreatedAt = time.Now()
	if err = client.RecordModelThought(ctx, req); err == nil {
		return nil
	}

	r.logger.Warn(ctx, "failed to record model thought", slog.Error(err), slog.F("interception_id", req.InterceptionID))
	return err
}

func NewWrappedRecorder(logger slog.Logger, tracer trace.Tracer, clientFn func() (Recorder, error)) *WrappedRecorder {
	return &WrappedRecorder{
		logger:   logger,
		tracer:   tracer,
		clientFn: clientFn,
	}
}

// AsyncRecorder calls [Recorder] methods asynchronously and logs any errors which may occur.
type AsyncRecorder struct {
	logger  slog.Logger
	wrapped Recorder
	timeout time.Duration
	metrics *metrics.Metrics

	provider    string
	model       string
	initiatorID string
	client      string

	wg sync.WaitGroup
}

func NewAsyncRecorder(logger slog.Logger, wrapped Recorder, timeout time.Duration) *AsyncRecorder {
	return &AsyncRecorder{logger: logger, wrapped: wrapped, timeout: timeout}
}

func (a *AsyncRecorder) WithMetrics(m any) {
	if m, ok := m.(*metrics.Metrics); ok {
		a.metrics = m
	}
}

func (a *AsyncRecorder) WithProvider(provider string) {
	a.provider = provider
}

func (a *AsyncRecorder) WithModel(model string) {
	a.model = model
}

func (a *AsyncRecorder) WithInitiatorID(initiatorID string) {
	a.initiatorID = initiatorID
}

func (a *AsyncRecorder) WithClient(client string) {
	a.client = client
}

// RecordInterception must NOT be called asynchronously.
// If an interception cannot be recorded, the whole request should fail.
func (*AsyncRecorder) RecordInterception(context.Context, *InterceptionRecord) error {
	panic("RecordInterception must not be called asynchronously")
}

func (a *AsyncRecorder) RecordInterceptionEnded(ctx context.Context, req *InterceptionRecordEnded) error {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		timedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), a.timeout)
		defer cancel()

		err := a.wrapped.RecordInterceptionEnded(timedCtx, req)
		if err != nil {
			a.logger.Warn(timedCtx, "failed to record interception end", slog.F("type", "prompt"), slog.Error(err), slog.F("payload", req))
		}
	}()

	return nil // Caller is not interested in error.
}

func (a *AsyncRecorder) RecordPromptUsage(ctx context.Context, req *PromptUsageRecord) error {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		timedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), a.timeout)
		defer cancel()

		err := a.wrapped.RecordPromptUsage(timedCtx, req)
		if err != nil {
			a.logger.Warn(timedCtx, "failed to record usage", slog.F("type", "prompt"), slog.Error(err), slog.F("payload", req))
		}

		if a.metrics != nil && req.Prompt != "" { // TODO: will be irrelevant once https://github.com/coder/aibridge/issues/55 is fixed.
			a.metrics.PromptCount.WithLabelValues(a.provider, a.model, a.initiatorID, a.client).Add(1)
		}
	}()

	return nil // Caller is not interested in error.
}

func (a *AsyncRecorder) RecordTokenUsage(ctx context.Context, req *TokenUsageRecord) error {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		timedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), a.timeout)
		defer cancel()

		err := a.wrapped.RecordTokenUsage(timedCtx, req)
		if err != nil {
			a.logger.Warn(timedCtx, "failed to record usage", slog.F("type", "token"), slog.Error(err), slog.F("payload", req))
		}

		if a.metrics != nil {
			a.metrics.TokenUseCount.WithLabelValues(a.provider, a.model, "input", a.initiatorID, a.client).Add(float64(req.Input))
			a.metrics.TokenUseCount.WithLabelValues(a.provider, a.model, "output", a.initiatorID, a.client).Add(float64(req.Output))
			a.metrics.TokenUseCount.WithLabelValues(a.provider, a.model, "cache_read_input_tokens", a.initiatorID, a.client).Add(float64(req.CacheReadInputTokens))
			a.metrics.TokenUseCount.WithLabelValues(a.provider, a.model, "cache_write_input_tokens", a.initiatorID, a.client).Add(float64(req.CacheWriteInputTokens))
			for k, v := range req.ExtraTokenTypes {
				a.metrics.TokenUseCount.WithLabelValues(a.provider, a.model, k, a.initiatorID, a.client).Add(float64(v))
			}
		}
	}()

	return nil // Caller is not interested in error.
}

func (a *AsyncRecorder) RecordToolUsage(ctx context.Context, req *ToolUsageRecord) error {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		timedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), a.timeout)
		defer cancel()

		err := a.wrapped.RecordToolUsage(timedCtx, req)
		if err != nil {
			a.logger.Warn(timedCtx, "failed to record usage", slog.F("type", "tool"), slog.Error(err), slog.F("payload", req))
		}

		if a.metrics != nil {
			if req.Injected {
				var srvURL string
				if req.ServerURL != nil {
					srvURL = *req.ServerURL
				}
				a.metrics.InjectedToolUseCount.WithLabelValues(a.provider, a.model, srvURL, req.Tool).Add(1)
			} else {
				a.metrics.NonInjectedToolUseCount.WithLabelValues(a.provider, a.model, req.Tool).Add(1)
			}
		}
	}()

	return nil // Caller is not interested in error.
}

func (a *AsyncRecorder) RecordModelThought(ctx context.Context, req *ModelThoughtRecord) error {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		timedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), a.timeout)
		defer cancel()

		err := a.wrapped.RecordModelThought(timedCtx, req)
		if err != nil {
			a.logger.Warn(timedCtx, "failed to record model thought", slog.F("type", "model_thought"), slog.Error(err), slog.F("payload", req))
		}
	}()

	return nil // Caller is not interested in error.
}

func (a *AsyncRecorder) Wait() {
	a.wg.Wait()
}
