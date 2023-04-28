package agent

import (
	"fmt"
	"strings"

	"tailscale.com/util/clientmetric"

	"github.com/coder/coder/codersdk/agentsdk"
)

func collectMetrics() []agentsdk.AgentMetric {
	// Tailscale metrics
	metrics := clientmetric.Metrics()
	collected := make([]agentsdk.AgentMetric, 0, len(metrics))
	for _, m := range metrics {
		if isIgnoredMetric(m.Name()) {
			continue
		}

		collected = append(collected, agentsdk.AgentMetric{
			Name:  m.Name(),
			Type:  asMetricType(m.Type()),
			Value: float64(m.Value()),
		})
	}
	return collected
}

// isIgnoredMetric checks if the metric should be ignored, as Coder agent doesn't use related features.
// Expected metric families: magicsock_*, derp_*, tstun_*, netcheck_*, portmap_*, etc.
func isIgnoredMetric(metricName string) bool {
	if strings.HasPrefix(metricName, "dns_") ||
		strings.HasPrefix(metricName, "controlclient_") ||
		strings.HasPrefix(metricName, "peerapi_") ||
		strings.HasPrefix(metricName, "profiles_") ||
		strings.HasPrefix(metricName, "tstun_") {
		return true
	}
	return false
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
