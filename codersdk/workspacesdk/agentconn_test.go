package workspacesdk_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
)

// TestAgentConn_DialBoundedByRequestContext verifies that the
// transport dial behind the agent HTTP API stops when the request
// context ends. http.Transport detaches dial contexts from the
// request context so a pending dial can outlive its request and
// serve future ones, but the agent API client is request-scoped
// with keep-alives disabled, so a detached dial can never be
// reused. If the transport does not re-link cancellation, the dial
// goroutine stays blocked in AwaitReachable pinging an unreachable
// agent forever, even after the tailnet conn is closed, and leaks.
//
//nolint:paralleltest // goleak.IgnoreCurrent requires this test to run non-parallel.
func TestAgentConn_DialBoundedByRequestContext(t *testing.T) {
	// goleak.IgnoreCurrent snapshots running goroutines, so this
	// test must not run in parallel with other tests.
	logger := testutil.Logger(t)

	// Snapshot before the tailnet conn exists so everything spawned
	// below, including the transport dial goroutine, is verified.
	ignoreCurrent := goleak.IgnoreCurrent()

	tailnetConn, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{tailnet.TailscaleServicePrefix.RandomPrefix()},
		Logger:    logger.Named("client"),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tailnetConn.Close()
	})

	conn := workspacesdk.NewAgentConn(tailnetConn, workspacesdk.AgentConnOptions{
		AgentID: uuid.New(),
	})

	// No agent exists, so the transport dial blocks in
	// AwaitReachable until the request context expires. The timeout
	// only needs to be long enough for the dial goroutine to start;
	// its expiry is the behavior under test.
	ctx, cancel := context.WithTimeout(context.Background(), testutil.IntervalSlow)
	defer cancel()
	_, err = conn.ListeningPorts(ctx)
	require.Error(t, err)

	// Close the conn like test teardown would. The conn's own
	// goroutines exit on close; the dial goroutine must have already
	// exited when the request context expired.
	err = tailnetConn.Close()
	require.NoError(t, err)

	goleak.VerifyNone(t, ignoreCurrent)
}

func TestAgentConnRejectsCrossAgentRedirects(t *testing.T) {
	t.Parallel()

	derpMap, _ := tailnettest.RunDERPAndSTUN(t)
	cases := []struct {
		name   string
		status int
		invoke func(context.Context, workspacesdk.AgentConn) error
	}{
		{
			name:   "get 302",
			status: http.StatusFound,
			invoke: func(ctx context.Context, conn workspacesdk.AgentConn) error {
				_, err := conn.ListeningPorts(ctx)
				return err
			},
		},
		{
			name:   "post 307",
			status: http.StatusTemporaryRedirect,
			invoke: func(ctx context.Context, conn workspacesdk.AgentConn) error {
				return conn.WriteFile(ctx, "/tmp/attacker", strings.NewReader("redirect-body"))
			},
		},
		{
			name:   "post 308",
			status: http.StatusPermanentRedirect,
			invoke: func(ctx context.Context, conn workspacesdk.AgentConn) error {
				return conn.WriteFile(ctx, "/tmp/attacker", strings.NewReader("redirect-body"))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)

			clientID := uuid.New()
			attackerID := uuid.New()
			victimID := uuid.New()
			clientConn, _ := newTailnetConn(t, derpMap, clientID, "client")
			attackerConn, attackerIP := newTailnetConn(t, derpMap, attackerID, "attacker")
			victimConn, victimIP := newTailnetConn(t, derpMap, victimID, "victim")
			stitchTailnet(t, map[uuid.UUID]*tailnet.Conn{
				clientID:   clientConn,
				attackerID: attackerConn,
				victimID:   victimConn,
			})

			var victimHit atomic.Bool
			victimRouter := http.NewServeMux()
			victimRouter.HandleFunc("/api/v0/listening-ports", func(rw http.ResponseWriter, _ *http.Request) {
				victimHit.Store(true)
				rw.Header().Set("Content-Type", "application/json")
				_, _ = rw.Write([]byte(`{"ports":[]}`))
			})
			victimRouter.HandleFunc("/api/v0/write-file", func(rw http.ResponseWriter, _ *http.Request) {
				victimHit.Store(true)
				rw.WriteHeader(http.StatusOK)
			})
			serveTailnetHTTP(t, victimConn, victimRouter)

			victimBaseURL := fmt.Sprintf("http://[%s]:%d", victimIP, workspacesdk.AgentHTTPAPIServerPort)
			attackerRouter := http.NewServeMux()
			attackerRouter.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
				http.Redirect(rw, r, victimBaseURL+r.URL.RequestURI(), tc.status)
			})
			serveTailnetHTTP(t, attackerConn, attackerRouter)

			require.True(t, clientConn.AwaitReachable(ctx, attackerIP))
			require.True(t, clientConn.AwaitReachable(ctx, victimIP))

			conn := workspacesdk.NewAgentConn(clientConn, workspacesdk.AgentConnOptions{
				AgentID: attackerID,
			})

			err := tc.invoke(ctx, conn)
			require.Error(t, err)
			require.False(t, victimHit.Load())
		})
	}
}

