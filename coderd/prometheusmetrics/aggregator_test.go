package prometheusmetrics_test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/agentmetrics"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/prometheusmetrics"
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
	metricsAggregator, err := prometheusmetrics.NewMetricsAggregator(slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), registry, time.Hour, nil) // time.Hour, so metrics won't expire
	require.NoError(t, err)

	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)

	closeFunc := metricsAggregator.Run(ctx)
	t.Cleanup(closeFunc)

	given1 := []*agentproto.Stats_Metric{
		{Name: "a_counter_one", Type: agentproto.Stats_Metric_COUNTER, Value: 1},
		{Name: "b_counter_two", Type: agentproto.Stats_Metric_COUNTER, Value: 2},
		// Tests that we update labels correctly when they have extra labels
		{Name: "b_counter_two", Type: agentproto.Stats_Metric_COUNTER, Value: 27, Labels: []*agentproto.Stats_Metric_Label{
			{Name: "lizz", Value: "rizz"},
		}},
		{Name: "c_gauge_three", Type: agentproto.Stats_Metric_GAUGE, Value: 3},
	}

	given2 := []*agentproto.Stats_Metric{
		{Name: "b_counter_two", Type: agentproto.Stats_Metric_COUNTER, Value: 4},
		// Tests that we update labels correctly when they have extra labels
		{Name: "b_counter_two", Type: agentproto.Stats_Metric_COUNTER, Value: -9, Labels: []*agentproto.Stats_Metric_Label{
			{Name: "lizz", Value: "rizz"},
		}},
		{Name: "c_gauge_three", Type: agentproto.Stats_Metric_GAUGE, Value: 5},
		{Name: "c_gauge_three", Type: agentproto.Stats_Metric_GAUGE, Value: 2, Labels: []*agentproto.Stats_Metric_Label{
			{Name: "foobar", Value: "Foobaz"},
			{Name: "hello", Value: "world"},
		}},
		{Name: "d_gauge_four", Type: agentproto.Stats_Metric_GAUGE, Value: 6},
		{Name: "e_gauge_four", Type: agentproto.Stats_Metric_GAUGE, Value: 15, Labels: []*agentproto.Stats_Metric_Label{
			{Name: "foobar", Value: "Foo,ba=z"},
			{Name: "halo", Value: "wor\\,d=1,e=\\,2"},
			{Name: "hello", Value: "wo,,r=d"},
		}},
		{Name: "f_gauge_four", Type: agentproto.Stats_Metric_GAUGE, Value: 6, Labels: []*agentproto.Stats_Metric_Label{
			{Name: "empty", Value: ""},
			{Name: "foobar", Value: "foobaz"},
		}},
	}

	given3 := []*agentproto.Stats_Metric{
		{Name: "e_gauge_four", Type: agentproto.Stats_Metric_GAUGE, Value: 17, Labels: []*agentproto.Stats_Metric_Label{
			{Name: "cat", Value: "do,=g"},
			{Name: "hello", Value: "wo,,rld"},
		}},
		{Name: "f_gauge_four", Type: agentproto.Stats_Metric_GAUGE, Value: 8, Labels: []*agentproto.Stats_Metric_Label{
			{Name: "foobar", Value: "foobaz"},
		}},
	}

	commonLabels := []*agentproto.Stats_Metric_Label{
		{Name: agentmetrics.LabelAgentName, Value: testAgentName},
		{Name: agentmetrics.LabelUsername, Value: testUsername},
		{Name: agentmetrics.LabelWorkspaceName, Value: testWorkspaceName},
		{Name: agentmetrics.LabelTemplateName, Value: testTemplateName},
	}
	expected := []*agentproto.Stats_Metric{
		{Name: "a_counter_one", Type: agentproto.Stats_Metric_COUNTER, Value: 1, Labels: commonLabels},
		{Name: "b_counter_two", Type: agentproto.Stats_Metric_COUNTER, Value: -9, Labels: []*agentproto.Stats_Metric_Label{
			{Name: agentmetrics.LabelAgentName, Value: testAgentName},
			{Name: "lizz", Value: "rizz"},
			{Name: agentmetrics.LabelUsername, Value: testUsername},
			{Name: agentmetrics.LabelWorkspaceName, Value: testWorkspaceName},
			{Name: agentmetrics.LabelTemplateName, Value: testTemplateName},
		}},
		{Name: "b_counter_two", Type: agentproto.Stats_Metric_COUNTER, Value: 4, Labels: commonLabels},
		{Name: "c_gauge_three", Type: agentproto.Stats_Metric_GAUGE, Value: 2, Labels: []*agentproto.Stats_Metric_Label{
			{Name: agentmetrics.LabelAgentName, Value: testAgentName},
			{Name: "foobar", Value: "Foobaz"},
			{Name: "hello", Value: "world"},
			{Name: agentmetrics.LabelUsername, Value: testUsername},
			{Name: agentmetrics.LabelWorkspaceName, Value: testWorkspaceName},
			{Name: agentmetrics.LabelTemplateName, Value: testTemplateName},
		}},
		{Name: "c_gauge_three", Type: agentproto.Stats_Metric_GAUGE, Value: 5, Labels: commonLabels},
		{Name: "d_gauge_four", Type: agentproto.Stats_Metric_GAUGE, Value: 6, Labels: commonLabels},
		{Name: "e_gauge_four", Type: agentproto.Stats_Metric_GAUGE, Value: 17, Labels: []*agentproto.Stats_Metric_Label{
			{Name: agentmetrics.LabelAgentName, Value: testAgentName},
			{Name: "cat", Value: "do,=g"},
			{Name: "hello", Value: "wo,,rld"},
			{Name: agentmetrics.LabelUsername, Value: testUsername},
			{Name: agentmetrics.LabelWorkspaceName, Value: testWorkspaceName},
			{Name: agentmetrics.LabelTemplateName, Value: testTemplateName},
		}},
		{Name: "e_gauge_four", Type: agentproto.Stats_Metric_GAUGE, Value: 15, Labels: []*agentproto.Stats_Metric_Label{
			{Name: agentmetrics.LabelAgentName, Value: testAgentName},
			{Name: "foobar", Value: "Foo,ba=z"},
			{Name: "halo", Value: "wor\\,d=1,e=\\,2"},
			{Name: "hello", Value: "wo,,r=d"},
			{Name: agentmetrics.LabelUsername, Value: testUsername},
			{Name: agentmetrics.LabelWorkspaceName, Value: testWorkspaceName},
			{Name: agentmetrics.LabelTemplateName, Value: testTemplateName},
		}},
		{Name: "f_gauge_four", Type: agentproto.Stats_Metric_GAUGE, Value: 8, Labels: []*agentproto.Stats_Metric_Label{
			{Name: agentmetrics.LabelAgentName, Value: testAgentName},
			{Name: "foobar", Value: "foobaz"},
			{Name: agentmetrics.LabelUsername, Value: testUsername},
			{Name: agentmetrics.LabelWorkspaceName, Value: testWorkspaceName},
			{Name: agentmetrics.LabelTemplateName, Value: testTemplateName},
		}},
	}

	// when
	metricsAggregator.Update(ctx, testLabels, given1)
	metricsAggregator.Update(ctx, testLabels, given2)
	metricsAggregator.Update(ctx, testLabels, given3)

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

