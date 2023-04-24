package agent

import (
	"fmt"

	"tailscale.com/util/clientmetric"

	"github.com/coder/coder/codersdk/agentsdk"
)

func collectMetrics() []agentsdk.AgentMetric {
	// Tailscale metrics
	metrics := clientmetric.Metrics()
	collected := make([]agentsdk.AgentMetric, 0, len(metrics))
	for _, m := range metrics {
		collected = append(collected, agentsdk.AgentMetric{
			Name:  m.Name(),
			Type:  asMetricType(m.Type()),
			Value: float64(m.Value()),
		})
	}
	return collected
}

func asMetricType(typ clientmetric.Type) agentsdk.AgentMetricType {
	switch typ {
	case clientmetric.TypeGauge:
		return agentsdk.AgentMetricTypeGauge
	case clientmetric.TypeCounter:
		return agentsdk.AgentMetricTypeCounter
	default:
		panic(fmt.Sprintf("unknown metric type: %d", typ))
	}
}
