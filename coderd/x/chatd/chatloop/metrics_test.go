package chatloop_test

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
)

func TestNewMetrics_RegistersAllMetrics(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := chatloop.NewMetrics(reg)

	// Initialize vector metrics so they appear in Gather output.
	m.Chats.WithLabelValues(chatloop.StateStreaming)
	m.CompactionTotal.WithLabelValues("anthropic", "claude-sonnet-4-5", chatloop.CompactionResultSuccess)
	m.ToolResultSizeBytes.WithLabelValues("anthropic", "claude-sonnet-4-5", "test")
	m.ToolErrorsTotal.WithLabelValues("anthropic", "claude-sonnet-4-5", "test")
	m.MessageCount.WithLabelValues("anthropic", "claude-sonnet-4-5")
	m.PromptSizeBytes.WithLabelValues("anthropic", "claude-sonnet-4-5")
	m.TTFTSeconds.WithLabelValues("anthropic", "claude-sonnet-4-5")
	m.StepsTotal.WithLabelValues("anthropic", "claude-sonnet-4-5")
	m.StreamRetriesTotal.WithLabelValues("anthropic", "claude-sonnet-4-5", string(codersdk.ChatErrorKindTimeout))
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
		"coderd_chatd_tool_errors_total":           dto.MetricType_COUNTER,
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
	m.ToolErrorsTotal.WithLabelValues("anthropic", "claude-sonnet-4-5", "execute").Inc()
	m.TTFTSeconds.WithLabelValues("anthropic", "claude-sonnet-4-5").Observe(0.5)
	m.CompactionTotal.WithLabelValues("anthropic", "claude-sonnet-4-5", "success").Inc()
	m.CompactionTotal.WithLabelValues("openai", "gpt-5", "error").Inc()
	m.CompactionTotal.WithLabelValues("google", "gemini-2.5-pro", "timeout").Inc()
	m.StepsTotal.WithLabelValues("anthropic", "claude-sonnet-4-5").Inc()
	m.StreamRetriesTotal.WithLabelValues("anthropic", "claude-sonnet-4-5", string(codersdk.ChatErrorKindTimeout)).Inc()
	m.StreamBufferDroppedTotal.Inc()

	// Nil-receiver guard for RecordStreamRetry and
	// RecordStreamBufferDropped mirrors the existing RecordCompaction nil
	// guard.
	var nilMetrics *chatloop.Metrics
	nilMetrics.RecordStreamRetry("anthropic", "claude-sonnet-4-5", chaterror.ClassifiedError{Kind: codersdk.ChatErrorKindTimeout})
	nilMetrics.RecordStreamBufferDropped()
	nilMetrics.RecordToolError("anthropic", "claude-sonnet-4-5", "test")
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

			requireCounter(t, reg, "coderd_chatd_compaction_total", float64(tt.wantCount), map[string]string{
				"provider": "test-provider",
				"model":    "test-model",
				"result":   tt.wantLabel,
			})
		})
	}
}

