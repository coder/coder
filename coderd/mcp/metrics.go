package mcp

import "github.com/prometheus/client_golang/prometheus"

// Metrics records observability signals for the MCP HTTP server.
// A nil Metrics is valid and acts as a no-op, which keeps tests and
// callers that don't care about metrics free of boilerplate.
type Metrics struct {
	toolCalls      *prometheus.CounterVec
	toolDuration   *prometheus.HistogramVec
	sessionsOpen   prometheus.Gauge
	agentDials     prometheus.Counter
	agentConnsOpen prometheus.Gauge
}

// NewMetrics creates and registers Metrics against reg. A nil reg is allowed
// (metrics are created but unregistered), which is convenient for tests.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		toolCalls: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "mcp",
			Name:      "tool_calls_total",
			Help:      "Count of MCP tool invocations by tool name and outcome (success|error).",
		}, []string{"tool", "outcome"}),
		toolDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "mcp",
			Name:      "tool_duration_seconds",
			Help:      "Duration of MCP tool invocations in seconds, by tool name and outcome.",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300},
		}, []string{"tool", "outcome"}),
		sessionsOpen: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: "mcp",
			Name:      "sessions_open",
			Help:      "Number of MCP HTTP requests (tool-call handlers) currently in flight.",
		}),
		agentDials: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "mcp",
			Name:      "agent_dials_total",
			Help:      "Count of workspace agent dials triggered by MCP tool handlers. Each dial materializes a fresh tailnet.Conn (and wireguard Device).",
		}),
		agentConnsOpen: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: "mcp",
			Name:      "agent_conns_open",
			Help:      "Workspace agent connections currently held open by MCP tool handlers.",
		}),
	}
	if reg != nil {
		reg.MustRegister(
			m.toolCalls,
			m.toolDuration,
			m.sessionsOpen,
			m.agentDials,
			m.agentConnsOpen,
		)
	}
	return m
}

func (m *Metrics) observeTool(tool, outcome string, seconds float64) {
	if m == nil {
		return
	}
	m.toolCalls.WithLabelValues(tool, outcome).Inc()
	m.toolDuration.WithLabelValues(tool, outcome).Observe(seconds)
}

func (m *Metrics) sessionInc() {
	if m == nil {
		return
	}
	m.sessionsOpen.Inc()
}

func (m *Metrics) sessionDec() {
	if m == nil {
		return
	}
	m.sessionsOpen.Dec()
}

// AgentDialObserver returns a closure shaped to match
// toolsdk.WithAgentConnObserver: each call increments both the dial counter
// and the live-connection gauge, returning a release func that decrements
// the gauge when invoked. Safe to call when m is nil.
func (m *Metrics) AgentDialObserver() func() (release func()) {
	if m == nil {
		return nil
	}
	return func() func() {
		m.agentDials.Inc()
		m.agentConnsOpen.Inc()
		return func() { m.agentConnsOpen.Dec() }
	}
}