// TestAgentConnAppHTTPClientRefusesRedirects verifies the app HTTP client does
// not follow redirects.
func TestAgentConnAppHTTPClientRefusesRedirects(t *testing.T) {
	t.Parallel()

	tailnetConn, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{tailnet.TailscaleServicePrefix.RandomPrefix()},
		Logger:    testutil.Logger(t),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tailnetConn.Close()
	})

	conn := workspacesdk.NewAgentConn(tailnetConn, workspacesdk.AgentConnOptions{
		AgentID: uuid.New(),
	})

	client := conn.AppHTTPClient()
	require.NotNil(t, client.CheckRedirect)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.invalid/", nil)
	require.NoError(t, err)
	require.ErrorIs(t, client.CheckRedirect(req, nil), http.ErrUseLastResponse)
}

// TestAgentConnToolCallRequests verifies that tool call headers come from
// each request's context rather than the connection, that CancelProcess
// uses its route, and that StartProcess decodes a coded 409.
func TestAgentConnToolCallRequests(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	derpMap, _ := tailnettest.RunDERPAndSTUN(t)

	clientID := uuid.New()
	agentID := uuid.New()
	clientConn, _ := newTailnetConn(t, derpMap, clientID, "client")
	agentTailnet, agentIP := newTailnetConn(t, derpMap, agentID, "agent")
	stitchTailnet(t, map[uuid.UUID]*tailnet.Conn{
		clientID: clientConn,
		agentID:  agentTailnet,
	})

	type received struct {
		method string
		path   string
		header http.Header
	}
	receivedCh := make(chan received, 3)
	record := func(r *http.Request) {
		receivedCh <- received{method: r.Method, path: r.URL.Path, header: r.Header.Clone()}
	}
	router := http.NewServeMux()
	router.HandleFunc("POST /api/v0/processes/start", func(rw http.ResponseWriter, r *http.Request) {
		record(r)
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusConflict)
		_, _ = rw.Write([]byte(`{"code":"stale_tool_call","message":"Stale."}`))
	})
	router.HandleFunc("POST /api/v0/processes/{id}/cancel", func(rw http.ResponseWriter, r *http.Request) {
		record(r)
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{"started":true,"canceled":true,"output":"partial","exit_code":137,"age_ms":1234}`))
	})
	router.HandleFunc("GET /api/v0/processes/list", func(rw http.ResponseWriter, r *http.Request) {
		record(r)
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{"processes":[]}`))
	})
	serveTailnetHTTP(t, agentTailnet, router)
	require.True(t, clientConn.AwaitReachable(ctx, agentIP))

	conn := workspacesdk.NewAgentConn(clientConn, workspacesdk.AgentConnOptions{AgentID: agentID})
	chatID := uuid.New()
	conn.SetExtraHeaders(http.Header{
		workspacesdk.CoderChatIDHeader:     {chatID.String()},
		workspacesdk.CoderToolCallIDHeader: {"connection-wide"},
	})

	startCall := workspacesdk.ToolCall{MessageID: 42, ID: "toolu_start", Age: 1500 * time.Millisecond}
	_, err := conn.StartProcess(workspacesdk.WithToolCall(ctx, startCall), workspacesdk.StartProcessRequest{Command: "true"})
	var tcErr *workspacesdk.ToolCallError
	require.ErrorAs(t, err, &tcErr)
	assert.Equal(t, workspacesdk.ToolCallErrorStale, tcErr.Code)
	got := testutil.RequireReceive(ctx, t, receivedCh)
	assert.Equal(t, chatID.String(), got.header.Get(workspacesdk.CoderChatIDHeader))
	assert.Equal(t, []string{"toolu_start"}, got.header.Values(workspacesdk.CoderToolCallIDHeader))
	assert.Equal(t, "42", got.header.Get(workspacesdk.CoderToolCallMessageIDHeader))
	assert.Equal(t, "1500", got.header.Get(workspacesdk.CoderToolCallAgeMsHeader))

	cancelCall := workspacesdk.ToolCall{MessageID: 43, ID: "toolu/cancel", Age: 20 * time.Millisecond}
	processID := workspacesdk.ToolCallUUID(chatID, cancelCall.MessageID, cancelCall.ID).String()
	cancelResp, err := conn.CancelProcess(workspacesdk.WithToolCall(ctx, cancelCall), processID)
	require.NoError(t, err)
	exitCode := 137
	assert.Equal(t, workspacesdk.CancelProcessResponse{
		Started:  true,
		Canceled: true,
		Output:   "partial",
		ExitCode: &exitCode,
		AgeMs:    1234,
	}, cancelResp)
	got = testutil.RequireReceive(ctx, t, receivedCh)
	assert.Equal(t, http.MethodPost, got.method)
	assert.Equal(t, "/api/v0/processes/"+processID+"/cancel", got.path)
	assert.Equal(t, []string{"toolu%2Fcancel"}, got.header.Values(workspacesdk.CoderToolCallIDHeader))
	assert.Equal(t, "43", got.header.Get(workspacesdk.CoderToolCallMessageIDHeader))
	assert.Equal(t, "20", got.header.Get(workspacesdk.CoderToolCallAgeMsHeader))

	_, err = conn.ListProcesses(ctx)
	require.NoError(t, err)
	got = testutil.RequireReceive(ctx, t, receivedCh)
	assert.Equal(t, []string{"connection-wide"}, got.header.Values(workspacesdk.CoderToolCallIDHeader))
	assert.Empty(t, got.header.Values(workspacesdk.CoderToolCallMessageIDHeader))
	assert.Empty(t, got.header.Values(workspacesdk.CoderToolCallAgeMsHeader))
}

