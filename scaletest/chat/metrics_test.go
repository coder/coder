package chat_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/scaletest/chat"
)

func TestNewMetricsUsesChatSpecificBuckets(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	metrics := chat.NewMetrics(reg, chat.MetricLabelNames()...)
	baseLabels := chat.MetricLabelValues("run-123")
	phaseLabels := append(append([]string{}, baseLabels...), "initial")

	metrics.ChatCreateLatencySeconds.WithLabelValues(baseLabels...).Observe(15)
	metrics.ChatMessageLatencySeconds.WithLabelValues(phaseLabels...).Observe(15)
	metrics.ChatConversationDurationSeconds.WithLabelValues(baseLabels...).Observe(200)
	metrics.ChatStageFailuresTotal.WithLabelValues(append(append([]string{}, baseLabels...), "create_chat")...).Inc()

	families, err := reg.Gather()
	require.NoError(t, err)

	createFamily := findMetricFamily(t, families, "coderd_scaletest_chat_create_latency_seconds")
	require.GreaterOrEqual(t, lastBucketUpperBound(t, createFamily), 120.0)

	messageFamily := findMetricFamily(t, families, "coderd_scaletest_chat_message_latency_seconds")
	require.GreaterOrEqual(t, lastBucketUpperBound(t, messageFamily), 120.0)

	conversationFamily := findMetricFamily(t, families, "coderd_scaletest_chat_conversation_duration_seconds")
	require.InDelta(t, 300.0, lastBucketUpperBound(t, conversationFamily), 0.01)

	findMetricFamily(t, families, "coderd_scaletest_chat_stage_failures_total")
}

func findMetricFamily(t *testing.T, families []*dto.MetricFamily, name string) *dto.MetricFamily {
	t.Helper()
	for _, family := range families {
		if family.GetName() == name {
			return family
		}
	}
	t.Fatalf("metric family %q not found", name)
	return nil
}

func lastBucketUpperBound(t *testing.T, family *dto.MetricFamily) float64 {
	t.Helper()
	require.NotEmpty(t, family.GetMetric())
	histogram := family.GetMetric()[0].GetHistogram()
	require.NotNil(t, histogram)
	require.NotEmpty(t, histogram.GetBucket())
	return histogram.GetBucket()[len(histogram.GetBucket())-1].GetUpperBound()
}
