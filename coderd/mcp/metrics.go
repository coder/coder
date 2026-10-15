package mcp

import (
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"
)

// Metrics records Prometheus metrics for MCP tool calls served by coderd.
type Metrics struct {
	toolCalls        *prometheus.CounterVec
	toolCallDuration *prometheus.HistogramVec
}

var _ ToolCallRecorder = (*Metrics)(nil)

// NewMetrics creates MCP tool call metrics and registers them with reg
// when it is non-nil.
func NewMetrics(reg prometheus.Registerer) (*Metrics, error) {
	m := &Metrics{
		toolCalls: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "mcp",
			Name:      "tool_calls_total",
			Help:      "Total number of MCP tool calls handled by the remote MCP server, by tool, normalized client, and result.",
		}, []string{"tool", "client", "result"}),
		toolCallDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "mcp",
			Name:      "tool_call_duration_seconds",
			Help:      "Duration of MCP tool calls handled by the remote MCP server, by tool.",
			// Tools range from quick lookups to commands and builds that
			// run for minutes.
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300},
		}, []string{"tool"}),
	}
	if reg != nil {
		if err := reg.Register(m.toolCalls); err != nil {
			return nil, xerrors.Errorf("register MCP tool calls counter: %w", err)
		}
		if err := reg.Register(m.toolCallDuration); err != nil {
			return nil, xerrors.Errorf("register MCP tool call duration histogram: %w", err)
		}
	}
	return m, nil
}

// RecordToolCall implements ToolCallRecorder.
func (m *Metrics) RecordToolCall(call ToolCall) {
	m.toolCalls.WithLabelValues(call.Tool, call.Client, string(call.Result)).Inc()
	m.toolCallDuration.WithLabelValues(call.Tool).Observe(call.Duration.Seconds())
}
