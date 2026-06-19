package recorder_test

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// TestAsyncRecorder_RecordTokenUsage_NegativeDoesNotPanic exercises the full
// async path: a provider emitting negative values must not panic the counter.
func TestAsyncRecorder_RecordTokenUsage_NegativeDoesNotPanic(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	mock := &testutil.MockRecorder{}
	ar := recorder.NewAsyncRecorder(slog.Make(), mock, time.Second)
	ar.WithProvider("openai")
	ar.WithModel("gpt-test")
	ar.WithInitiatorID("init-test")
	ar.WithClient("client-test")
	ar.WithMetrics(m)

	// Input and an ExtraTokenTypes value are negative; before the guard this
	// reached Counter.Add and panicked.
	err := ar.RecordTokenUsage(context.Background(), &recorder.TokenUsageRecord{
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

	assert.Equal(t, 0.0, promtestutil.ToFloat64(inputCounter))
	assert.Equal(t, 0.0, promtestutil.ToFloat64(reasoningCounter))
	assert.Equal(t, 20.0, promtestutil.ToFloat64(outputCounter))

	// The DB path is untouched by the metric guard: the raw record is still
	// recorded exactly as the provider sent it.
	usages := mock.RecordedTokenUsages()
	require.Len(t, usages, 1)
	assert.Equal(t, int64(-5), usages[0].Input)
}
