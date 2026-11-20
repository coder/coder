package mcpclient

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/safedial"
)

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
	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	server.AddTool(
		&mcp.Tool{Name: "echo", InputSchema: map[string]any{"type": "object"}},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		},
	)
	healthy := httptest.NewServer(mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true},
	))
	t.Cleanup(healthy.Close)

	timeout := 1 * time.Second

	start := time.Now()
	tools, summaries, cleanup := connectAllWithHooks(
		ctx, logger,
		[]Server{
			{ID: uuid.New(), Slug: "blackhole", URL: bh.url(), Transport: TransportStreamableHTTP},
			{ID: uuid.New(), Slug: "healthy", URL: healthy.URL, Transport: TransportStreamableHTTP},
		},
		nil, uuid.Nil, nil, nil,
		connectOptions{
			// Both servers listen on loopback, which the guard
			// blocks by default.
			httpClient: NewHTTPClient(nil, safedial.WithAllowedPrefixes(
				netip.MustParsePrefix("127.0.0.0/8"),
				netip.MustParsePrefix("::1/128"),
			)),
			timeout: timeout,
			kind:    connectionKindOrg,
		},
	)
	elapsed := time.Since(start)
	t.Cleanup(cleanup)

	// The budget must hold: well under the SDK's unbounded
	// behavior (6x the deadline), with margin for slow CI.
	require.Less(t, elapsed, 4*timeout,
		"ConnectAll took %s, budget was %s", elapsed, timeout)
	require.Len(t, tools, 1)
	require.Equal(t, "healthy__echo", tools[0].Info().Name)

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
}

func TestAwaitConnect_ClosesLateSession(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := mcp.NewServer(&mcp.Implementation{Name: "late"}, nil).
		Connect(ctx, serverTransport, nil)
	require.NoError(t, err)

	expired, cancel := context.WithCancel(ctx)
	cancel()
	budgetExpired := make(chan struct{})
	res := awaitConnect(expired, func() connectResult {
		select {
		case <-budgetExpired:
		case <-ctx.Done():
		}
		session, err := mcp.NewClient(&mcp.Implementation{Name: "coder"}, nil).
			Connect(ctx, clientTransport, nil)
		return connectResult{session: session, err: err}
	})
	require.ErrorIs(t, res.err, context.Canceled)
	close(budgetExpired)

	serverSawClose := make(chan error, 1)
	go func() { serverSawClose <- serverSession.Wait() }()
	testutil.TryReceive(ctx, t, serverSawClose)
}