func verifyCollectedMetrics(t *testing.T, expected []*agentproto.Stats_Metric, actual []prometheus.Metric) bool {
	if len(expected) != len(actual) {
		t.Logf("expected %d metrics, got %d", len(expected), len(actual))
		return false
	}

	// ensure stable iteration order
	sort.Slice(expected, func(i, j int) bool {
		return expected[i].Name < expected[j].Name
	})

	sort.Slice(actual, func(i, j int) bool {
		m1 := prometheusMetricToString(t, actual[i])
		m2 := prometheusMetricToString(t, actual[j])
		return m1 < m2
	})

	for i, e := range expected {
		desc := actual[i].Desc()
		assert.Contains(t, desc.String(), e.Name)

		var d dto.Metric
		err := actual[i].Write(&d)
		require.NoError(t, err)

		if e.Type == agentproto.Stats_Metric_COUNTER {
			require.Equal(t, e.Value, d.Counter.GetValue())
		} else if e.Type == agentproto.Stats_Metric_GAUGE {
			require.Equal(t, e.Value, d.Gauge.GetValue())
		} else {
			require.Failf(t, "unsupported type: %s", string(e.Type))
		}

		dtoLabels := asMetricAgentLabels(d.GetLabel())
		// dto labels are sorted in alphabetical order.
		sortFn := func(i, j int) bool {
			return e.Labels[i].Name < e.Labels[j].Name
		}
		sort.Slice(e.Labels, sortFn)
		sort.Slice(dtoLabels, sortFn)
		require.Equal(t, e.Labels, dtoLabels, d.String())
	}
	return true
}

