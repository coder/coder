package prometheusmetrics_test

import (
	"context"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/prometheusmetrics"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/testutil"
)

const (
	testWorkspaceName = "yogi-workspace"
	testUsername      = "yogi-bear"
	testAgentName     = "main-agent"
	testTemplateName  = "main-template"
)

var testLabels = prometheusmetrics.AgentMetricLabels{
	Username:      testUsername,
	WorkspaceName: testWorkspaceName,
	AgentName:     testAgentName,
	TemplateName:  testTemplateName,
}

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
		{Name: "template_name", Value: testTemplateName},
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
			{Name: "template_name", Value: testTemplateName},
		}},
		{Name: "d_gauge_four", Type: agentsdk.AgentMetricTypeGauge, Value: 6, Labels: commonLabels},
	}

	// when
	metricsAggregator.Update(ctx, testLabels, given1)
	metricsAggregator.Update(ctx, testLabels, given2)

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
		// dto labels are sorted in alphabetical order.
		sort.Slice(e.Labels, func(i, j int) bool {
			return e.Labels[i].Name < e.Labels[j].Name
		})
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
	metricsAggregator.Update(ctx, testLabels, given)

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

func Benchmark_MetricsAggregator_Run(b *testing.B) {
	// Number of metrics to generate and send in each iteration.
	// Hard-coded to 1024 to avoid overflowing the queue in the metrics aggregator.
	numMetrics := 1024

	// given
	registry := prometheus.NewRegistry()
	metricsAggregator := must(prometheusmetrics.NewMetricsAggregator(
		slogtest.Make(b, &slogtest.Options{IgnoreErrors: true}),
		registry,
		time.Hour,
	))

	ctx, cancelFunc := context.WithCancel(context.Background())
	b.Cleanup(cancelFunc)

	closeFunc := metricsAggregator.Run(ctx)
	b.Cleanup(closeFunc)

	ch := make(chan prometheus.Metric)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				metricsAggregator.Collect(ch)
			}
		}
	}()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		b.Logf("N=%d generating %d metrics", b.N, numMetrics)
		metrics := make([]agentsdk.AgentMetric, 0, numMetrics)
		for i := 0; i < numMetrics; i++ {
			metrics = append(metrics, genAgentMetric(b))
		}

		b.Logf("N=%d sending %d metrics", b.N, numMetrics)
		var nGot atomic.Int64
		b.StartTimer()
		metricsAggregator.Update(ctx, testLabels, metrics)
		for i := 0; i < numMetrics; i++ {
			select {
			case <-ctx.Done():
				b.FailNow()
			case <-ch:
				nGot.Add(1)
			}
		}
		b.StopTimer()
		b.Logf("N=%d got %d metrics", b.N, nGot.Load())
	}
}

func genAgentMetric(t testing.TB) agentsdk.AgentMetric {
	t.Helper()

	var metricType agentsdk.AgentMetricType
	if must(cryptorand.Float64()) >= 0.5 {
		metricType = agentsdk.AgentMetricTypeCounter
	} else {
		metricType = agentsdk.AgentMetricTypeGauge
	}

	// Ensure that metric name does not start or end with underscore, as it is not allowed by Prometheus.
	metricName := "metric_" + must(cryptorand.StringCharset(cryptorand.Alpha, 80)) + "_gen"
	// Generate random metric value between 0 and 1000.
	metricValue := must(cryptorand.Float64()) * float64(must(cryptorand.Intn(1000)))

	return agentsdk.AgentMetric{
		Name: metricName, Type: metricType, Value: metricValue, Labels: []agentsdk.AgentMetricLabel{},
	}
}

func must[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}
	return t
}