func TestRecordStreamRetry(t *testing.T) {
	t.Parallel()

	// One row per ChatErrorKind constant. Production callers always
	// reach RecordStreamRetry through chaterror.Classify, which
	// guarantees Kind is non-empty, so no empty-string case is
	// needed.
	tests := []struct {
		name string
		kind codersdk.ChatErrorKind
	}{
		{name: "overloaded", kind: codersdk.ChatErrorKindOverloaded},
		{name: "rate_limit", kind: codersdk.ChatErrorKindRateLimit},
		{name: "timeout", kind: codersdk.ChatErrorKindTimeout},
		{name: "stream_silence_timeout", kind: codersdk.ChatErrorKindStreamSilenceTimeout},
		{name: "auth", kind: codersdk.ChatErrorKindAuth},
		{name: "config", kind: codersdk.ChatErrorKindConfig},
		{name: "missing_key", kind: codersdk.ChatErrorKindMissingKey},
		{name: "generic", kind: codersdk.ChatErrorKindGeneric},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reg := prometheus.NewRegistry()
			m := chatloop.NewMetrics(reg)
			m.RecordStreamRetry("test-provider", "test-model", chaterror.ClassifiedError{
				Kind: tt.kind,
			})

			requireCounter(t, reg, "coderd_chatd_stream_retries_total", 1, map[string]string{
				"provider": "test-provider",
				"model":    "test-model",
				"kind":     string(tt.kind),
			})
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

// requireCounter gathers metrics from reg, finds the named counter
// family, and asserts it has exactly one series with the given value
// and labels.
func requireCounter(t *testing.T, reg *prometheus.Registry, name string, wantValue float64, wantLabels map[string]string) {
	t.Helper()

	families, err := reg.Gather()
	require.NoError(t, err)

	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		require.Len(t, f.GetMetric(), 1, "expected exactly one series for %s", name)
		metric := f.GetMetric()[0]
		assert.Equal(t, wantValue, metric.GetCounter().GetValue(), "counter value for %s", name)
		labels := map[string]string{}
		for _, lp := range metric.GetLabel() {
			labels[lp.GetName()] = lp.GetValue()
		}
		for k, v := range wantLabels {
			assert.Equal(t, v, labels[k], "label %s for %s", k, name)
		}
		return
	}
	t.Fatalf("metric %s not found in gathered families", name)
}

func TestRecordToolError(t *testing.T) {
	t.Parallel()

	t.Run("nil metrics does not panic", func(t *testing.T) {
		t.Parallel()
		var m *chatloop.Metrics
		m.RecordToolError("anthropic", "claude-sonnet-4-5", "test")
	})

	t.Run("increments with correct labels", func(t *testing.T) {
		t.Parallel()

		reg := prometheus.NewRegistry()
		m := chatloop.NewMetrics(reg)
		m.RecordToolError("test-provider", "test-model", "read_file")

		requireCounter(t, reg, "coderd_chatd_tool_errors_total", 1, map[string]string{
			"provider":  "test-provider",
			"model":     "test-model",
			"tool_name": "read_file",
		})
	})
}

func TestGenerateAssistant_StreamRetryRecordsMetric(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	metrics := chatloop.NewMetrics(reg)

	calls := 0
	model := &chattest.FakeModel{
		ProviderName: "test-provider",
		ModelName:    "test-model",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			calls++
			if calls == 1 {
				return nil, chaterror.WithClassification(
					xerrors.New("received status 429 from upstream"),
					chaterror.ClassifiedError{
						Kind:      codersdk.ChatErrorKindRateLimit,
						Provider:  "test-provider",
						Retryable: true,
					},
				)
			}
			return func(yield func(fantasy.StreamPart) bool) {
				yield(fantasy.StreamPart{
					Type:         fantasy.StreamPartTypeFinish,
					FinishReason: fantasy.FinishReasonStop,
				})
			}, nil
		},
	}

	_, err := chatloop.GenerateAssistant(context.Background(), chatloop.GenerateAssistantOptions{
		Model: model,
		// ErrorProvider diverges from the transport provider. Error copy must
		// use it while the retry metric stays on the transport provider.
		ErrorProvider: "bedrock",
		Metrics:       metrics,
	})
	require.Error(t, err)
	require.Equal(t, 1, calls)
	// Error classification uses the configured provider.
	require.Equal(t, "bedrock", chaterror.Classify(err).Provider)
	// Retry metric keeps the transport provider label, not "bedrock".
	requireCounter(t, reg, "coderd_chatd_stream_retries_total", 1, map[string]string{
		"provider": "test-provider",
		"model":    "test-model",
		"kind":     string(codersdk.ChatErrorKindRateLimit),
	})
}

// TestGenerateAssistant_StreamRetry_ContextCanceledTransportResetIncrements pins the
// invariant that provider-originated context cancellation is counted as a
// retryable transport reset when the chat context is still alive.
func TestGenerateAssistant_StreamRetry_ContextCanceledTransportResetIncrements(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	metrics := chatloop.NewMetrics(reg)

	attempts := 0
	model := &chattest.FakeModel{
		ProviderName: "test-provider",
		ModelName:    "test-model",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			attempts++
			return nil, context.Canceled
		},
	}

	_, err := chatloop.GenerateAssistant(context.Background(), chatloop.GenerateAssistantOptions{
		Model:   model,
		Metrics: metrics,
	})
	require.Error(t, err)
	require.Equal(t, 1, attempts)

	requireCounter(t, reg, "coderd_chatd_stream_retries_total", 1, map[string]string{
		"provider": "test-provider",
		"model":    "test-model",
		"kind":     string(codersdk.ChatErrorKindTimeout),
	})
}
