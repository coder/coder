package recorder

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/metrics"
)

// captureRecorder avoids an import cycle: aibridge/internal/testutil imports
// this package, so the recorder internal test cannot.
type captureRecorder struct {
	mu   sync.Mutex
	seen []*TokenUsageRecord
}

func (c *captureRecorder) RecordTokenUsage(_ context.Context, req *TokenUsageRecord) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seen = append(c.seen, req)
	return nil
}

func (*captureRecorder) RecordInterception(context.Context, *InterceptionRecord) error {
	return nil
}

func (*captureRecorder) RecordInterceptionEnded(context.Context, *InterceptionRecordEnded) error {
	return nil
}

func (*captureRecorder) RecordPromptUsage(context.Context, *PromptUsageRecord) error {
	return nil
}

func (*captureRecorder) RecordToolUsage(context.Context, *ToolUsageRecord) error {
	return nil
}

func (*captureRecorder) RecordModelThought(context.Context, *ModelThoughtRecord) error {
	return nil
}

func (c *captureRecorder) tokenUsages() []*TokenUsageRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]*TokenUsageRecord, len(c.seen))
	copy(cp, c.seen)
	return cp
}

// TestAsyncRecorder_RecordTokenUsage_NegativeDoesNotPanic exercises the full
// async path: a provider emitting negative values must not panic the counter.
func TestAsyncRecorder_RecordTokenUsage_NegativeDoesNotPanic(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	mock := &captureRecorder{}
	ar := NewAsyncRecorder(slog.Make(), mock, time.Second)
	ar.WithProvider("openai")
	ar.WithModel("gpt-test")
	ar.WithInitiatorID("init-test")
	ar.WithClient("client-test")
	ar.WithMetrics(m)

	// Input and an ExtraTokenTypes value are negative; before the guard this
	// reached Counter.Add and panicked.
	err := ar.RecordTokenUsage(context.Background(), &TokenUsageRecord{
		InterceptionID: "intc-1",
		MsgID:          "msg-1",
		Input:          -5,
		Output:         20,
		ExtraTokenTypes: map[string]int64{
			"completion_reasoning": -3,
		},
	})
	require.NoError(t, err)

	// Drain the goroutine before reading, or the writes may not have landed.
	ar.Wait()

	inputCounter := m.TokenUseCount.WithLabelValues("openai", "gpt-test", "input", "init-test", "client-test")
	outputCounter := m.TokenUseCount.WithLabelValues("openai", "gpt-test", "output", "init-test", "client-test")
	reasoningCounter := m.TokenUseCount.WithLabelValues("openai", "gpt-test", "completion_reasoning", "init-test", "client-test")

	assert.Equal(t, 0.0, testutil.ToFloat64(inputCounter))
	assert.Equal(t, 0.0, testutil.ToFloat64(reasoningCounter))
	assert.Equal(t, 20.0, testutil.ToFloat64(outputCounter))

	// The DB path is untouched by the metric guard: the raw record is still
	// recorded exactly as the provider sent it.
	usages := mock.tokenUsages()
	require.Len(t, usages, 1)
	assert.Equal(t, int64(-5), usages[0].Input)
}
