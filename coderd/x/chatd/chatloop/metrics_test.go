package chatloop_test

import (
	"context"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatretry"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
)

func TestNewMetrics_RegistersAllMetrics(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := chatloop.NewMetrics(reg)

	// Initialize vector metrics so they appear in Gather output.
	m.Chats.WithLabelValues(chatloop.StateStreaming)
	m.CompactionTotal.WithLabelValues("anthropic", "claude-sonnet-4-5", chatloop.CompactionResultSuccess)
	m.ToolResultSizeBytes.WithLabelValues("anthropic", "claude-sonnet-4-5", "test")
	m.MessageCount.WithLabelValues("anthropic", "claude-sonnet-4-5")
	m.PromptSizeBytes.WithLabelValues("anthropic", "claude-sonnet-4-5")
	m.TTFTSeconds.WithLabelValues("anthropic", "claude-sonnet-4-5")
	m.StepsTotal.WithLabelValues("anthropic", "claude-sonnet-4-5")
	m.StreamRetriesTotal.WithLabelValues("anthropic", "claude-sonnet-4-5", chaterror.KindTimeout)
	// StreamBufferDroppedTotal is a plain Counter, so it's always present
	// in Gather output once registered; no exerciser call is
	// needed.

	families, err := reg.Gather()
	require.NoError(t, err)

	expected := map[string]dto.MetricType{
		"coderd_chatd_chats":                       dto.MetricType_GAUGE,
		"coderd_chatd_message_count":               dto.MetricType_HISTOGRAM,
		"coderd_chatd_prompt_size_bytes":           dto.MetricType_HISTOGRAM,
		"coderd_chatd_tool_result_size_bytes":      dto.MetricType_HISTOGRAM,
		"coderd_chatd_ttft_seconds":                dto.MetricType_HISTOGRAM,
		"coderd_chatd_compaction_total":            dto.MetricType_COUNTER,
		"coderd_chatd_steps_total":                 dto.MetricType_COUNTER,
		"coderd_chatd_stream_retries_total":        dto.MetricType_COUNTER,
		"coderd_chatd_stream_buffer_dropped_total": dto.MetricType_COUNTER,
	}

	found := make(map[string]dto.MetricType)
	for _, f := range families {
		found[f.GetName()] = f.GetType()
	}

	for name, expectedType := range expected {
		actualType, ok := found[name]
		assert.True(t, ok, "metric %q not registered", name)
		if ok {
			assert.Equal(t, expectedType, actualType, "metric %q has wrong type", name)
		}
	}
}

func TestNopMetrics_DoesNotPanic(t *testing.T) {
	t.Parallel()

	m := chatloop.NopMetrics()

	// Exercise every metric to confirm no nil-pointer panics.
	m.Chats.WithLabelValues("streaming").Inc()
	m.Chats.WithLabelValues("streaming").Dec()
	m.Chats.WithLabelValues("waiting").Inc()
	m.Chats.WithLabelValues("waiting").Dec()
	m.MessageCount.WithLabelValues("anthropic", "claude-sonnet-4-5").Observe(10)
	m.PromptSizeBytes.WithLabelValues("openai", "gpt-5").Observe(4096)
	m.ToolResultSizeBytes.WithLabelValues("anthropic", "claude-sonnet-4-5", "execute").Observe(512)
	m.TTFTSeconds.WithLabelValues("anthropic", "claude-sonnet-4-5").Observe(0.5)
	m.CompactionTotal.WithLabelValues("anthropic", "claude-sonnet-4-5", "success").Inc()
	m.CompactionTotal.WithLabelValues("openai", "gpt-5", "error").Inc()
	m.CompactionTotal.WithLabelValues("google", "gemini-2.5-pro", "timeout").Inc()
	m.StepsTotal.WithLabelValues("anthropic", "claude-sonnet-4-5").Inc()
	m.StreamRetriesTotal.WithLabelValues("anthropic", "claude-sonnet-4-5", chaterror.KindTimeout).Inc()
	m.StreamBufferDroppedTotal.Inc()

	// Nil-receiver guard for RecordStreamRetry and
	// RecordStreamBufferDropped mirrors the existing RecordCompaction nil
	// guard.
	var nilMetrics *chatloop.Metrics
	nilMetrics.RecordStreamRetry("anthropic", "claude-sonnet-4-5", chaterror.ClassifiedError{Kind: chaterror.KindTimeout})
	nilMetrics.RecordStreamBufferDropped()
}

