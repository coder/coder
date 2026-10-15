package mcp_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/aisdk-go"
	mcpserver "github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestNormalizeClientName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"claude-code", mcpserver.ClientClaudeCode},
		{"Claude Code", mcpserver.ClientClaudeCode},
		{"claude-code/2.1.0 (external, cli)", mcpserver.ClientClaudeCode},
		{"cursor-vscode", mcpserver.ClientCursor},
		{"Cursor", mcpserver.ClientCursor},
		{"Visual Studio Code", mcpserver.ClientVSCode},
		{"Visual-Studio-Code", mcpserver.ClientVSCode},
		{"vscode", mcpserver.ClientVSCode},
		{"openai-mcp", mcpserver.ClientChatGPT},
		{"ChatGPT", mcpserver.ClientChatGPT},
		{"", mcpserver.ClientOther},
		{"claude-ai", mcpserver.ClientOther},
		{"my-custom-agent", mcpserver.ClientOther},
		{"Mozilla/5.0 (X11; Linux x86_64)", mcpserver.ClientOther},
	}
	allowed := map[string]bool{
		mcpserver.ClientClaudeCode: true,
		mcpserver.ClientCursor:     true,
		mcpserver.ClientVSCode:     true,
		mcpserver.ClientChatGPT:    true,
		mcpserver.ClientOther:      true,
	}
	for _, tt := range tests {
		got := mcpserver.NormalizeClientName(tt.input)
		assert.Equal(t, tt.want, got, "input %q", tt.input)
		assert.True(t, allowed[got], "normalized client %q is not in the allowlist", got)
	}
}

func TestNormalizeToolName(t *testing.T) {
	t.Parallel()

	for _, tool := range toolsdk.All {
		assert.Equal(t, tool.Name, mcpserver.NormalizeToolName(tool.Name))
	}
	assert.Equal(t, mcpserver.ToolOther, mcpserver.NormalizeToolName(""))
	assert.Equal(t, mcpserver.ToolOther, mcpserver.NormalizeToolName("not_a_real_tool"))
}

func TestClassifyToolError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want mcpserver.ToolCallResult
	}{
		{"Nil", nil, mcpserver.ToolCallResultSuccess},
		{"Generic", assert.AnError, mcpserver.ToolCallResultToolError},
		{"Unauthorized", codersdk.NewError(http.StatusUnauthorized, codersdk.Response{}), mcpserver.ToolCallResultAuthzError},
		{"Forbidden", codersdk.NewError(http.StatusForbidden, codersdk.Response{}), mcpserver.ToolCallResultAuthzError},
		{"WrappedForbidden", xerrors.Errorf("get workspace: %w", codersdk.NewError(http.StatusForbidden, codersdk.Response{})), mcpserver.ToolCallResultAuthzError},
		{"NotFound", codersdk.NewError(http.StatusNotFound, codersdk.Response{}), mcpserver.ToolCallResultToolError},
		{"InternalError", codersdk.NewError(http.StatusInternalServerError, codersdk.Response{}), mcpserver.ToolCallResultToolError},
		{"AuthzError", xerrors.Errorf("dial agent: %w", mcpserver.NewAuthzError("denied")), mcpserver.ToolCallResultAuthzError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, mcpserver.ClassifyToolError(tt.err))
		})
	}
}

// fakeRecorder collects tool calls for assertions.
type fakeRecorder struct {
	mu    sync.Mutex
	calls []mcpserver.ToolCall
}

func (r *fakeRecorder) RecordToolCall(call mcpserver.ToolCall) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, call)
}

func (r *fakeRecorder) last(t *testing.T) mcpserver.ToolCall {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	require.NotEmpty(t, r.calls)
	return r.calls[len(r.calls)-1]
}

