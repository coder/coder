package mcp

import (
	"github.com/mark3labs/mcp-go/server"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/coder/coder/v2/codersdk/toolsdk"
)

// MCPFromSDK exposes the internal mcpFromSDK wrapper to tests so they can
// exercise the instrumented handler without running the full HTTP transport.
func (s *Server) MCPFromSDK(tool toolsdk.GenericTool, deps toolsdk.Deps) server.ServerTool {
	return s.mcpFromSDK(tool, deps)
}

// ToolDepsOpts exposes the observer wiring for tests.
func (s *Server) ToolDepsOpts() []func(*toolsdk.Deps) {
	return s.toolDepsOpts()
}

// Test accessors for individual metrics. Kept in _test.go so they are only
// compiled for tests and do not widen the public surface.
func (m *Metrics) ToolCallsTotal() *prometheus.CounterVec { return m.toolCalls }
func (m *Metrics) SessionsOpen() prometheus.Gauge         { return m.sessionsOpen }
func (m *Metrics) AgentDialsTotal() prometheus.Counter    { return m.agentDials }
func (m *Metrics) AgentConnsOpen() prometheus.Gauge       { return m.agentConnsOpen }

// SessionInc/SessionDec re-export the unexported helpers for direct testing.
func (m *Metrics) SessionInc() { m.sessionInc() }
func (m *Metrics) SessionDec() { m.sessionDec() }