func TestEstimatePromptSize(t *testing.T) {
	t.Parallel()

	messages := []fantasy.Message{
		{
			Role: fantasy.MessageRoleSystem,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "You are a helpful assistant."},
			},
		},
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "Hello world"},
				fantasy.ReasoningPart{Text: "thinking..."},
				fantasy.FilePart{Data: []byte("filedata")},
			},
		},
		{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "Hi there!"},
				fantasy.ToolCallPart{Input: `{"file":"main.go"}`},
			},
		},
		{
			Role: fantasy.MessageRoleTool,
			Content: []fantasy.MessagePart{
				fantasy.ToolResultPart{
					Output: fantasy.ToolResultOutputContentText{Text: "result"},
				},
			},
		},
	}

	size := chatloop.EstimatePromptSize(messages)
	// "You are a helpful assistant." (28) + "Hello world" (11) +
	// "thinking..." (11) + "filedata" (8) +
	// "Hi there!" (9) + `{"file":"main.go"}` (18) +
	// "result" (6) = 91
	assert.Equal(t, 91, size)
}

func TestToolResultSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		result   fantasy.ToolResultContent
		expected int
	}{
		{
			name: "text",
			result: fantasy.ToolResultContent{
				Result: fantasy.ToolResultOutputContentText{Text: "hello"},
			},
			expected: 5,
		},
		{
			name: "error",
			result: fantasy.ToolResultContent{
				Result: fantasy.ToolResultOutputContentError{
					Error: assert.AnError,
				},
			},
			expected: len(assert.AnError.Error()),
		},
		{
			name: "media",
			result: fantasy.ToolResultContent{
				Result: fantasy.ToolResultOutputContentMedia{Data: "base64data"},
			},
			expected: 10,
		},
		{
			name:     "nil_result",
			result:   fantasy.ToolResultContent{},
			expected: 0,
		},
		{
			name: "error_nil_error",
			result: fantasy.ToolResultContent{
				Result: fantasy.ToolResultOutputContentError{Error: nil},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, chatloop.ToolResultSize(tt.result))
		})
	}
}

func TestRecordCompaction(t *testing.T) {
	t.Parallel()

	t.Run("nil metrics does not panic", func(t *testing.T) {
		t.Parallel()
		var m *chatloop.Metrics
		m.RecordCompaction("anthropic", "claude-sonnet-4-5", true, nil)
	})

	tests := []struct {
		name      string
		compacted bool
		err       error
		wantLabel string
		wantCount int
	}{
		{
			name:      "success",
			compacted: true,
			err:       nil,
			wantLabel: chatloop.CompactionResultSuccess,
			wantCount: 1,
		},
		{
			name:      "error",
			compacted: false,
			err:       assert.AnError,
			wantLabel: chatloop.CompactionResultError,
			wantCount: 1,
		},
		{
			name:      "timeout",
			compacted: false,
			err:       context.DeadlineExceeded,
			wantLabel: chatloop.CompactionResultTimeout,
			wantCount: 1,
		},
		{
			name:      "threshold_not_reached",
			compacted: false,
			err:       nil,
			wantLabel: "",
			wantCount: 0,
		},
		{
			name:      "canceled",
			compacted: false,
			err:       context.Canceled,
			wantLabel: "",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reg := prometheus.NewRegistry()
			m := chatloop.NewMetrics(reg)
			m.RecordCompaction("test-provider", "test-model", tt.compacted, tt.err)

			families, err := reg.Gather()
			require.NoError(t, err)

			if tt.wantCount == 0 {
				for _, f := range families {
					assert.NotEqual(t, "coderd_chatd_compaction_total", f.GetName(),
						"compaction_total should not be recorded")
				}
				return
			}

			var found bool
			for _, f := range families {
				if f.GetName() != "coderd_chatd_compaction_total" {
					continue
				}
				found = true
				require.Len(t, f.GetMetric(), 1)
				metric := f.GetMetric()[0]
				assert.Equal(t, float64(tt.wantCount), metric.GetCounter().GetValue())
				// Check labels: provider, model, result.
				labels := map[string]string{}
				for _, lp := range metric.GetLabel() {
					labels[lp.GetName()] = lp.GetValue()
				}
				assert.Equal(t, "test-provider", labels["provider"])
				assert.Equal(t, "test-model", labels["model"])
				assert.Equal(t, tt.wantLabel, labels["result"])
			}
			assert.True(t, found, "compaction_total metric not found")
		})
	}
}

