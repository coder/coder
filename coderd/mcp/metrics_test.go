package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"

	"github.com/coder/aisdk-go"
	mcpserver "github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	codertestutil "github.com/coder/coder/v2/testutil"
)

// fakeTool returns a toolsdk.GenericTool whose handler invokes fn. It is
// sufficient for exercising the metrics/logging wrapper without the real
// coderd stack.
func fakeTool(name string, fn toolsdk.GenericHandlerFunc) toolsdk.GenericTool {
	return toolsdk.GenericTool{
		Tool: aisdk.Tool{
			Name:        name,
			Description: "test tool",
			Schema:      aisdk.Schema{Properties: map[string]any{}},
		},
		Handler: fn,
	}
}

func gaugeValue(t *testing.T, g prometheus.Gauge) float64 {
	t.Helper()
	var m dto.Metric
	require.NoError(t, g.Write(&m))
	return m.GetGauge().GetValue()
}

func TestMetrics_ToolCallSuccess(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := mcpserver.NewMetrics(reg)
	srv, err := mcpserver.NewServer(codertestutil.Logger(t), mcpserver.WithMetrics(m))
	require.NoError(t, err)

	tool := fakeTool("fake_ok", func(_ context.Context, _ toolsdk.Deps, _ json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`"ok"`), nil
	})
	wrapped := srv.MCPFromSDK(tool, toolsdk.Deps{})

	_, err = wrapped.Handler(context.Background(), mcp.CallToolRequest{})
	require.NoError(t, err)

	assert.InDelta(t, 1, testutil.ToFloat64(
		m.ToolCallsTotal().WithLabelValues("fake_ok", "success"),
	), 0.0001)
	assert.Equal(t, uint64(1), histogramCount(t, reg, "coderd_mcp_tool_duration_seconds", map[string]string{
		"tool":    "fake_ok",
		"outcome": "success",
	}))
}

func TestMetrics_ToolCallError(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := mcpserver.NewMetrics(reg)
	srv, err := mcpserver.NewServer(codertestutil.Logger(t), mcpserver.WithMetrics(m))
	require.NoError(t, err)

	tool := fakeTool("fake_err", func(_ context.Context, _ toolsdk.Deps, _ json.RawMessage) (json.RawMessage, error) {
		return nil, xerrors.New("boom")
	})
	wrapped := srv.MCPFromSDK(tool, toolsdk.Deps{})

	_, err = wrapped.Handler(context.Background(), mcp.CallToolRequest{})
	require.Error(t, err)

	assert.InDelta(t, 1, testutil.ToFloat64(
		m.ToolCallsTotal().WithLabelValues("fake_err", "error"),
	), 0.0001)
	// Success counter for same tool must remain at zero to confirm outcome labeling.
	assert.InDelta(t, 0, testutil.ToFloat64(
		m.ToolCallsTotal().WithLabelValues("fake_err", "success"),
	), 0.0001)
}

func TestMetrics_NilMetricsIsNoOp(t *testing.T) {
	t.Parallel()

	srv, err := mcpserver.NewServer(codertestutil.Logger(t)) // no WithMetrics
	require.NoError(t, err)

	tool := fakeTool("no_metrics", func(_ context.Context, _ toolsdk.Deps, _ json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`"ok"`), nil
	})
	wrapped := srv.MCPFromSDK(tool, toolsdk.Deps{})

	// Primary assertion: tool execution must not panic when metrics is nil.
	_, err = wrapped.Handler(context.Background(), mcp.CallToolRequest{})
	require.NoError(t, err)
}

func TestMetrics_AgentDialObserver(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := mcpserver.NewMetrics(reg)
	obs := m.AgentDialObserver()
	require.NotNil(t, obs)

	// First dial: counter == 1, live gauge == 1.
	release1 := obs()
	assert.InDelta(t, 1, testutil.ToFloat64(m.AgentDialsTotal()), 0.0001)
	assert.InDelta(t, 1, gaugeValue(t, m.AgentConnsOpen()), 0.0001)

	// Second concurrent dial: counter == 2, live gauge == 2.
	release2 := obs()
	assert.InDelta(t, 2, testutil.ToFloat64(m.AgentDialsTotal()), 0.0001)
	assert.InDelta(t, 2, gaugeValue(t, m.AgentConnsOpen()), 0.0001)

	// Releasing one drains the gauge but leaves the counter.
	release1()
	assert.InDelta(t, 2, testutil.ToFloat64(m.AgentDialsTotal()), 0.0001)
	assert.InDelta(t, 1, gaugeValue(t, m.AgentConnsOpen()), 0.0001)

	release2()
	assert.InDelta(t, 0, gaugeValue(t, m.AgentConnsOpen()), 0.0001)
}

