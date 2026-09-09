package recorder

import (
	"context"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/metrics"
)

var (
	_ Recorder = &WrappedRecorder{}
	_ Recorder = &AsyncRecorder{}
)

// WrappedRecorder is a convenience struct which implements Recorder and resolves a client before calling each method.
// It also sets the start/creation time of each record.
type WrappedRecorder struct {
	clientFn func(context.Context) (Recorder, error)
}

func (r *WrappedRecorder) RecordInterception(ctx context.Context, req *InterceptionRecord) error {
	client, err := r.clientFn(ctx)
	if err != nil {
		return xerrors.Errorf("acquire client: %w", err)
	}

	req.StartedAt = time.Now()
	return client.RecordInterception(ctx, req)
}

func (r *WrappedRecorder) RecordInterceptionEnded(ctx context.Context, req *InterceptionRecordEnded) error {
	client, err := r.clientFn(ctx)
	if err != nil {
		return xerrors.Errorf("acquire client: %w", err)
	}

	req.EndedAt = time.Now().UTC()
	return client.RecordInterceptionEnded(ctx, req)
}

func (r *WrappedRecorder) RecordPromptUsage(ctx context.Context, req *PromptUsageRecord) error {
	client, err := r.clientFn(ctx)
	if err != nil {
		return xerrors.Errorf("acquire client: %w", err)
	}

	req.CreatedAt = time.Now()
	return client.RecordPromptUsage(ctx, req)
}

func (r *WrappedRecorder) RecordTokenUsage(ctx context.Context, req *TokenUsageRecord) error {
	client, err := r.clientFn(ctx)
	if err != nil {
		return xerrors.Errorf("acquire client: %w", err)
	}

	req.CreatedAt = time.Now()
	return client.RecordTokenUsage(ctx, req)
}

func (r *WrappedRecorder) RecordToolUsage(ctx context.Context, req *ToolUsageRecord) error {
	client, err := r.clientFn(ctx)
	if err != nil {
		return xerrors.Errorf("acquire client: %w", err)
	}

	req.CreatedAt = time.Now()
	return client.RecordToolUsage(ctx, req)
}

func (r *WrappedRecorder) RecordModelThought(ctx context.Context, req *ModelThoughtRecord) error {
	client, err := r.clientFn(ctx)
	if err != nil {
		return xerrors.Errorf("acquire client: %w", err)
	}

	req.CreatedAt = time.Now()
	return client.RecordModelThought(ctx, req)
}

// NewWrappedRecorder creates a [WrappedRecorder]. clientFn receives the
// context of the call it serves.
func NewWrappedRecorder(clientFn func(context.Context) (Recorder, error)) *WrappedRecorder {
	return &WrappedRecorder{clientFn: clientFn}
}

// AsyncRecorder calls [Recorder] methods asynchronously, discarding any errors
// which may occur; wrap it in a [LogRecorder] to have those logged.
type AsyncRecorder struct {
	wrapped Recorder
	timeout time.Duration
	metrics *metrics.Metrics

	provider    string
	model       string
	initiatorID string
	client      string

	wg sync.WaitGroup
}

func NewAsyncRecorder(wrapped Recorder, timeout time.Duration) *AsyncRecorder {
	return &AsyncRecorder{wrapped: wrapped, timeout: timeout}
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

		_ = a.wrapped.RecordInterceptionEnded(timedCtx, req)
	}()

	return nil // Caller is not interested in error.
}

func (a *AsyncRecorder) RecordPromptUsage(ctx context.Context, req *PromptUsageRecord) error {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		timedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), a.timeout)
		defer cancel()

		_ = a.wrapped.RecordPromptUsage(timedCtx, req)

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

		_ = a.wrapped.RecordTokenUsage(timedCtx, req)

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

		_ = a.wrapped.RecordToolUsage(timedCtx, req)

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

		_ = a.wrapped.RecordModelThought(timedCtx, req)
	}()

	return nil // Caller is not interested in error.
}

func (a *AsyncRecorder) Wait() {
	a.wg.Wait()
}
