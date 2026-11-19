package mcpclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/safedial"
)

// loopbackHTTPClient returns a guarded MCP client that allows
// loopback, where every test server in this file listens.
func loopbackHTTPClient() *http.Client {
	return mcpclient.NewHTTPClient(nil, safedial.WithAllowedPrefixes(
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("::1/128"),
	))
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
	tools, _, cleanup := mcpclient.ConnectAll(ctx, logger, []mcpclient.Server{cfg}, nil, uuid.Nil, nil, nil, loopbackHTTPClient())
	t.Cleanup(cleanup)

	require.Equal(t, []string{"slow__echo"}, toolNames(tools))
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
	tools, _, cleanup := mcpclient.ConnectAll(ctx, logger, []mcpclient.Server{cfg}, nil, uuid.Nil, nil, nil, loopbackHTTPClient())
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
	tools, summaries, cleanup := mcpclient.ConnectAll(ctx, logger, []mcpclient.Server{cfg}, nil, uuid.Nil, nil, nil, loopbackHTTPClient())
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
