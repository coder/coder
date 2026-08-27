package mcpclient_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
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
	healthy := newTestMCPServer(t, echoTool())

	reaperDone := make(chan struct{}, 2)
	timeout := 1 * time.Second

	start := time.Now()
	tools, summaries, cleanup := mcpclient.ConnectAllForTest(ctx, logger,
		[]database.MCPServerConfig{
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
	require.Equal(t, mcpclient.ConnectOutcomeTimeout, summaries[0].Outcome)
	require.NotEmpty(t, summaries[0].Error)
	require.GreaterOrEqual(t, summaries[0].DurationMS, timeout.Milliseconds())
	require.Equal(t, "healthy", summaries[1].Slug)
	require.Equal(t, mcpclient.ConnectOutcomeConnected, summaries[1].Outcome)
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

// TestConnectAll_SlowServerStillConnects proves that a server that
// is slow but within the budget still connects and serves tools.
func TestConnectAll_SlowServerStillConnects(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	srv := mcp.NewServer(&mcp.Implementation{Name: "slow", Version: "1.0.0"}, nil)
	tool := echoTool()
	srv.AddTool(tool.tool, tool.handler)
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true},
	)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)

	cfg := makeConfig("slow", ts.URL)
	tools, _, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil, uuid.Nil, nil, nil)
	t.Cleanup(cleanup)

	require.Equal(t, []string{"slow__echo"}, toolNames(tools))
}

// TestConnectAll_LateServerReaped proves that when a server only
// responds after the budget expired, ConnectAll has long returned
// and the abandoned connect goroutine's late result is drained by
// the reaper so nothing leaks.
func TestConnectAll_LateServerReaped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	srv := mcp.NewServer(&mcp.Implementation{Name: "late", Version: "1.0.0"}, nil)
	tool := echoTool()
	srv.AddTool(tool.tool, tool.handler)
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true},
	)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)

	reaperDone := make(chan struct{}, 1)
	timeout := 500 * time.Millisecond

	start := time.Now()
	tools, summaries, cleanup := mcpclient.ConnectAllForTest(ctx, logger,
		[]database.MCPServerConfig{makeConfig("late", ts.URL)},
		timeout,
		func() { reaperDone <- struct{}{} },
	)
	elapsed := time.Since(start)
	t.Cleanup(cleanup)

	require.Less(t, elapsed, 4*timeout,
		"ConnectAll took %s, budget was %s", elapsed, timeout)
	require.Empty(t, tools)
	require.Len(t, summaries, 1)
	require.Equal(t, mcpclient.ConnectOutcomeTimeout, summaries[0].Outcome)

	select {
	case <-reaperDone:
	case <-time.After(30 * time.Second):
		t.Fatal("reaper did not exit after the late server responded")
	}
}

// TestConnectAll_CleanupPromptWhenServerWedges proves that the
// cleanup function returns promptly even when a connected session's
// server has stopped responding, so a wedged server cannot stall
// the generation loop at step boundaries. The session teardown
// DELETE is held server-side while cleanup must already have
// returned.
func TestConnectAll_CleanupPromptWhenServerWedges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	srv := mcp.NewServer(&mcp.Implementation{Name: "wedge", Version: "1.0.0"}, nil)
	tool := echoTool()
	srv.AddTool(tool.tool, tool.handler)
	// Stateful handler so closing the session sends a DELETE.
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv }, nil,
	)

	var deleteArrived atomic.Bool
	releaseDelete := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteArrived.Store(true)
			select {
			case <-releaseDelete:
			case <-r.Context().Done():
			}
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)

	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseDelete) }) }
	// Registered after ts.Close so it runs before it, letting the
	// wedged DELETE finish and the session unwind before the
	// server waits for outstanding requests.
	t.Cleanup(release)

	cfg := makeConfig("wedge", ts.URL)
	tools, _, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil, uuid.Nil, nil, nil)
	require.Equal(t, []string{"wedge__echo"}, toolNames(tools))

	start := time.Now()
	cleanup()
	elapsed := time.Since(start)
	require.Less(t, elapsed, 1*time.Second,
		"cleanup took %s with a wedged server", elapsed)

	// The teardown must still happen in the background: the wedged
	// DELETE arrives even though cleanup already returned.
	require.Eventually(t, deleteArrived.Load,
		10*time.Second, 10*time.Millisecond,
		"session close DELETE never reached the server")
	release()
}

// TestConnectAll_NoToolsWedgedCloseWithinBudget proves that a
// server whose session yields no usable tools cannot stall
// ConnectAll past the connect budget when its teardown DELETE
// wedges. The discarded session must be closed off the caller's
// path, and the teardown must still happen in the background.
func TestConnectAll_NoToolsWedgedCloseWithinBudget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	// Stateful handler with no registered tools so closing the
	// discarded session sends a DELETE.
	srv := mcp.NewServer(&mcp.Implementation{Name: "notools", Version: "1.0.0"}, nil)
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv }, nil,
	)

	var deleteArrived atomic.Bool
	releaseDelete := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteArrived.Store(true)
			select {
			case <-releaseDelete:
			case <-r.Context().Done():
			}
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)

	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseDelete) }) }
	t.Cleanup(release)

	cfg := makeConfig("notools", ts.URL)
	start := time.Now()
	tools, summaries, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil, uuid.Nil, nil, nil)
	elapsed := time.Since(start)
	t.Cleanup(cleanup)

	require.Empty(t, tools)
	require.Len(t, summaries, 1)
	require.Equal(t, mcpclient.ConnectOutcomeNoTools, summaries[0].Outcome)
	require.Less(t, elapsed, 5*time.Second,
		"ConnectAll took %s with a wedged no-tools teardown", elapsed)

	require.Eventually(t, deleteArrived.Load,
		10*time.Second, 10*time.Millisecond,
		"discarded session close DELETE never reached the server")
	release()
}
