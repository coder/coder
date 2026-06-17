package agentfake

import "github.com/prometheus/client_golang/prometheus"

// Metrics holds the Prometheus collectors for the agentfake manager.
// A nil *Metrics is a valid no-op.
type Metrics struct {
	// ConnectedAgents is the number of fake agents with an established dRPC connection.
	ConnectedAgents prometheus.Gauge
}

// NewMetrics registers agentfake collectors on reg and returns the handle.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		ConnectedAgents: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "coder",
			Subsystem: "scaletest_agentfake",
			Name:      "connected_agents",
			Help:      "Number of fake agents with an established dRPC connection to coderd.",
		}),
	}
	reg.MustRegister(m.ConnectedAgents)
	m.ConnectedAgents.Set(0) // ensure the metric appears before any agent connects
	return m
}

func (m *Metrics) incConnected() {
	if m == nil {
		return
	}
	m.ConnectedAgents.Inc()
}

func (m *Metrics) decConnected() {
	if m == nil {
		return
	}
	m.ConnectedAgents.Dec()
}
