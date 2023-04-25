package prometheusmetrics

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/codersdk/agentsdk"
)

const (
	// MetricHelpForAgent is a help string that replaces all agent metric help
	// messages. This is because a registry cannot have conflicting
	// help messages for the same metric in a "gather". If our coder agents are
	// on different versions, this is a possible scenario.
	metricHelpForAgent = "Metric is forwarded from workspace agent connected to this instance of coderd."
)

type MetricsAggregator struct {
	m     sync.Mutex
	log   slog.Logger
	queue []annotatedMetric
}

type annotatedMetric struct {
	agentsdk.AgentMetric

	username      string
	workspaceName string
	agentName     string
}

var _ prometheus.Collector = new(MetricsAggregator)

// Describe function does not have any knowledge about the metrics schema,
// so it does not emit anything.
func (*MetricsAggregator) Describe(_ chan<- *prometheus.Desc) {
}

var agentMetricsLabels = []string{usernameLabel, workspaceNameLabel, agentNameLabel}

func (ma *MetricsAggregator) Collect(ch chan<- prometheus.Metric) {
	ma.m.Lock()
	defer ma.m.Unlock()

	for _, m := range ma.queue {
		desc := prometheus.NewDesc(m.Name, metricHelpForAgent, agentMetricsLabels, nil)
		valueType, err := asPrometheusValueType(m.Type)
		if err != nil {
			ma.log.Error(context.Background(), "can't convert Prometheus value type", slog.F("value_type", m.Type), slog.Error(err))
		}
		constMetric := prometheus.MustNewConstMetric(desc, valueType, m.Value, m.username, m.workspaceName, m.agentName)
		ch <- constMetric
	}
}

// TODO Run function with done channel

func (ma *MetricsAggregator) Update(_ context.Context, username, workspaceName, agentName string, metrics []agentsdk.AgentMetric) {
	ma.m.Lock()
	defer ma.m.Unlock()

UpdateLoop:
	for _, m := range metrics {
		for i, q := range ma.queue {
			if q.username == username && q.workspaceName == workspaceName && q.agentName == agentName && q.Name == m.Name {
				ma.queue[i].AgentMetric.Value = m.Value
				continue UpdateLoop
			}
		}

		ma.queue = append(ma.queue, annotatedMetric{
			username:      username,
			workspaceName: workspaceName,
			agentName:     agentName,

			AgentMetric: m,
		})
	}
}

func asPrometheusValueType(metricType agentsdk.AgentMetricType) (prometheus.ValueType, error) {
	switch metricType {
	case agentsdk.AgentMetricTypeGauge:
		return prometheus.GaugeValue, nil
	case agentsdk.AgentMetricTypeCounter:
		return prometheus.CounterValue, nil
	default:
		return -1, xerrors.Errorf("unsupported value type: %s", metricType)
	}
}
