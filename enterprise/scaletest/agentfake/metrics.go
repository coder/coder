package agentfake

import "github.com/prometheus/client_golang/prometheus"

// Metrics holds the Prometheus collectors for the agentfake manager.
// A nil *Metrics is a valid no-op.
type Metrics struct {
	// ConnectedAgents is the number of fake agents with an established dRPC connection.
	ConnectedAgents  prometheus.Gauge
	DERPMapsReceived prometheus.Counter
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
		DERPMapsReceived: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "scaletest_agentfake",
			Name:      "derp_maps_received_total",
			Help:      "Total number of DERP map messages received by fake agents.",
		}),
	}
	reg.MustRegister(m.ConnectedAgents, m.DERPMapsReceived)
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

func (m *Metrics) incDERPMapsReceived() {
	if m == nil {
		return
	}
	m.DERPMapsReceived.Inc()
}
