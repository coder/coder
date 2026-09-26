package workspacesdk_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
// each request's context rather than the connection, that StartProcess
// reads the run age header, and that CancelToolCall sends its route and
// body, round-trips the recorded body, and decodes a coded 409.
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

	chatID := uuid.New()
	startCall := workspacesdk.ToolCall{MessageID: 42, ID: "toolu_start", Age: 1500 * time.Millisecond}
	startID := workspacesdk.ToolCallUUID(chatID, startCall.MessageID, startCall.ID).String()
	staleCall := workspacesdk.ToolCall{MessageID: 41, ID: "toolu_stale", Age: time.Minute}
	staleID := workspacesdk.ToolCallUUID(chatID, staleCall.MessageID, staleCall.ID).String()
	// Not valid UTF-8 and not JSON, so only a byte-exact round trip
	// matches.
	recordedBody := []byte{'{', 0xff, 0x00, '"', '\\', '\n'}
	exitCode := 137
	cancelResp := workspacesdk.CancelToolCallResponse{
		Started:     true,
		StatusCode:  http.StatusOK,
		ContentType: "application/json",
		Body:        recordedBody,
		Process: &workspacesdk.ToolCallProcess{
			Canceled: true,
			Output:   "partial",
			ExitCode: &exitCode,
			RunAgeMs: 1234,
		},
	}

	type received struct {
		method string
		path   string
		header http.Header
		body   []byte
	}
	receivedCh := make(chan received, 4)
	record := func(r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		receivedCh <- received{method: r.Method, path: r.URL.Path, header: r.Header.Clone(), body: body}
	}
	router := http.NewServeMux()
	router.HandleFunc("POST /api/v0/processes/start", func(rw http.ResponseWriter, r *http.Request) {
		record(r)
		rw.Header().Set("Content-Type", "application/json")
		rw.Header().Set(workspacesdk.CoderToolCallRunAgeMsHeader, "2500")
		_, _ = fmt.Fprintf(rw, `{"id":%q,"started":true}`, startID)
	})
	router.HandleFunc("POST /api/v0/tool-calls/{id}/cancel", func(rw http.ResponseWriter, r *http.Request) {
		record(r)
		rw.Header().Set("Content-Type", "application/json")
		if r.PathValue("id") == staleID {
			rw.WriteHeader(http.StatusConflict)
			_, _ = rw.Write([]byte(`{"code":"stale_tool_call","message":"Stale."}`))
			return
		}
		assert.NoError(t, json.NewEncoder(rw).Encode(cancelResp))
	})
	router.HandleFunc("GET /api/v0/processes/list", func(rw http.ResponseWriter, r *http.Request) {
		record(r)
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{"processes":[]}`))
	})
	serveTailnetHTTP(t, agentTailnet, router)
	require.True(t, clientConn.AwaitReachable(ctx, agentIP))

	conn := workspacesdk.NewAgentConn(clientConn, workspacesdk.AgentConnOptions{AgentID: agentID})
	conn.SetExtraHeaders(http.Header{
		workspacesdk.CoderChatIDHeader:     {chatID.String()},
		workspacesdk.CoderToolCallIDHeader: {"connection-wide"},
	})

	startResp, err := conn.StartProcess(workspacesdk.WithToolCall(ctx, startCall), workspacesdk.StartProcessRequest{Command: "true"})
	require.NoError(t, err)
	assert.Equal(t, workspacesdk.StartProcessResponse{ID: startID, Started: true, RunAge: 2500 * time.Millisecond}, startResp)
	got := testutil.RequireReceive(ctx, t, receivedCh)
	assert.Equal(t, chatID.String(), got.header.Get(workspacesdk.CoderChatIDHeader))
	assert.Equal(t, []string{"toolu_start"}, got.header.Values(workspacesdk.CoderToolCallIDHeader))
	assert.Equal(t, "42", got.header.Get(workspacesdk.CoderToolCallMessageIDHeader))
	assert.Equal(t, "1500", got.header.Get(workspacesdk.CoderToolCallAgeMsHeader))

	cancelCall := workspacesdk.ToolCall{MessageID: 43, ID: "toolu/cancel", Age: 20 * time.Millisecond}
	cancelID := workspacesdk.ToolCallUUID(chatID, cancelCall.MessageID, cancelCall.ID).String()
	gotCancel, err := conn.CancelToolCall(workspacesdk.WithToolCall(ctx, cancelCall), cancelID, workspacesdk.CancelToolCallRequest{StopIfRunAgeBelowMs: 30000})
	require.NoError(t, err)
	assert.Equal(t, cancelResp, gotCancel)
	assert.Equal(t, recordedBody, gotCancel.Body)
	got = testutil.RequireReceive(ctx, t, receivedCh)
	assert.Equal(t, http.MethodPost, got.method)
	assert.Equal(t, "/api/v0/tool-calls/"+cancelID+"/cancel", got.path)
	assert.JSONEq(t, `{"stop_if_run_age_below_ms":30000}`, string(got.body))
	assert.Equal(t, []string{"toolu%2Fcancel"}, got.header.Values(workspacesdk.CoderToolCallIDHeader))
	assert.Equal(t, "43", got.header.Get(workspacesdk.CoderToolCallMessageIDHeader))
	assert.Equal(t, "20", got.header.Get(workspacesdk.CoderToolCallAgeMsHeader))

	_, err = conn.CancelToolCall(workspacesdk.WithToolCall(ctx, staleCall), staleID, workspacesdk.CancelToolCallRequest{})
	var tcErr *workspacesdk.ToolCallError
	require.ErrorAs(t, err, &tcErr)
	assert.Equal(t, workspacesdk.ToolCallErrorStale, tcErr.Code)
	got = testutil.RequireReceive(ctx, t, receivedCh)
	assert.JSONEq(t, `{"stop_if_run_age_below_ms":0}`, string(got.body))
	assert.Equal(t, "41", got.header.Get(workspacesdk.CoderToolCallMessageIDHeader))

	_, err = conn.ListProcesses(ctx)
	require.NoError(t, err)
	got = testutil.RequireReceive(ctx, t, receivedCh)
	assert.Equal(t, []string{"connection-wide"}, got.header.Values(workspacesdk.CoderToolCallIDHeader))
	assert.Empty(t, got.header.Values(workspacesdk.CoderToolCallMessageIDHeader))
	assert.Empty(t, got.header.Values(workspacesdk.CoderToolCallAgeMsHeader))
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