// TestAgentConnFileToolCallRequests verifies that EditFiles and WriteFile
// decode a coded 409, that the cancel clients use their routes with the
// tool call headers, and that a cancel response rebuilds what the
// original request returned.
func TestAgentConnFileToolCallRequests(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	derpMap, _ := tailnettest.RunDERPAndSTUN(t)

	clientID := uuid.New()
	agentID := uuid.New()
	clientConn, _ := newTailnetConn(t, derpMap, clientID, "client")
	agentTailnet, agentIP := newTailnetConn(t, derpMap, agentID, "agent")
	stitchTailnet(t, map[uuid.UUID]*tailnet.Conn{
		clientID: clientConn,
		agentID:  agentTailnet,
	})

	// recorded is the response the fake agent gives to an edit or write,
	// and returns from a cancel of the same tool call.
	type recorded struct {
		status int
		body   string
	}
	var current atomic.Pointer[recorded]
	cancelPaths := make(chan string, 1)
	apply := func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		if r.Header.Get(workspacesdk.CoderToolCallIDHeader) == "stale" {
			rw.WriteHeader(http.StatusConflict)
			_, _ = rw.Write([]byte(`{"code":"stale_tool_call","message":"Stale."}`))
			return
		}
		rec := current.Load()
		rw.WriteHeader(rec.status)
		_, _ = rw.Write([]byte(rec.body))
	}
	cancel := func(rw http.ResponseWriter, r *http.Request) {
		cancelPaths <- r.URL.Path
		rw.Header().Set("Content-Type", "application/json")
		if r.Header.Get(workspacesdk.CoderToolCallIDHeader) == "stale" {
			rw.WriteHeader(http.StatusConflict)
			_, _ = rw.Write([]byte(`{"code":"stale_tool_call","message":"Stale."}`))
			return
		}
		rec := current.Load()
		_, _ = fmt.Fprintf(rw, `{"started":true,"status_code":%d,"body":%s}`, rec.status, rec.body)
	}
	router := http.NewServeMux()
	router.HandleFunc("POST /api/v0/edit-files", apply)
	router.HandleFunc("POST /api/v0/write-file", apply)
	router.HandleFunc("POST /api/v0/edit-files/{id}/cancel", cancel)
	router.HandleFunc("POST /api/v0/write-file/{id}/cancel", cancel)
	serveTailnetHTTP(t, agentTailnet, router)
	require.True(t, clientConn.AwaitReachable(ctx, agentIP))

	conn := workspacesdk.NewAgentConn(clientConn, workspacesdk.AgentConnOptions{AgentID: agentID})
	chatID := uuid.New()
	conn.SetExtraHeaders(http.Header{workspacesdk.CoderChatIDHeader: {chatID.String()}})
	toolCall := workspacesdk.ToolCall{MessageID: 7, ID: "toolu/file", Age: 30 * time.Millisecond}
	id := workspacesdk.ToolCallUUID(chatID, toolCall.MessageID, toolCall.ID).String()
	tcCtx := workspacesdk.WithToolCall(ctx, toolCall)
	staleCtx := workspacesdk.WithToolCall(ctx, workspacesdk.ToolCall{MessageID: 7, ID: "stale"})

	// requireSameError requires that rebuilt is the error original was,
	// without the request method and URL.
	requireSameError := func(t *testing.T, original, rebuilt error) {
		t.Helper()

		var origSDK, rebuiltSDK *codersdk.Error
		require.ErrorAs(t, original, &origSDK)
		require.ErrorAs(t, rebuilt, &rebuiltSDK)
		assert.Equal(t, origSDK.StatusCode(), rebuiltSDK.StatusCode())
		assert.Equal(t, origSDK.Response, rebuiltSDK.Response)
		assert.Equal(t, origSDK.Helper, rebuiltSDK.Helper)
		assert.Empty(t, rebuiltSDK.Method())
		assert.Empty(t, rebuiltSDK.URL())
		assert.Equal(t, original.Error(), fmt.Sprintf("%s %s: %s", origSDK.Method(), origSDK.URL(), rebuilt.Error()))
	}

	// The fake agent's response is shared state, so the cases run in
	// sequence.
	for _, rec := range []recorded{
		{status: http.StatusOK, body: `{"files":[{"path":"/a","diff":"@@ -1 +1 @@"}]}`},
		{status: http.StatusBadRequest, body: `{"message":"edit /a: no match","detail":"hint"}`},
	} {
		current.Store(&rec)
		wantResp, wantErr := conn.EditFiles(tcCtx, workspacesdk.FileEditRequest{})
		cancelResp, err := conn.CancelEditFiles(tcCtx, id)
		require.NoError(t, err)
		assert.Equal(t, "/api/v0/edit-files/"+id+"/cancel", testutil.RequireReceive(ctx, t, cancelPaths))
		require.True(t, cancelResp.Started)
		gotResp, gotErr := cancelResp.EditFilesResult()
		assert.Equal(t, wantResp, gotResp)
		if wantErr == nil {
			assert.NoError(t, gotErr)
			assert.NotEmpty(t, gotResp.Files)
		} else {
			requireSameError(t, wantErr, gotErr)
		}
	}

	_, err := conn.EditFiles(staleCtx, workspacesdk.FileEditRequest{})
	var tcErr *workspacesdk.ToolCallError
	require.ErrorAs(t, err, &tcErr)
	assert.Equal(t, workspacesdk.ToolCallErrorStale, tcErr.Code)
	_, err = conn.CancelEditFiles(staleCtx, id)
	require.ErrorAs(t, err, &tcErr)
	assert.Equal(t, workspacesdk.ToolCallErrorStale, tcErr.Code)
	testutil.RequireReceive(ctx, t, cancelPaths)

	for _, rec := range []recorded{
		{status: http.StatusOK, body: `{"message":"Successfully wrote to \"/a\""}`},
		{status: http.StatusForbidden, body: `{"message":"open /a: permission denied"}`},
	} {
		current.Store(&rec)
		wantErr := conn.WriteFile(tcCtx, "/a", strings.NewReader("content"))
		cancelResp, err := conn.CancelWriteFile(tcCtx, id)
		require.NoError(t, err)
		assert.Equal(t, "/api/v0/write-file/"+id+"/cancel", testutil.RequireReceive(ctx, t, cancelPaths))
		require.True(t, cancelResp.Started)
		gotErr := cancelResp.WriteFileResult()
		if wantErr == nil {
			assert.NoError(t, gotErr)
		} else {
			requireSameError(t, wantErr, gotErr)
		}
	}

	err = conn.WriteFile(staleCtx, "/a", strings.NewReader("content"))
	require.ErrorAs(t, err, &tcErr)
	assert.Equal(t, workspacesdk.ToolCallErrorStale, tcErr.Code)
	_, err = conn.CancelWriteFile(staleCtx, id)
	require.ErrorAs(t, err, &tcErr)
	assert.Equal(t, workspacesdk.ToolCallErrorStale, tcErr.Code)
	testutil.RequireReceive(ctx, t, cancelPaths)
}

