package prometheusmetrics_test

import (
	"sort"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/prometheusmetrics"
)

func TestCollector_Add(t *testing.T) {
	t.Parallel()

	// given
	agentsGauge := prometheusmetrics.NewCachedGaugeVec(prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "agents",
		Name:      "up",
		Help:      "The number of active agents per workspace.",
	}, []string{"username", "workspace_name"}))

	// when
	agentsGauge.WithLabelValues(prometheusmetrics.VectorOperationAdd, 7, "first user", "my workspace")
	agentsGauge.WithLabelValues(prometheusmetrics.VectorOperationAdd, 23, "second user", "your workspace")
	agentsGauge.WithLabelValues(prometheusmetrics.VectorOperationAdd, 1, "first user", "my workspace")
	agentsGauge.WithLabelValues(prometheusmetrics.VectorOperationAdd, 25, "second user", "your workspace")
	agentsGauge.Commit()

	// then
	ch := make(chan prometheus.Metric, 2)
	agentsGauge.Collect(ch)

	metrics := collectAndSortMetrics(t, agentsGauge, 2)

	assert.Equal(t, "first user", metrics[0].Label[0].GetValue())   // Username
	assert.Equal(t, "my workspace", metrics[0].Label[1].GetValue()) // Workspace name
	assert.Equal(t, 8, int(metrics[0].Gauge.GetValue()))            // Metric value

	assert.Equal(t, "second user", metrics[1].Label[0].GetValue())    // Username
	assert.Equal(t, "your workspace", metrics[1].Label[1].GetValue()) // Workspace name
	assert.Equal(t, 48, int(metrics[1].Gauge.GetValue()))             // Metric value
}

func TestCollector_Set(t *testing.T) {
	t.Parallel()

	// given
	agentsGauge := prometheusmetrics.NewCachedGaugeVec(prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "agents",
		Name:      "up",
		Help:      "The number of active agents per workspace.",
	}, []string{"username", "workspace_name"}))

	// when
	agentsGauge.WithLabelValues(prometheusmetrics.VectorOperationSet, 3, "first user", "my workspace")
	agentsGauge.WithLabelValues(prometheusmetrics.VectorOperationSet, 4, "second user", "your workspace")
	agentsGauge.WithLabelValues(prometheusmetrics.VectorOperationSet, 5, "first user", "my workspace")
	agentsGauge.WithLabelValues(prometheusmetrics.VectorOperationSet, 6, "second user", "your workspace")
	agentsGauge.Commit()

	// then
	ch := make(chan prometheus.Metric, 2)
	agentsGauge.Collect(ch)

	metrics := collectAndSortMetrics(t, agentsGauge, 2)

	assert.Equal(t, "first user", metrics[0].Label[0].GetValue())   // Username
	assert.Equal(t, "my workspace", metrics[0].Label[1].GetValue()) // Workspace name
	assert.Equal(t, 5, int(metrics[0].Gauge.GetValue()))            // Metric value

	assert.Equal(t, "second user", metrics[1].Label[0].GetValue())    // Username
	assert.Equal(t, "your workspace", metrics[1].Label[1].GetValue()) // Workspace name
	assert.Equal(t, 6, int(metrics[1].Gauge.GetValue()))              // Metric value
}

func TestCollector_Set_Add(t *testing.T) {
	t.Parallel()

	// given
	agentsGauge := prometheusmetrics.NewCachedGaugeVec(prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "agents",
		Name:      "up",
		Help:      "The number of active agents per workspace.",
	}, []string{"username", "workspace_name"}))

	// when
	agentsGauge.WithLabelValues(prometheusmetrics.VectorOperationAdd, 9, "first user", "my workspace")
	agentsGauge.WithLabelValues(prometheusmetrics.VectorOperationAdd, 8, "second user", "your workspace")
	agentsGauge.WithLabelValues(prometheusmetrics.VectorOperationAdd, 7, "first user", "my workspace")
	agentsGauge.WithLabelValues(prometheusmetrics.VectorOperationAdd, 6, "second user", "your workspace")
	agentsGauge.WithLabelValues(prometheusmetrics.VectorOperationSet, 5, "first user", "my workspace")
	agentsGauge.WithLabelValues(prometheusmetrics.VectorOperationSet, 4, "second user", "your workspace")
	agentsGauge.WithLabelValues(prometheusmetrics.VectorOperationAdd, 3, "first user", "my workspace")
	agentsGauge.WithLabelValues(prometheusmetrics.VectorOperationAdd, 2, "second user", "your workspace")
	agentsGauge.Commit()

	// then
	ch := make(chan prometheus.Metric, 2)
	agentsGauge.Collect(ch)

	metrics := collectAndSortMetrics(t, agentsGauge, 2)

	assert.Equal(t, "first user", metrics[0].Label[0].GetValue())   // Username
	assert.Equal(t, "my workspace", metrics[0].Label[1].GetValue()) // Workspace name
	assert.Equal(t, 8, int(metrics[0].Gauge.GetValue()))            // Metric value

	assert.Equal(t, "second user", metrics[1].Label[0].GetValue())    // Username
	assert.Equal(t, "your workspace", metrics[1].Label[1].GetValue()) // Workspace name
	assert.Equal(t, 6, int(metrics[1].Gauge.GetValue()))              // Metric value
}

func collectAndSortMetrics(t *testing.T, collector prometheus.Collector, count int) []*dto.Metric {
	ch := make(chan prometheus.Metric, count)
	defer close(ch)

	var metrics []*dto.Metric

	collector.Collect(ch)
	for i := 0; i < count; i++ {
		m := <-ch

		var metric dto.Metric
		err := m.Write(&metric)
		require.NoError(t, err)

		metrics = append(metrics, &metric)
	}

	// Ensure always the same order of metrics
	sort.Slice(metrics, func(i, j int) bool {
		return sort.StringsAreSorted([]string{metrics[i].Label[0].GetValue(), metrics[j].Label[1].GetValue()})
	})
	return metrics
}
