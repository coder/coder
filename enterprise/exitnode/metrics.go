package exitnode

import "github.com/prometheus/client_golang/prometheus"

// Metrics holds the Prometheus collectors exposed by an exit node.
type Metrics struct {
	// FlowsTotal counts flows by policy decision ("allow" or "deny").
	FlowsTotal *prometheus.CounterVec
	// BytesTotal counts proxied bytes by direction ("in" is destination to
	// agent, "out" is agent to destination).
	BytesTotal *prometheus.CounterVec
	// ActiveFlows is the number of flows currently being proxied.
	ActiveFlows prometheus.Gauge
	// UnknownSourceTotal counts connections rejected because the source
	// tailnet address did not belong to a known agent.
	UnknownSourceTotal prometheus.Counter
	// PolicyReloadTotal counts policy reloads by result ("success" or
	// "error").
	PolicyReloadTotal *prometheus.CounterVec
	// PolicyMismatch reports whether a sibling replica has a different policy.
	PolicyMismatch prometheus.Gauge
	// FlowReportsSent counts flow reports delivered to coderd.
	FlowReportsSent prometheus.Counter
	// FlowReportsDropped counts flow reports discarded because the queue
	// overflowed.
	FlowReportsDropped prometheus.Counter
}

// NewMetrics creates the exit node collectors and registers them with reg
// when reg is non-nil. Opts are written as literals so the metrics docs
// scanner (scripts/metricsdocgen) can extract them.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		FlowsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder", Subsystem: "exit_node", Name: "flows_total",
			Help: "Total number of TCP flows handled by policy decision.",
		}, []string{"decision"}),
		BytesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder", Subsystem: "exit_node", Name: "bytes_total",
			Help: "Total number of bytes proxied by direction.",
		}, []string{"direction"}),
		ActiveFlows: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "coder", Subsystem: "exit_node", Name: "active_flows",
			Help: "Number of TCP flows currently being proxied.",
		}),
		UnknownSourceTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coder", Subsystem: "exit_node", Name: "unknown_source_total",
			Help: "Connections rejected because the source address was not a known agent.",
		}),
		PolicyReloadTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder", Subsystem: "exit_node", Name: "policy_reload_total",
			Help: "Policy reload attempts by result.",
		}, []string{"result"}),
		PolicyMismatch: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "coder", Subsystem: "exit_node", Name: "policy_mismatch",
			Help: "Whether a live sibling replica reports a different policy hash.",
		}),
		FlowReportsSent: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coder", Subsystem: "exit_node", Name: "flow_reports_sent_total",
			Help: "Flow reports delivered to coderd.",
		}),
		FlowReportsDropped: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coder", Subsystem: "exit_node", Name: "flow_reports_dropped_total",
			Help: "Flow reports dropped because the outgoing queue was full.",
		}),
	}
	if reg != nil {
		reg.MustRegister(m.FlowsTotal, m.BytesTotal, m.ActiveFlows, m.UnknownSourceTotal,
			m.PolicyReloadTotal, m.PolicyMismatch, m.FlowReportsSent, m.FlowReportsDropped)
	}
	return m
}