func newTailnetConn(t *testing.T, derpMap *tailcfg.DERPMap, id uuid.UUID, name string) (*tailnet.Conn, netip.Addr) {
	t.Helper()

	addr := tailnet.TailscaleServicePrefix.AddrFromUUID(id)
	conn, err := tailnet.NewConn(&tailnet.Options{
		ID:        id,
		Addresses: []netip.Prefix{netip.PrefixFrom(addr, 128)},
		Logger:    testutil.Logger(t).Named(name),
		DERPMap:   derpMap,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, conn.Close())
	})

	return conn, addr
}

func serveTailnetHTTP(t *testing.T, conn *tailnet.Conn, handler http.Handler) {
	t.Helper()

	ln, err := conn.Listen("tcp", fmt.Sprintf(":%d", workspacesdk.AgentHTTPAPIServerPort))
	require.NoError(t, err)

	server := &http.Server{Handler: handler, ReadHeaderTimeout: testutil.WaitShort}
	t.Cleanup(func() {
		assert.NoError(t, server.Close())
		assert.NoError(t, ln.Close())
	})

	go func() {
		err := server.Serve(ln)
		if err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, http.ErrServerClosed) {
			assert.NoError(t, err)
		}
	}()
}

// stitchTailnet cross-programs every conn's node into every other conn, the
// N-peer analog of tailnet's stitch test helper, so the peers can reach each
// other without a coordinator.
func stitchTailnet(t *testing.T, conns map[uuid.UUID]*tailnet.Conn) {
	t.Helper()

	sendNode := func(srcID uuid.UUID, node *tailnet.Node) {
		protoNode, err := tailnet.NodeToProto(node)
		if !assert.NoError(t, err) {
			return
		}
		for dstID, dst := range conns {
			if dstID == srcID {
				continue
			}
			err = dst.UpdatePeers([]*proto.CoordinateResponse_PeerUpdate{{
				Id:   srcID[:],
				Node: protoNode,
				Kind: proto.CoordinateResponse_PeerUpdate_NODE,
			}})
			if err != nil && !errors.Is(err, tailnet.ErrConnClosed) {
				assert.NoError(t, err)
			}
		}
	}

	for srcID, src := range conns {
		src.SetNodeCallback(func(node *tailnet.Node) {
			sendNode(srcID, node)
		})
		if node := src.Node(); node != nil {
			sendNode(srcID, node)
		}
	}

	t.Cleanup(func() {
		for _, conn := range conns {
			conn.SetNodeCallback(nil)
		}
	})
}