func TestMetrics_AgentDialObserver_NilSafe(t *testing.T) {
	t.Parallel()

	var m *mcpserver.Metrics
	require.Nil(t, m.AgentDialObserver(), "nil Metrics must return a nil observer so toolsdk skips it")
}

func TestLogging_EmitsRequestorAndToolFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.Make(sloghuman.Sink(&buf)).Leveled(slog.LevelDebug)

	srv, err := mcpserver.NewServer(logger)
	require.NoError(t, err)

	tool := fakeTool("fake_logged", func(_ context.Context, _ toolsdk.Deps, _ json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`"ok"`), nil
	})
	wrapped := srv.MCPFromSDK(tool, toolsdk.Deps{})

	ctx := mcpserver.WithRequestor(context.Background(), mcpserver.Requestor{
		UserID:    "u-123",
		Username:  "alice",
		Email:     "alice@example.com",
		APIKeyID:  "kABC",
		RequestID: "req-xyz",
		UserAgent: "Claude-Test/1.0",
	})

	_, err = wrapped.Handler(ctx, mcp.CallToolRequest{})
	require.NoError(t, err)

	line := buf.String()
	for _, want := range []string{
		"mcp tool call",
		"tool=fake_logged",
		"outcome=success",
		"requestor_name=alice",
		"requestor_email=alice@example.com",
		"api_key_id=kABC",
		"request_id=req-xyz",
		"user_agent=Claude-Test/1.0",
	} {
		assert.True(t, strings.Contains(line, want),
			"log line missing %q\nfull output:\n%s", want, line)
	}
}

func TestLogging_ErrorPath(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.Make(sloghuman.Sink(&buf)).Leveled(slog.LevelDebug)

	srv, err := mcpserver.NewServer(logger)
	require.NoError(t, err)

	tool := fakeTool("fake_logged_err", func(_ context.Context, _ toolsdk.Deps, _ json.RawMessage) (json.RawMessage, error) {
		return nil, xerrors.New("kaboom")
	})
	wrapped := srv.MCPFromSDK(tool, toolsdk.Deps{})

	_, err = wrapped.Handler(context.Background(), mcp.CallToolRequest{})
	require.Error(t, err)

	line := buf.String()
	assert.Contains(t, line, "mcp tool call failed")
	assert.Contains(t, line, "outcome=error")
	assert.Contains(t, line, "kaboom")
}

func TestMetrics_SessionsOpenTracksServeHTTP(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := mcpserver.NewMetrics(reg)

	assert.InDelta(t, 0, gaugeValue(t, m.SessionsOpen()), 0.0001)

	// The server inc/dec wraps streamableServer.ServeHTTP; exercising the
	// public helpers directly is the narrowest possible assertion.
	m.SessionInc()
	assert.InDelta(t, 1, gaugeValue(t, m.SessionsOpen()), 0.0001)
	m.SessionDec()
	assert.InDelta(t, 0, gaugeValue(t, m.SessionsOpen()), 0.0001)
}

// histogramCount pulls the cumulative sample count for a histogram with the
// given fully-qualified name and label set.
func histogramCount(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) uint64 {
	t.Helper()
	mfs, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, metric := range mf.GetMetric() {
			if !labelsEqual(metric.GetLabel(), labels) {
				continue
			}
			return metric.GetHistogram().GetSampleCount()
		}
	}
	t.Fatalf("histogram %s with labels %v not found", name, labels)
	return 0
}

func labelsEqual(actual []*dto.LabelPair, want map[string]string) bool {
	if len(actual) != len(want) {
		return false
	}
	for _, lp := range actual {
		if want[lp.GetName()] != lp.GetValue() {
			return false
		}
	}
	return true
}