func TestRecordStreamRetry(t *testing.T) {
	t.Parallel()

	// One row per chaterror.Kind* constant. Production callers always
	// reach RecordStreamRetry through chaterror.Classify, which
	// guarantees Kind is non-empty, so no empty-string case is
	// needed.
	tests := []struct {
		name string
		kind string
	}{
		{name: "overloaded", kind: chaterror.KindOverloaded},
		{name: "rate_limit", kind: chaterror.KindRateLimit},
		{name: "timeout", kind: chaterror.KindTimeout},
		{name: "startup_timeout", kind: chaterror.KindStartupTimeout},
		{name: "auth", kind: chaterror.KindAuth},
		{name: "config", kind: chaterror.KindConfig},
		{name: "generic", kind: chaterror.KindGeneric},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reg := prometheus.NewRegistry()
			m := chatloop.NewMetrics(reg)
			m.RecordStreamRetry("test-provider", "test-model", chaterror.ClassifiedError{
				Kind: tt.kind,
			})

			families, err := reg.Gather()
			require.NoError(t, err)

			var found bool
			for _, f := range families {
				if f.GetName() != "coderd_chatd_stream_retries_total" {
					continue
				}
				found = true
				require.Len(t, f.GetMetric(), 1)
				metric := f.GetMetric()[0]
				assert.Equal(t, float64(1), metric.GetCounter().GetValue())
				labels := map[string]string{}
				for _, lp := range metric.GetLabel() {
					labels[lp.GetName()] = lp.GetValue()
				}
				assert.Equal(t, "test-provider", labels["provider"])
				assert.Equal(t, "test-model", labels["model"])
				assert.Equal(t, tt.kind, labels["kind"])
			}
			assert.True(t, found, "stream_retries_total metric not found")
		})
	}
}

func TestRecordStreamBufferDropped(t *testing.T) {
	t.Parallel()

	t.Run("nil metrics does not panic", func(t *testing.T) {
		t.Parallel()
		var m *chatloop.Metrics
		m.RecordStreamBufferDropped()
	})

	t.Run("increments monotonically", func(t *testing.T) {
		t.Parallel()

		reg := prometheus.NewRegistry()
		m := chatloop.NewMetrics(reg)

		m.RecordStreamBufferDropped()
		m.RecordStreamBufferDropped()
		m.RecordStreamBufferDropped()

		families, err := reg.Gather()
		require.NoError(t, err)

		var found bool
		for _, f := range families {
			if f.GetName() != "coderd_chatd_stream_buffer_dropped_total" {
				continue
			}
			found = true
			require.Len(t, f.GetMetric(), 1)
			assert.Equal(t, float64(3), f.GetMetric()[0].GetCounter().GetValue())
			assert.Empty(t, f.GetMetric()[0].GetLabel(),
				"stream_buffer_dropped_total must be an unlabeled counter")
		}
		assert.True(t, found, "stream_buffer_dropped_total metric not found")
	})
}

func TestRun_RecordsMetrics(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	metrics := chatloop.NewMetrics(reg)

	model := &chattest.FakeModel{
		ProviderName: "test-provider",
		ModelName:    "test-model",
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			return func(yield func(fantasy.StreamPart) bool) {
				parts := []fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "t1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "t1", Delta: "hello"},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "t1"},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}
				for _, p := range parts {
					if !yield(p) {
						return
					}
				}
			}, nil
		},
	}

	err := chatloop.Run(context.Background(), chatloop.RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "hello"},
				},
			},
		},
		MaxSteps: 1,
		PersistStep: func(_ context.Context, _ chatloop.PersistedStep) error {
			return nil
		},
		Metrics: metrics,
	})
	require.NoError(t, err)

	families, err := reg.Gather()
	require.NoError(t, err)

	assertProviderModelLabels := func(t *testing.T, metric *dto.Metric) {
		t.Helper()
		labels := map[string]string{}
		for _, lp := range metric.GetLabel() {
			labels[lp.GetName()] = lp.GetValue()
		}
		assert.Equal(t, "test-provider", labels["provider"])
		assert.Equal(t, "test-model", labels["model"])
	}

	found := make(map[string]bool)
	for _, f := range families {
		found[f.GetName()] = true

		switch f.GetName() {
		case "coderd_chatd_steps_total":
			require.Len(t, f.GetMetric(), 1)
			assert.Equal(t, float64(1), f.GetMetric()[0].GetCounter().GetValue(),
				"steps_total should be 1 after one step")
			assertProviderModelLabels(t, f.GetMetric()[0])
		case "coderd_chatd_message_count":
			require.Len(t, f.GetMetric(), 1)
			assert.Equal(t, uint64(1), f.GetMetric()[0].GetHistogram().GetSampleCount(),
				"message_count should have 1 observation")
			assertProviderModelLabels(t, f.GetMetric()[0])
		case "coderd_chatd_prompt_size_bytes":
			require.Len(t, f.GetMetric(), 1)
			assert.Equal(t, uint64(1), f.GetMetric()[0].GetHistogram().GetSampleCount(),
				"prompt_size_bytes should have 1 observation")
			assertProviderModelLabels(t, f.GetMetric()[0])
		case "coderd_chatd_ttft_seconds":
			require.Len(t, f.GetMetric(), 1)
			assert.Equal(t, uint64(1), f.GetMetric()[0].GetHistogram().GetSampleCount(),
				"ttft_seconds should have 1 observation")
			assertProviderModelLabels(t, f.GetMetric()[0])
		}
	}

	assert.True(t, found["coderd_chatd_steps_total"], "steps_total not recorded")
	assert.True(t, found["coderd_chatd_message_count"], "message_count not recorded")
	assert.True(t, found["coderd_chatd_prompt_size_bytes"], "prompt_size_bytes not recorded")
	assert.True(t, found["coderd_chatd_ttft_seconds"], "ttft_seconds not recorded")
}

