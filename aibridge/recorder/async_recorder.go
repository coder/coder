package recorder

import (
	"context"
	"sync"
	"time"

	"github.com/coder/coder/v2/aibridge/metrics"
)

var _ Recorder = &AsyncRecorder{}

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