func prometheusMetricToString(t *testing.T, m prometheus.Metric) string {
	var sb strings.Builder

	desc := m.Desc()
	_, _ = sb.WriteString(desc.String())
	_ = sb.WriteByte('|')

	var d dto.Metric
	err := m.Write(&d)
	require.NoError(t, err)
	dtoLabels := asMetricAgentLabels(d.GetLabel())
	sort.Slice(dtoLabels, func(i, j int) bool {
		return dtoLabels[i].Name < dtoLabels[j].Name
	})

	for _, dtoLabel := range dtoLabels {
		if dtoLabel.Value == "" {
			continue
		}
		_, _ = sb.WriteString(dtoLabel.Name)
		_ = sb.WriteByte('=')
		_, _ = sb.WriteString(prometheusmetrics.MetricLabelValueEncoder.Replace(dtoLabel.Value))
	}
	return strings.TrimRight(sb.String(), ",")
}

func asMetricAgentLabels(dtoLabels []*dto.LabelPair) []*agentproto.Stats_Metric_Label {
	metricLabels := make([]*agentproto.Stats_Metric_Label, 0, len(dtoLabels))
	for _, dtoLabel := range dtoLabels {
		if dtoLabel.GetValue() == "" {
			continue
		}

		metricLabels = append(metricLabels, &agentproto.Stats_Metric_Label{
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
	metricsAggregator, err := prometheusmetrics.NewMetricsAggregator(slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), registry, time.Millisecond, nil)
	require.NoError(t, err)

	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)

	closeFunc := metricsAggregator.Run(ctx)
	t.Cleanup(closeFunc)

	given := []*agentproto.Stats_Metric{
		{Name: "a_counter_one", Type: agentproto.Stats_Metric_COUNTER, Value: 1},
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

func TestLabelsAggregation(t *testing.T) {
	t.Parallel()

	type statCollection struct {
		labels  prometheusmetrics.AgentMetricLabels
		metrics []*agentproto.Stats_Metric
	}

	commonLabels := []*agentproto.Stats_Metric_Label{
		{Name: agentmetrics.LabelUsername, Value: testUsername},
		{Name: agentmetrics.LabelAgentName, Value: testAgentName},
		{Name: agentmetrics.LabelWorkspaceName, Value: testWorkspaceName},
		{Name: agentmetrics.LabelTemplateName, Value: testTemplateName},
	}

	tests := []struct {
		name        string
		given       []statCollection
		expected    []*agentproto.Stats_Metric
		aggregateOn []string
	}{
		{
			name:        "label aggregations not specified, keep all (high cardinality, default behavior)",
			aggregateOn: agentmetrics.LabelAll,
			given: []statCollection{
				{
					labels: testLabels,
					metrics: []*agentproto.Stats_Metric{
						{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 1},
					},
				},
				{
					labels: testLabels,
					metrics: []*agentproto.Stats_Metric{
						{Name: "active_conns", Type: agentproto.Stats_Metric_GAUGE, Value: 4},
					},
				},
			},
			expected: []*agentproto.Stats_Metric{
				{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 1, Labels: commonLabels},
				{Name: "active_conns", Type: agentproto.Stats_Metric_GAUGE, Value: 4, Labels: commonLabels},
			},
		},
		{
			// Scenario: 2 users are using the same agent and we've configured the deployment to aggregate on the "agent_name" label.
			name:        "single label aggregation, aggregating to single metric",
			aggregateOn: []string{agentmetrics.LabelAgentName},
			given: []statCollection{
				{
					labels: prometheusmetrics.AgentMetricLabels{
						Username:  "user1",
						AgentName: "agent1",
					},
					metrics: []*agentproto.Stats_Metric{
						{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 1},
					},
				},
				{
					labels: prometheusmetrics.AgentMetricLabels{
						Username:  "user2",
						AgentName: "agent1",
					},
					metrics: []*agentproto.Stats_Metric{
						{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 7},
					},
				},
			},
			expected: []*agentproto.Stats_Metric{
				// We only observed one agent_name value, so all metrics are aggregated to a single series.
				{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 8, Labels: []*agentproto.Stats_Metric_Label{
					{Name: agentmetrics.LabelAgentName, Value: "agent1"},
				}},
			},
		},
		{
			// Scenario: as above, but we're aggregating on two invariant labels.
			name:        "multiple label aggregation, aggregating to single metric",
			aggregateOn: []string{agentmetrics.LabelAgentName, agentmetrics.LabelTemplateName},
			given: []statCollection{
				{
					labels: prometheusmetrics.AgentMetricLabels{
						Username:     "user1",
						AgentName:    "agent1",
						TemplateName: "template1",
					},
					metrics: []*agentproto.Stats_Metric{
						{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 1},
					},
				},
				{
					labels: prometheusmetrics.AgentMetricLabels{
						Username:     "user2",
						AgentName:    "agent1",
						TemplateName: "template1",
					},
					metrics: []*agentproto.Stats_Metric{
						{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 7},
					},
				},
			},
			expected: []*agentproto.Stats_Metric{
				// We only observed one agent_name & template_name tuple, so all metrics are aggregated to a single series.
				{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 8, Labels: []*agentproto.Stats_Metric_Label{
					{Name: agentmetrics.LabelAgentName, Value: "agent1"},
					{Name: agentmetrics.LabelTemplateName, Value: "template1"},
				}},
			},
		},
		{
			// Scenario: aggregating on a label which is unique across all metrics.
			name:        "single label aggregation, aggregating to multiple metrics",
			aggregateOn: []string{agentmetrics.LabelUsername},
			given: []statCollection{
				{
					labels: prometheusmetrics.AgentMetricLabels{
						Username:     "user1",
						AgentName:    "agent1",
						TemplateName: "template1",
					},
					metrics: []*agentproto.Stats_Metric{
						{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 1},
					},
				},
				{
					labels: prometheusmetrics.AgentMetricLabels{
						Username:     "user2",
						AgentName:    "agent1",
						TemplateName: "template1",
					},
					metrics: []*agentproto.Stats_Metric{
						{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 7},
					},
				},
			},
			expected: []*agentproto.Stats_Metric{
				// We observed two unique username values, and therefore we have a metric for each.
				{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 1, Labels: []*agentproto.Stats_Metric_Label{
					{Name: agentmetrics.LabelUsername, Value: "user1"},
				}},
				{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 7, Labels: []*agentproto.Stats_Metric_Label{
					{Name: agentmetrics.LabelUsername, Value: "user2"},
				}},
			},
		},
		{
			// Scenario: aggregating on a label which is unique across all metrics, plus two invariant labels.
			name:        "multiple label aggregation, aggregating to multiple metrics",
			aggregateOn: []string{agentmetrics.LabelUsername, agentmetrics.LabelAgentName, agentmetrics.LabelTemplateName},
			given: []statCollection{
				{
					labels: prometheusmetrics.AgentMetricLabels{
						Username:     "user1",
						AgentName:    "agent1",
						TemplateName: "template1",
					},
					metrics: []*agentproto.Stats_Metric{
						{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 1},
					},
				},
				{
					labels: prometheusmetrics.AgentMetricLabels{
						Username:     "user2",
						AgentName:    "agent1",
						TemplateName: "template1",
					},
					metrics: []*agentproto.Stats_Metric{
						{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 7},
					},
				},
			},
			expected: []*agentproto.Stats_Metric{
				// We observed two unique username values, and therefore we have a metric for each.
				{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 1, Labels: []*agentproto.Stats_Metric_Label{
					{Name: agentmetrics.LabelUsername, Value: "user1"},
					{Name: agentmetrics.LabelAgentName, Value: "agent1"},
					{Name: agentmetrics.LabelTemplateName, Value: "template1"},
				}},
				{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 7, Labels: []*agentproto.Stats_Metric_Label{
					{Name: agentmetrics.LabelUsername, Value: "user2"},
					{Name: agentmetrics.LabelAgentName, Value: "agent1"},
					{Name: agentmetrics.LabelTemplateName, Value: "template1"},
				}},
			},
		},
		{
			name:        "extra labels are retained, even with label aggregations",
			aggregateOn: []string{agentmetrics.LabelUsername},
			given: []statCollection{
				{
					labels: testLabels,
					metrics: []*agentproto.Stats_Metric{
						{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 1},
					},
				},
				{
					labels: testLabels,
					metrics: []*agentproto.Stats_Metric{
						{Name: "extra_label", Type: agentproto.Stats_Metric_COUNTER, Value: 27, Labels: []*agentproto.Stats_Metric_Label{
							{Name: "lizz", Value: "rizz"},
						}},
					},
				},
			},
			expected: []*agentproto.Stats_Metric{
				{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 1, Labels: []*agentproto.Stats_Metric_Label{
					{Name: agentmetrics.LabelUsername, Value: testUsername},
				}},
				{Name: "extra_label", Type: agentproto.Stats_Metric_COUNTER, Value: 27, Labels: []*agentproto.Stats_Metric_Label{
					{Name: "lizz", Value: "rizz"},
					{Name: agentmetrics.LabelUsername, Value: testUsername},
				}},
			},
		},
		{
			// Both counters and gauges should have all their values summed to produce the correct output.
			name:        "counters & gauges behave identically",
			aggregateOn: []string{agentmetrics.LabelTemplateName},
			given: []statCollection{
				{
					labels: prometheusmetrics.AgentMetricLabels{
						Username:     "username1",
						TemplateName: "template1",
					},
					metrics: []*agentproto.Stats_Metric{
						{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 1},
						{Name: "active_conns", Type: agentproto.Stats_Metric_GAUGE, Value: 3},
					},
				},
				{
					labels: prometheusmetrics.AgentMetricLabels{
						Username:     "username2",
						TemplateName: "template1",
					},
					metrics: []*agentproto.Stats_Metric{
						{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 2},
						{Name: "active_conns", Type: agentproto.Stats_Metric_GAUGE, Value: 4},
					},
				},
			},
			expected: []*agentproto.Stats_Metric{
				{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 3, Labels: []*agentproto.Stats_Metric_Label{
					{Name: agentmetrics.LabelTemplateName, Value: "template1"},
				}},
				{Name: "active_conns", Type: agentproto.Stats_Metric_GAUGE, Value: 7, Labels: []*agentproto.Stats_Metric_Label{
					{Name: agentmetrics.LabelTemplateName, Value: "template1"},
				}},
			},
		},
		{
			// Scenario: validation fails and an invalid label is selected for aggregation.
			name:        "invalid label aggregation",
			aggregateOn: []string{"nonsense"},
			given: []statCollection{
				{
					labels: testLabels,
					metrics: []*agentproto.Stats_Metric{
						{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 1},
					},
				},
			},
			// Nothing will be returned.
			expected: []*agentproto.Stats_Metric{},
		},
		{
			// Scenario: validation fails and an empty list is given for aggregation.
			name:        "empty label aggregation list",
			aggregateOn: []string{},
			given: []statCollection{
				{
					labels: testLabels,
					metrics: []*agentproto.Stats_Metric{
						{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 1},
					},
				},
			},
			// Default aggregation will be used.
			expected: []*agentproto.Stats_Metric{
				{Name: "user_counter", Type: agentproto.Stats_Metric_COUNTER, Value: 1, Labels: commonLabels},
			},
		},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// given
			registry := prometheus.NewRegistry()
			metricsAggregator, err := prometheusmetrics.NewMetricsAggregator(slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), registry, time.Hour, tc.aggregateOn) // time.Hour, so metrics won't expire
			require.NoError(t, err)

			ctx, cancelFunc := context.WithCancel(context.Background())
			t.Cleanup(cancelFunc)

			closeFunc := metricsAggregator.Run(ctx)
			t.Cleanup(closeFunc)

			// when
			for _, sc := range tc.given {
				metricsAggregator.Update(ctx, sc.labels, sc.metrics)
			}

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
				return verifyCollectedMetrics(t, tc.expected, actual)
			}, testutil.WaitMedium, testutil.IntervalSlow)
		})
	}
}