// TestRun_StreamRetry_RecordsMetric exercises the end-to-end retry
// path: a retryable error on the first Stream call, success on the
// second. Asserts both the metric and the back-compat OnRetry
// callback fire.
//
// Note: chatretry.Retry uses time.NewTimer (not quartz.Clock), so
// this test pays chatretry.InitialDelay (1s) of real wall-clock
// time per retry. Keep it to one retry.
func TestRun_StreamRetry_RecordsMetric(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	metrics := chatloop.NewMetrics(reg)

	type retryCall struct {
		attempt    int
		classified chatretry.ClassifiedError
	}
	var retries []retryCall

	calls := 0
	model := &chattest.FakeModel{
		ProviderName: "test-provider",
		ModelName:    "test-model",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			calls++
			if calls == 1 {
				return nil, xerrors.New("received status 429 from upstream")
			}
			return func(yield func(fantasy.StreamPart) bool) {
				yield(fantasy.StreamPart{
					Type:         fantasy.StreamPartTypeFinish,
					FinishReason: fantasy.FinishReasonStop,
				})
			}, nil
		},
	}

	err := chatloop.Run(context.Background(), chatloop.RunOptions{
		Model:                model,
		MaxSteps:             1,
		ContextLimitFallback: 4096,
		PersistStep: func(_ context.Context, _ chatloop.PersistedStep) error {
			return nil
		},
		Metrics: metrics,
		OnRetry: func(
			attempt int,
			_ error,
			classified chatretry.ClassifiedError,
			_ time.Duration,
		) {
			retries = append(retries, retryCall{
				attempt:    attempt,
				classified: classified,
			})
		},
	})
	require.NoError(t, err)

	// Back-compat: OnRetry still fires with classified error.
	require.Len(t, retries, 1)
	assert.Equal(t, 1, retries[0].attempt)
	assert.Equal(t, chaterror.KindRateLimit, retries[0].classified.Kind)
	assert.Equal(t, "test-provider", retries[0].classified.Provider)

	// Metric assertion.
	families, err := reg.Gather()
	require.NoError(t, err)

	var found bool
	for _, f := range families {
		if f.GetName() != "coderd_chatd_stream_retries_total" {
			continue
		}
		found = true
		require.Len(t, f.GetMetric(), 1)
		metric := f.GetMetric()[0]
		assert.Equal(t, float64(1), metric.GetCounter().GetValue())
		labels := map[string]string{}
		for _, lp := range metric.GetLabel() {
			labels[lp.GetName()] = lp.GetValue()
		}
		assert.Equal(t, "test-provider", labels["provider"])
		assert.Equal(t, "test-model", labels["model"])
		assert.Equal(t, chaterror.KindRateLimit, labels["kind"])
	}
	assert.True(t, found, "stream_retries_total metric not found")
}

// TestRun_StreamRetry_CanceledDoesNotIncrement pins the invariant
// that canceled streams never increment stream_retries_total.
// chaterror.Classify routes context.Canceled to
// ClassifiedError{Retryable: false}, so chatretry.Retry returns
// immediately without calling onRetry. This test guards against
// future classification changes that could silently introduce
// misleading retry samples.
func TestRun_StreamRetry_CanceledDoesNotIncrement(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	metrics := chatloop.NewMetrics(reg)

	model := &chattest.FakeModel{
		ProviderName: "test-provider",
		ModelName:    "test-model",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			return nil, context.Canceled
		},
	}

	err := chatloop.Run(context.Background(), chatloop.RunOptions{
		Model:                model,
		MaxSteps:             1,
		ContextLimitFallback: 4096,
		PersistStep: func(_ context.Context, _ chatloop.PersistedStep) error {
			return nil
		},
		Metrics: metrics,
	})
	// Expect an error (the stream failed); we don't care which error
	// kind as long as no retry was recorded.
	require.Error(t, err)

	families, err := reg.Gather()
	require.NoError(t, err)

	for _, f := range families {
		if f.GetName() == "coderd_chatd_stream_retries_total" {
			assert.Empty(t, f.GetMetric(),
				"stream_retries_total should have no samples after a canceled stream")
		}
	}
}
