package chatloop_test

import (
	"testing"

	"charm.land/fantasy"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
)

func TestNewMetrics_RegistersAllMetrics(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := chatloop.NewMetrics(reg)

	// Initialize vector metrics so they appear in Gather output.
	m.ActiveSessions.WithLabelValues(chatloop.StatusStreaming)
	m.CompactionTotal.WithLabelValues("anthropic", chatloop.CompactionResultSuccess)
	m.ToolResultSizeBytes.WithLabelValues("anthropic", "test")
	m.MessageCount.WithLabelValues("anthropic")
	m.PromptSizeBytes.WithLabelValues("anthropic")
	m.SerializationDurationSeconds.WithLabelValues("anthropic")
	m.StepsTotal.WithLabelValues("anthropic")

	families, err := reg.Gather()
	require.NoError(t, err)

	expected := map[string]dto.MetricType{
		"coderd_chatd_active_sessions":                dto.MetricType_GAUGE,
		"coderd_chatd_message_count":                  dto.MetricType_HISTOGRAM,
		"coderd_chatd_prompt_size_bytes":              dto.MetricType_HISTOGRAM,
		"coderd_chatd_tool_result_size_bytes":         dto.MetricType_HISTOGRAM,
		"coderd_chatd_serialization_duration_seconds": dto.MetricType_HISTOGRAM,
		"coderd_chatd_compaction_total":               dto.MetricType_COUNTER,
		"coderd_chatd_steps_total":                    dto.MetricType_COUNTER,
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
	m.ActiveSessions.WithLabelValues("streaming").Inc()
	m.ActiveSessions.WithLabelValues("streaming").Dec()
	m.ActiveSessions.WithLabelValues("waiting").Inc()
	m.ActiveSessions.WithLabelValues("waiting").Dec()
	m.MessageCount.WithLabelValues("anthropic").Observe(10)
	m.PromptSizeBytes.WithLabelValues("openai").Observe(4096)
	m.ToolResultSizeBytes.WithLabelValues("anthropic", "execute").Observe(512)
	m.SerializationDurationSeconds.WithLabelValues("anthropic").Observe(0.5)
	m.CompactionTotal.WithLabelValues("anthropic", "success").Inc()
	m.CompactionTotal.WithLabelValues("openai", "error").Inc()
	m.CompactionTotal.WithLabelValues("google", "timeout").Inc()
	m.StepsTotal.WithLabelValues("anthropic").Inc()
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
			},
		},
		{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "Hi there!"},
				fantasy.ToolCallPart{Input: `{"file":"main.go"}`},
			},
		},
	}

	size := chatloop.EstimatePromptSize(messages)
	// "You are a helpful assistant." (28) + "Hello world" (11) +
	// "Hi there!" (9) + `{"file":"main.go"}` (18) = 66
	assert.Equal(t, 66, size)
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
		m.RecordCompaction("anthropic", true, nil) // should not panic
	})
}
