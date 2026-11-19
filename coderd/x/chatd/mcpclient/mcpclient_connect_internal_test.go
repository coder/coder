package mcpclient

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/safedial"
)

// connectAllForTest exposes connectAll with an injectable connect
// timeout and a reaperDone hook that fires after an abandoned
// connect goroutine has been drained and its late session closed.
func connectAllForTest(
	ctx context.Context,
	logger slog.Logger,
	servers []Server,
	timeout time.Duration,
	reaperDone func(),
) ([]fantasy.AgentTool, []ConnectSummary, func()) {
	// Connect-budget tests serve from loopback, which the guard
	// blocks by default, so allow it explicitly.
	httpClient := NewHTTPClient(nil, safedial.WithAllowedPrefixes(
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("::1/128"),
	))
	return connectAllWithHooks(
		ctx, logger, servers, nil, uuid.Nil, nil, nil,
		connectOptions{
			httpClient: httpClient,
			timeout:    timeout,
			hooks:      connectHooks{reaperDone: reaperDone},
			kind:       connectionKindOrg,
		},
	)
}

type testTool struct {
	tool    *mcp.Tool
	handler mcp.ToolHandler
}

func newTestMCPServer(t *testing.T, tools ...testTool) *httptest.Server {
	t.Helper()
	srv := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	for _, tool := range tools {
		srv.AddTool(tool.tool, tool.handler)
	}
	httpSrv := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true},
	)
	ts := httptest.NewServer(httpSrv)
	t.Cleanup(ts.Close)
	return ts
}

func textToolResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

func echoTool() testTool {
	return testTool{
		tool: &mcp.Tool{
			Name:        "echo",
			Description: "Echoes the input",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"input": map[string]any{
						"type":        "string",
						"description": "The input",
					},
				},
				"required": []string{"input"},
			},
		},
		handler: func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var args map[string]any
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return nil, err
			}
			input, _ := args["input"].(string)
			return textToolResult("echo: " + input), nil
		},
	}
}

func makeConfig(slug, url string) Server {
	return Server{
		ID:        uuid.New(),
		Slug:      slug,
		URL:       url,
		Transport: TransportStreamableHTTP,
	}
}

func toolNames(tools []fantasy.AgentTool) []string {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Info().Name)
	}
	return names
}

// blackHoleListener accepts TCP connections and never responds,
// simulating a server (or an edge in front of it) that silently
// drops requests. Returned connections are tracked so the test can
// terminate them.
type blackHoleListener struct {
	ln net.Listener

	mu    sync.Mutex
	conns []net.Conn
}

func newBlackHoleListener(t *testing.T) *blackHoleListener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	bh := &blackHoleListener{ln: ln}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			bh.mu.Lock()
			bh.conns = append(bh.conns, conn)
			bh.mu.Unlock()
		}
	}()
	t.Cleanup(bh.close)
	return bh
}

func (b *blackHoleListener) close() {
	_ = b.ln.Close()
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, c := range b.conns {
		_ = c.Close()
	}
	b.conns = nil
}

func (b *blackHoleListener) url() string {
	return "http://" + b.ln.Addr().String()
}

// TestConnectAll_BlackHoledServerBudget is the acceptance test for
// the connect budget: one black-holed server must not delay turn
// preparation beyond the budget, and healthy servers' tools must
// still be discovered. Without external budget enforcement the SDK
// blocks several times past the context deadline (observed: 12s
// for a 2s deadline) because its transport detaches the context.
func TestConnectAll_BlackHoledServerBudget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	bh := newBlackHoleListener(t)
	healthy := newTestMCPServer(t, echoTool())

	reaperDone := make(chan struct{}, 2)
	timeout := 1 * time.Second

	start := time.Now()
	tools, summaries, cleanup := connectAllForTest(ctx, logger,
		[]Server{
			makeConfig("blackhole", bh.url()),
			makeConfig("healthy", healthy.URL),
		},
		timeout,
		func() { reaperDone <- struct{}{} },
	)
	elapsed := time.Since(start)
	t.Cleanup(cleanup)

	// The budget must hold: well under the SDK's unbounded
	// behavior (6x the deadline), with margin for slow CI.
	require.Less(t, elapsed, 4*timeout,
		"ConnectAll took %s, budget was %s", elapsed, timeout)
	require.Equal(t, []string{"healthy__echo"}, toolNames(tools))

	// Per-server outcomes are summarized for logs and debug runs,
	// sorted by slug.
	require.Len(t, summaries, 2)
	require.Equal(t, "blackhole", summaries[0].Slug)
	require.Equal(t, ConnectOutcomeTimeout, summaries[0].Outcome)
	require.NotEmpty(t, summaries[0].Error)
	require.GreaterOrEqual(t, summaries[0].DurationMS, timeout.Milliseconds())
	require.Equal(t, "healthy", summaries[1].Slug)
	require.Equal(t, ConnectOutcomeConnected, summaries[1].Outcome)
	require.Equal(t, 1, summaries[1].ToolCount)

	// Terminating the black-holed connections unblocks the
	// abandoned connect goroutine; the reaper must then drain its
	// result and exit.
	bh.close()
	select {
	case <-reaperDone:
	case <-time.After(30 * time.Second):
		t.Fatal("reaper did not exit after black-holed connections were closed")
	}
}
