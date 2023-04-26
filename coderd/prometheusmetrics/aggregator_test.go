package prometheusmetrics_test

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd/prometheusmetrics"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/testutil"
)

const (
	testWorkspaceName = "yogi-workspace"
	testUsername      = "yogi-bear"
	testAgentName     = "main-agent"
)

func TestUpdateMetrics_MetricsDoNotExpire(t *testing.T) {
	t.Parallel()

	// given
	registry := prometheus.NewRegistry()
	metricsAggregator, err := prometheusmetrics.NewMetricsAggregator(slogtest.Make(t, &slogtest.Options{
		IgnoreErrors: true,
	}), registry, time.Hour) // time.Hour, so metrics won't expire
	require.NoError(t, err)

	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)

	closeFunc := metricsAggregator.Run(ctx)
	t.Cleanup(closeFunc)

	given := []agentsdk.AgentMetric{
		{
			Name:  "a_counter_one",
			Type:  agentsdk.AgentMetricTypeCounter,
			Value: 1,
		},
		{
			Name:  "b_counter_two",
			Type:  agentsdk.AgentMetricTypeCounter,
			Value: 2,
		},
		{
			Name:  "c_gauge_three",
			Type:  agentsdk.AgentMetricTypeCounter,
			Value: 3,
		},
		{
			Name:  "d_gauge_four",
			Type:  agentsdk.AgentMetricTypeCounter,
			Value: 4,
		},
	}

	// when
	metricsAggregator.Update(ctx, testUsername, testWorkspaceName, testAgentName, given)

	// then
	require.Eventually(t, func() bool {
		var actual []prometheus.Metric
		metricsCh := make(chan prometheus.Metric)
		go func() {
			for m := range metricsCh {
				actual = append(actual, m)
			}
		}()
		metricsAggregator.Collect(metricsCh)
		return verifyCollectedMetrics(t, given, actual)
	}, testutil.WaitMedium, testutil.IntervalFast)
}

func verifyCollectedMetrics(t *testing.T, expected []agentsdk.AgentMetric, actual []prometheus.Metric) bool {
	if len(expected) != len(actual) {
		return false
	}

	// Metrics are expected to arrive in order
	for i, e := range expected {
		desc := actual[i].Desc()
		assert.Contains(t, desc.String(), e.Name)

		var d dto.Metric
		err := actual[i].Write(&d)
		require.NoError(t, err)

		require.Equal(t, "agent_name", *d.Label[0].Name)
		require.Equal(t, testAgentName, *d.Label[0].Value)
		require.Equal(t, "username", *d.Label[1].Name)
		require.Equal(t, testUsername, *d.Label[1].Value)
		require.Equal(t, "workspace_name", *d.Label[2].Name)
		require.Equal(t, testWorkspaceName, *d.Label[2].Value)

		if e.Type == agentsdk.AgentMetricTypeCounter {
			require.Equal(t, e.Value, *d.Counter.Value)
		} else if e.Type == agentsdk.AgentMetricTypeGauge {
			require.Equal(t, e.Value, *d.Gauge.Value)
		} else {
			require.Failf(t, "unsupported type: %s", string(e.Type))
		}
	}
	return true
}
