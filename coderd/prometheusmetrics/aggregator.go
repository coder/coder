package prometheusmetrics

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/coder/coder/codersdk/agentsdk"
)

type MetricsAggregator struct{}

var _ prometheus.Collector = new(MetricsAggregator)

// Describe function does not have any knowledge about the metrics schema,
// so it does not emit anything.
func (*MetricsAggregator) Describe(_ chan<- *prometheus.Desc) {
}

func (ma *MetricsAggregator) Collect(ch chan<- prometheus.Metric) {
}

// TODO Run function with done channel

func (ma *MetricsAggregator) Update(ctx context.Context, workspaceID uuid.UUID, agentID uuid.UUID, metrics []agentsdk.AgentMetric) {
	log.Printf("Workspace: %s, Agent: %s, Metrics: %v", workspaceID, agentID, metrics) // FIXME
}