func Benchmark_MetricsAggregator_Run(b *testing.B) {
	benchmarkRunner(b, agentmetrics.LabelAll)
}

func Benchmark_MetricsAggregator_RunWithAggregations(b *testing.B) {
	for i := 1; i <= len(agentmetrics.LabelAll); i++ {
		b.Run(fmt.Sprintf("%d labels", i), func(b *testing.B) {
			benchmarkRunner(b, agentmetrics.LabelAll[0:i])
		})
	}
}

func benchmarkRunner(b *testing.B, aggregateByLabels []string) {
	b.ReportAllocs()

	// Number of metrics to generate and send in each iteration.
	// Hard-coded to 1024 to avoid overflowing the queue in the metrics aggregator.
	numMetrics := 1024

	// given
	registry := prometheus.NewRegistry()
	metricsAggregator := must(prometheusmetrics.NewMetricsAggregator(slogtest.Make(b, &slogtest.Options{IgnoreErrors: true}), registry, time.Hour, aggregateByLabels))

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
		metrics := make([]*agentproto.Stats_Metric, 0, numMetrics)
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

func genAgentMetric(t testing.TB) *agentproto.Stats_Metric {
	t.Helper()

	var metricType agentproto.Stats_Metric_Type
	if must(cryptorand.Float64()) >= 0.5 {
		metricType = agentproto.Stats_Metric_COUNTER
	} else {
		metricType = agentproto.Stats_Metric_GAUGE
	}

	// Ensure that metric name does not start or end with underscore, as it is not allowed by Prometheus.
	metricName := "metric_" + must(cryptorand.StringCharset(cryptorand.Alpha, 80)) + "_gen"
	// Generate random metric value between 0 and 1000.
	metricValue := must(cryptorand.Float64()) * float64(must(cryptorand.Intn(1000)))

	return &agentproto.Stats_Metric{
		Name: metricName, Type: metricType, Value: metricValue, Labels: []*agentproto.Stats_Metric_Label{},
	}
}

func must[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}
	return t
}
