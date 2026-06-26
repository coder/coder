package integrationtest

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/client/transport"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/mcp"
)

// mockToolName is the primary mock tool name used in MCP tests.
const mockToolName = "coder_list_workspaces"

// mockMCP wraps a real mcp.ServerProxier with test assertion helpers.
// Implements mcp.ServerProxier so it can be passed directly to NewRequestBridge.
type mockMCP struct {
	mcp.ServerProxier
	calls *callAccumulator
}

// getCallsByTool returns recorded arguments for a given tool name.
func (m *mockMCP) getCallsByTool(name string) []any {
	return m.calls.getCallsByTool(name)
}

// setToolError configures a tool to return an error when invoked.
func (m *mockMCP) setToolError(tool, errMsg string) {
	m.calls.setToolError(tool, errMsg)
}

// setupMCPForTest creates a ready-to-use MCP server with proxy named "coder".
func setupMCPForTest(t *testing.T, tracer trace.Tracer) *mockMCP {
	t.Helper()
	return setupMCPForTestWithName(t, "coder", tracer)
}

func setupMCPForTestWithName(t *testing.T, name string, tracer trace.Tracer) *mockMCP {
	t.Helper()

	srv, acc := createMockMCPSrv(t)
	mcpSrv := httptest.NewServer(srv)
	t.Cleanup(mcpSrv.Close) // FIRST registered → runs LAST (LIFO)

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false}).Leveled(slog.LevelDebug)
	// Use a dedicated HTTP client so MCP mocks don't use http.DefaultTransport,
	// which can break when httptest.Server calls CloseIdleConnections in parallel
	// resulting in error `init MCP client: failed to send initialized notification: failed to send request: failed to send request: Post "http://127.0.0.1:43843": net/http: HTTP/1.x transport connection broken: http: CloseIdleConnections called`
	// https://github.com/golang/go/blob/44ec057a3e89482cf775f5eaaf03b0b5fcab1fa4/src/net/http/httptest/server.go#L268
	httpTransport := &http.Transport{}
	t.Cleanup(httpTransport.CloseIdleConnections)
	httpClient := &http.Client{Transport: httpTransport}
	proxy, err := mcp.NewStreamableHTTPServerProxy(name, mcpSrv.URL, nil, nil, nil, logger, tracer, transport.WithHTTPBasicClient(httpClient))
	require.NoError(t, err)

	mgr := mcp.NewServerProxyManager(map[string]mcp.ServerProxier{proxy.Name(): proxy}, tracer)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		require.NoError(t, mgr.Shutdown(ctx))
	})

	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	t.Cleanup(cancel)
	require.NoError(t, mgr.Init(ctx))
	require.NotEmpty(t, mgr.ListTools(), "mock MCP server should expose tools after init")

	return &mockMCP{ServerProxier: mgr, calls: acc}
}

func newNoopMCPManager() mcp.ServerProxier {
	return mcp.NewServerProxyManager(nil, noop.NewTracerProvider().Tracer(""))
}

// callAccumulator tracks all tool invocations by name and each instance's arguments.
type callAccumulator struct {
	calls      map[string][]any
	callsMu    sync.Mutex
	toolErrors map[string]string
}

func newCallAccumulator() *callAccumulator {
	return &callAccumulator{
		calls:      make(map[string][]any),
		toolErrors: make(map[string]string),
	}
}

func (a *callAccumulator) setToolError(tool string, errMsg string) {
	a.callsMu.Lock()
	defer a.callsMu.Unlock()
	a.toolErrors[tool] = errMsg
}

func (a *callAccumulator) getToolError(tool string) (string, bool) {
	a.callsMu.Lock()
	defer a.callsMu.Unlock()
	errMsg, ok := a.toolErrors[tool]
	return errMsg, ok
}

func (a *callAccumulator) addCall(tool string, args any) {
	a.callsMu.Lock()
	defer a.callsMu.Unlock()
	a.calls[tool] = append(a.calls[tool], args)
}

func (a *callAccumulator) getCallsByTool(name string) []any {
	a.callsMu.Lock()
	defer a.callsMu.Unlock()
	result := make([]any, len(a.calls[name]))
	copy(result, a.calls[name])
	return result
}

func createMockMCPSrv(t *testing.T) (http.Handler, *callAccumulator) {
	t.Helper()

	s := server.NewMCPServer(
		"Mock coder MCP server",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	acc := newCallAccumulator()

	for _, name := range []string{mockToolName, "coder_list_templates", "coder_template_version_parameters", "coder_get_authenticated_user", "coder_create_workspace_build", "coder_delete_template"} {
		tool := mcplib.NewTool(name,
			mcplib.WithDescription(fmt.Sprintf("Mock of the %s tool", name)),
		)
		s.AddTool(tool, func(_ context.Context, request mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			acc.addCall(request.Params.Name, request.Params.Arguments)
			if errMsg, ok := acc.getToolError(request.Params.Name); ok {
				return nil, xerrors.New(errMsg)
			}
			return mcplib.NewToolResultText("mock"), nil
		})
	}

	return server.NewStreamableHTTPServer(s), acc
}