func TestRegisterSDKTool_RecordsResultClasses(t *testing.T) {
	t.Parallel()

	type arguments struct {
		Mode string `json:"mode"`
	}
	tool := toolsdk.Tool[arguments, struct{}]{
		Tool: aisdk.Tool{
			Name: "test_metrics_tool",
			Schema: aisdk.Schema{
				Properties: map[string]any{
					"mode": map[string]any{"type": "string"},
				},
				Required: []string{"mode"},
			},
		},
		Handler: func(_ context.Context, _ toolsdk.Deps, args arguments) (struct{}, error) {
			switch args.Mode {
			case "tool_error":
				return struct{}{}, assert.AnError
			case "forbidden":
				return struct{}{}, xerrors.Errorf("get workspace: %w", codersdk.NewError(http.StatusForbidden, codersdk.Response{Message: "Forbidden."}))
			case "unauthorized":
				return struct{}{}, codersdk.NewError(http.StatusUnauthorized, codersdk.Response{Message: "Unauthorized."})
			default:
				return struct{}{}, nil
			}
		},
	}.Generic()

	reg := prometheus.NewRegistry()
	metrics, err := mcpserver.NewMetrics(reg)
	require.NoError(t, err)
	recorder := &fakeRecorder{}
	multi := multiRecorder{metrics, recorder}

	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	mcpserver.RegisterSDKTool(server, tool, toolsdk.Deps{}, multi)
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	ctx := testutil.Context(t, testutil.WaitShort)
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, serverSession.Close())
	})
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "claude-code", Version: "2.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, clientSession.Close())
	})

	tests := []struct {
		name      string
		arguments map[string]any
		want      mcpserver.ToolCallResult
	}{
		{"Success", map[string]any{"mode": "ok"}, mcpserver.ToolCallResultSuccess},
		{"ToolError", map[string]any{"mode": "tool_error"}, mcpserver.ToolCallResultToolError},
		{"ValidationError", map[string]any{}, mcpserver.ToolCallResultValidationError},
		{"Forbidden", map[string]any{"mode": "forbidden"}, mcpserver.ToolCallResultAuthzError},
		{"Unauthorized", map[string]any{"mode": "unauthorized"}, mcpserver.ToolCallResultAuthzError},
	}
	// Run sequentially so that the last recorded call belongs to the
	// current case.
	for _, tt := range tests {
		_, err := clientSession.CallTool(ctx, &sdkmcp.CallToolParams{
			Name:      tool.Name,
			Arguments: tt.arguments,
		})
		require.NoError(t, err, tt.name)
		call := recorder.last(t)
		assert.Equal(t, tt.want, call.Result, tt.name)
		// The test tool is not part of toolsdk.All, so its label must
		// collapse to the fallback.
		assert.Equal(t, mcpserver.ToolOther, call.Tool, tt.name)
		assert.Equal(t, mcpserver.ClientClaudeCode, call.Client, tt.name)
	}

	families, err := reg.Gather()
	require.NoError(t, err)
	// Labels are gathered in name order: client, result, tool.
	for result, count := range map[mcpserver.ToolCallResult]float64{
		mcpserver.ToolCallResultSuccess:         1,
		mcpserver.ToolCallResultToolError:       1,
		mcpserver.ToolCallResultValidationError: 1,
		mcpserver.ToolCallResultAuthzError:      2,
	} {
		assert.True(t, testutil.PromCounterHasValue(t, families, count, "coderd_mcp_tool_calls_total",
			mcpserver.ClientClaudeCode, string(result), mcpserver.ToolOther), "result %s", result)
	}
	assert.Equal(t, uint64(5), testutil.PromHistogramSampleCount(t, families, "coderd_mcp_tool_call_duration_seconds", mcpserver.ToolOther))
}

// TestMCPHTTP_ToolCallMetrics exercises the stateless HTTP server with a
// registered toolsdk tool and a coderd stand-in that rejects the session.
func TestMCPHTTP_ToolCallMetrics(t *testing.T) {
	t.Parallel()

	coderd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"You must be logged in."}`))
	}))
	t.Cleanup(coderd.Close)

	reg := prometheus.NewRegistry()
	metrics, err := mcpserver.NewMetrics(reg)
	require.NoError(t, err)

	server, err := mcpserver.NewServer(testutil.Logger(t), mcpserver.WithToolCallRecorder(metrics))
	require.NoError(t, err)
	require.NoError(t, server.RegisterTools(codersdk.New(testutil.MustURL(t, coderd.URL))))
	ts := httptest.NewServer(server)
	t.Cleanup(ts.Close)

	// Stateless requests on older protocol versions carry no clientInfo,
	// so the client is derived from the User-Agent header.
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` +
		toolsdk.ToolNameGetAuthenticatedUser + `","arguments":{}}}`
	ctx := testutil.Context(t, testutil.WaitShort)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.URL, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", "2025-06-18")
	req.Header.Set("User-Agent", "claude-code/2.0.0")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(respBody))
	require.Contains(t, string(respBody), `"isError":true`)

	families, err := reg.Gather()
	require.NoError(t, err)
	assert.True(t, testutil.PromCounterHasValue(t, families, 1, "coderd_mcp_tool_calls_total",
		mcpserver.ClientClaudeCode, string(mcpserver.ToolCallResultAuthzError), toolsdk.ToolNameGetAuthenticatedUser))
	assert.Equal(t, uint64(1), testutil.PromHistogramSampleCount(t, families, "coderd_mcp_tool_call_duration_seconds", toolsdk.ToolNameGetAuthenticatedUser))
}

type multiRecorder []mcpserver.ToolCallRecorder

func (m multiRecorder) RecordToolCall(call mcpserver.ToolCall) {
	for _, r := range m {
		r.RecordToolCall(call)
	}
}
