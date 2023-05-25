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
	metricsAggregator, err := prometheusmetrics.NewMetricsAggregator(slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), registry, time.Hour) // time.Hour, so metrics won't expire
	require.NoError(t, err)

	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)

	closeFunc := metricsAggregator.Run(ctx)
	t.Cleanup(closeFunc)

	given1 := []agentsdk.AgentMetric{
		{Name: "a_counter_one", Type: agentsdk.AgentMetricTypeCounter, Value: 1},
		{Name: "b_counter_two", Type: agentsdk.AgentMetricTypeCounter, Value: 2},
		{Name: "c_gauge_three", Type: agentsdk.AgentMetricTypeGauge, Value: 3},
	}

	given2 := []agentsdk.AgentMetric{
		{Name: "b_counter_two", Type: agentsdk.AgentMetricTypeCounter, Value: 4},
		{Name: "c_gauge_three", Type: agentsdk.AgentMetricTypeGauge, Value: 5},
		{Name: "c_gauge_three", Type: agentsdk.AgentMetricTypeGauge, Value: 2, Labels: []agentsdk.AgentMetricLabel{
			{Name: "foobar", Value: "Foobaz"},
			{Name: "hello", Value: "world"},
		}},
		{Name: "d_gauge_four", Type: agentsdk.AgentMetricTypeGauge, Value: 6},
	}

	commonLabels := []agentsdk.AgentMetricLabel{
		{Name: "agent_name", Value: testAgentName},
		{Name: "username", Value: testUsername},
		{Name: "workspace_name", Value: testWorkspaceName},
	}
	expected := []agentsdk.AgentMetric{
		{Name: "a_counter_one", Type: agentsdk.AgentMetricTypeCounter, Value: 1, Labels: commonLabels},
		{Name: "b_counter_two", Type: agentsdk.AgentMetricTypeCounter, Value: 4, Labels: commonLabels},
		{Name: "c_gauge_three", Type: agentsdk.AgentMetricTypeGauge, Value: 5, Labels: commonLabels},
		{Name: "c_gauge_three", Type: agentsdk.AgentMetricTypeGauge, Value: 2, Labels: []agentsdk.AgentMetricLabel{
			{Name: "agent_name", Value: testAgentName},
			{Name: "foobar", Value: "Foobaz"},
			{Name: "hello", Value: "world"},
			{Name: "username", Value: testUsername},
			{Name: "workspace_name", Value: testWorkspaceName},
		}},
		{Name: "d_gauge_four", Type: agentsdk.AgentMetricTypeGauge, Value: 6, Labels: commonLabels},
	}

	// when
	metricsAggregator.Update(ctx, testUsername, testWorkspaceName, testAgentName, given1)
	metricsAggregator.Update(ctx, testUsername, testWorkspaceName, testAgentName, given2)

	// then
	require.Eventually(t, func() bool {
		var actual []prometheus.Metric
		metricsCh := make(chan prometheus.Metric)

		done := make(chan struct{}, 1)
		defer close(done)
		go func() {
			for m := range metricsCh {
				actual = append(actual, m)
			}
			done <- struct{}{}
		}()
		metricsAggregator.Collect(metricsCh)
		close(metricsCh)
		<-done
		return verifyCollectedMetrics(t, expected, actual)
	}, testutil.WaitMedium, testutil.IntervalSlow)
}

func verifyCollectedMetrics(t *testing.T, expected []agentsdk.AgentMetric, actual []prometheus.Metric) bool {
	if len(expected) != len(actual) {
		return false
	}

	for i, e := range expected {
		desc := actual[i].Desc()
		assert.Contains(t, desc.String(), e.Name)

		var d dto.Metric
		err := actual[i].Write(&d)
		require.NoError(t, err)

		if e.Type == agentsdk.AgentMetricTypeCounter {
			require.Equal(t, e.Value, d.Counter.GetValue())
		} else if e.Type == agentsdk.AgentMetricTypeGauge {
			require.Equal(t, e.Value, d.Gauge.GetValue())
		} else {
			require.Failf(t, "unsupported type: %s", string(e.Type))
		}

		dtoLabels := asMetricAgentLabels(d.GetLabel())
		require.Equal(t, e.Labels, dtoLabels, d.String())
	}
	return true
}

func asMetricAgentLabels(dtoLabels []*dto.LabelPair) []agentsdk.AgentMetricLabel {
	metricLabels := make([]agentsdk.AgentMetricLabel, 0, len(dtoLabels))
	for _, dtoLabel := range dtoLabels {
		metricLabels = append(metricLabels, agentsdk.AgentMetricLabel{
			Name:  dtoLabel.GetName(),
			Value: dtoLabel.GetValue(),
		})
	}
	return metricLabels
}

func TestUpdateMetrics_MetricsExpire(t *testing.T) {
	t.Parallel()

	// given
	registry := prometheus.NewRegistry()
	metricsAggregator, err := prometheusmetrics.NewMetricsAggregator(slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), registry, time.Millisecond)
	require.NoError(t, err)

	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)

	closeFunc := metricsAggregator.Run(ctx)
	t.Cleanup(closeFunc)

	given := []agentsdk.AgentMetric{
		{Name: "a_counter_one", Type: agentsdk.AgentMetricTypeCounter, Value: 1},
	}

	// when
	metricsAggregator.Update(ctx, testUsername, testWorkspaceName, testAgentName, given)

	time.Sleep(time.Millisecond * 10) // Ensure that metric is expired

	// then
	require.Eventually(t, func() bool {
		var actual []prometheus.Metric
		metricsCh := make(chan prometheus.Metric)

		done := make(chan struct{}, 1)
		defer close(done)
		go func() {
			for m := range metricsCh {
				actual = append(actual, m)
			}
			done <- struct{}{}
		}()
		metricsAggregator.Collect(metricsCh)
		close(metricsCh)
		<-done
		return len(actual) == 0
	}, testutil.WaitShort, testutil.IntervalFast)
}
