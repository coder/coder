package workspacesdk_test

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
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

// TestAgentConn_CloseEndsKeepAliveDial verifies that with
// APIIdleConnTimeout set, a dial that outlives its request ends when the
// conn is closed.
//
//nolint:paralleltest // goleak.IgnoreCurrent requires this test to run non-parallel.
func TestAgentConn_CloseEndsKeepAliveDial(t *testing.T) {
	logger := testutil.Logger(t)
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
		AgentID:            uuid.New(),
		CloseFunc:          func() error { return workspacesdk.ErrSkipClose },
		APIIdleConnTimeout: time.Minute,
	})

	// No agent exists, so the dial is still blocked in AwaitReachable when
	// the request context expires.
	ctx, cancel := context.WithTimeout(context.Background(), testutil.IntervalSlow)
	defer cancel()
	_, err = conn.ListeningPorts(ctx)
	require.Error(t, err)

	require.NoError(t, conn.Close())
	require.NoError(t, tailnetConn.Close())

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

func TestAgent_SSHUpgrade_Fallback(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	derpMap, _ := tailnettest.RunDERPAndSTUN(t)

	clientID := uuid.New()
	agentID := uuid.New()
	clientConn, _ := newTailnetConn(t, derpMap, clientID, "client")
	agentConn, agentIP := newTailnetConn(t, derpMap, agentID, "agent")
	stitchTailnet(t, map[uuid.UUID]*tailnet.Conn{
		clientID: clientConn,
		agentID:  agentConn,
	})

	// Simulate an older agent that does not support the tcp upgrade endpoint.
	var upgradeHit atomic.Bool
	upgradeHandler := http.NewServeMux()
	upgradeHandler.HandleFunc("/api/v0/tcp/", func(rw http.ResponseWriter, r *http.Request) {
		upgradeHit.Store(true)
		rw.WriteHeader(http.StatusNotFound)
	})
	serveTailnetHTTP(t, agentConn, upgradeHandler)

	ln, err := agentConn.Listen("tcp", fmt.Sprintf(":%d", workspacesdk.AgentStandardSSHPort))
	require.NoError(t, err)
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		assert.NoError(t, err)
		defer conn.Close()
		_, err = conn.Write([]byte("hello world"))
		assert.NoError(t, err)
	}()

	conn := workspacesdk.NewAgentConn(clientConn, workspacesdk.AgentConnOptions{
		AgentID: agentID,
	})
	require.True(t, clientConn.AwaitReachable(ctx, agentIP))

	sshConn, err := conn.SSHTCPConn(ctx)
	require.NoError(t, err)
	defer sshConn.Close()

	b, err := io.ReadAll(sshConn)
	require.NoError(t, err)

	require.True(t, upgradeHit.Load(), "should hit upgrade")
	require.Equal(t, "hello world", string(b))
}

func TestAgent_ReconnectingPTYUpgrade_Fallback(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	derpMap, _ := tailnettest.RunDERPAndSTUN(t)

	clientID := uuid.New()
	agentID := uuid.New()
	clientConn, _ := newTailnetConn(t, derpMap, clientID, "client")
	agentConn, agentIP := newTailnetConn(t, derpMap, agentID, "agent")
	stitchTailnet(t, map[uuid.UUID]*tailnet.Conn{
		clientID: clientConn,
		agentID:  agentConn,
	})

	// Simulate an older agent that does not support the tcp upgrade endpoint.
	var upgradeHit atomic.Bool
	upgradeHandler := http.NewServeMux()
	upgradeHandler.HandleFunc("/api/v0/tcp/", func(rw http.ResponseWriter, r *http.Request) {
		upgradeHit.Store(true)
		rw.WriteHeader(http.StatusNotFound)
	})
	serveTailnetHTTP(t, agentConn, upgradeHandler)

	ln, err := agentConn.Listen("tcp", fmt.Sprintf(":%d", workspacesdk.AgentReconnectingPTYPort))
	require.NoError(t, err)
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		assert.NoError(t, err)
		defer conn.Close()
		// Read in the init message.
		rawLen := make([]byte, 2)
		_, err = io.ReadFull(conn, rawLen)
		assert.NoError(t, err)
		_, err = io.ReadFull(conn, make([]byte, binary.LittleEndian.Uint16(rawLen)))
		assert.NoError(t, err)
		// Reply with some bytes.
		_, err = conn.Write([]byte("hello world"))
		assert.NoError(t, err)
	}()

	conn := workspacesdk.NewAgentConn(clientConn, workspacesdk.AgentConnOptions{
		AgentID: agentID,
	})
	require.True(t, clientConn.AwaitReachable(ctx, agentIP))

	ptyConn, err := conn.ReconnectingPTY(ctx, uuid.New(), 128, 128, "bash")
	require.NoError(t, err)
	defer ptyConn.Close()

	b, err := io.ReadAll(ptyConn)
	require.NoError(t, err)

	require.True(t, upgradeHit.Load(), "should hit upgrade")
	require.Equal(t, "hello world", string(b))
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

// newAPIPeers returns a client tailnet conn and the ID of an agent tailnet
// conn that runs server on the agent API port, on a listener wrapped by
// wrap if it is not nil.
func newAPIPeers(t *testing.T, server *http.Server, wrap func(net.Listener) net.Listener) (client *tailnet.Conn, agentID uuid.UUID) {
	t.Helper()

	derpMap, _ := tailnettest.RunDERPAndSTUN(t)
	clientID := uuid.New()
	agentID = uuid.New()
	clientConn, _ := newTailnetConn(t, derpMap, clientID, "client")
	agentConn, agentIP := newTailnetConn(t, derpMap, agentID, "agent")
	stitchTailnet(t, map[uuid.UUID]*tailnet.Conn{clientID: clientConn, agentID: agentConn})

	ln, err := agentConn.Listen("tcp", fmt.Sprintf(":%d", workspacesdk.AgentHTTPAPIServerPort))
	require.NoError(t, err)
	if wrap != nil {
		ln = wrap(ln)
	}
	t.Cleanup(func() {
		assert.NoError(t, server.Close())
	})
	go func() {
		err := server.Serve(ln)
		if err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, http.ErrServerClosed) {
			assert.NoError(t, err)
		}
	}()
	require.True(t, clientConn.AwaitReachable(testutil.Context(t, testutil.WaitMedium), agentIP))
	return clientConn, agentID
}

func jsonHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}\n"))
	})
}

// reuseTrace returns ctx with a trace that records whether the connection
// the request got was reused.
func reuseTrace(ctx context.Context) (context.Context, *atomic.Bool) {
	var reused atomic.Bool
	return httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			reused.Store(info.Reused)
		},
	}), &reused
}

var agentConnRequests = []struct {
	name string
	call func(context.Context, workspacesdk.AgentConn) error
}{
	{
		name: "GET",
		call: func(ctx context.Context, conn workspacesdk.AgentConn) error {
			_, err := conn.ListeningPorts(ctx)
			return err
		},
	},
	{
		name: "POST",
		call: func(ctx context.Context, conn workspacesdk.AgentConn) error {
			return conn.WriteFile(ctx, "/tmp/f", strings.NewReader("x"))
		},
	},
	{
		name: "POSTToolCall",
		call: func(ctx context.Context, conn workspacesdk.AgentConn) error {
			_, err := conn.StartProcess(workspacesdk.WithToolCallID(ctx, uuid.New()), workspacesdk.StartProcessRequest{Command: "true"})
			return err
		},
	},
}

// TestAgentConn_APIIdleConnTimeout checks that with APIIdleConnTimeout set,
// requests share one connection that Close closes, and that without it,
// each request uses a new connection.
func TestAgentConn_APIIdleConnTimeout(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		idleTimeout time.Duration
		conns       int
	}{
		{name: "Set", idleTimeout: time.Minute, conns: 1},
		{name: "Unset", conns: len(agentConnRequests)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var accepted, closed atomic.Int32
			clientConn, agentID := newAPIPeers(t, &http.Server{
				Handler:           jsonHandler(),
				ReadHeaderTimeout: testutil.WaitShort,
				ConnState: func(_ net.Conn, state http.ConnState) {
					switch state {
					case http.StateNew:
						accepted.Add(1)
					case http.StateClosed:
						closed.Add(1)
					}
				},
			}, nil)
			ctx := testutil.Context(t, testutil.WaitMedium)
			conn := workspacesdk.NewAgentConn(clientConn, workspacesdk.AgentConnOptions{
				AgentID:            agentID,
				CloseFunc:          func() error { return workspacesdk.ErrSkipClose },
				APIIdleConnTimeout: tc.idleTimeout,
			})

			for _, req := range agentConnRequests {
				require.NoError(t, req.call(ctx, conn), req.name)
			}
			require.Equal(t, tc.conns, int(accepted.Load()), "connections")

			require.NoError(t, conn.Close())
			require.Eventually(t, func() bool {
				return closed.Load() == accepted.Load()
			}, testutil.WaitShort, testutil.IntervalFast, "Close closes the idle connection")
			if tc.idleTimeout > 0 {
				_, err := conn.ListeningPorts(ctx)
				require.Error(t, err, "a closed conn dials no new connection")
			}
		})
	}
}

// TestAgentConn_ChunkedResponseKeepsIdleAPIConn checks that a decoded JSON
// response leaves the API connection reusable when the body is chunked, so
// the decoder may stop before EOF.
func TestAgentConn_ChunkedResponseKeepsIdleAPIConn(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	clientConn, agentID := newAPIPeers(t, &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ports": []any{}, "pad": strings.Repeat("x", 20<<10)})
			w.(http.Flusher).Flush() // Forces a chunked body.
		}),
		ReadHeaderTimeout: testutil.WaitShort,
	}, nil)
	conn := workspacesdk.NewAgentConn(clientConn, workspacesdk.AgentConnOptions{
		AgentID:            agentID,
		CloseFunc:          func() error { return workspacesdk.ErrSkipClose },
		APIIdleConnTimeout: time.Minute,
	})
	t.Cleanup(func() { _ = conn.Close() })

	_, err := conn.ListeningPorts(ctx)
	require.NoError(t, err)
	for i := range 10 {
		traceCtx, reused := reuseTrace(ctx)
		_, err := conn.ListeningPorts(traceCtx)
		require.NoError(t, err)
		require.Truef(t, reused.Load(), "request %d reused the idle connection", i+1)
	}
}

// dropIdleListener closes a connection when a request arrives on it after a
// response was written, as an agent does that restarted while the
// connection was idle.
type dropIdleListener struct{ net.Listener }

func (l dropIdleListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &dropIdleConn{Conn: c}, nil
}

type dropIdleConn struct {
	net.Conn
	responded atomic.Bool
}

func (c *dropIdleConn) Write(p []byte) (int, error) {
	c.responded.Store(true)
	return c.Conn.Write(p)
}

func (c *dropIdleConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 && c.responded.Load() {
		_ = c.Close()
		return 0, io.EOF
	}
	return n, err
}

// TestAgentConn_DroppedIdleAPIConn checks what APIIdleConnTimeout documents
// when the agent drops a kept connection: http.Transport resends a GET on a
// new connection, and a POST returns the error.
func TestAgentConn_DroppedIdleAPIConn(t *testing.T) {
	t.Parallel()

	clientConn, agentID := newAPIPeers(t, &http.Server{Handler: jsonHandler(), ReadHeaderTimeout: testutil.WaitShort}, func(ln net.Listener) net.Listener {
		return dropIdleListener{ln}
	})
	for _, tc := range agentConnRequests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			conn := workspacesdk.NewAgentConn(clientConn, workspacesdk.AgentConnOptions{
				AgentID:            agentID,
				CloseFunc:          func() error { return workspacesdk.ErrSkipClose },
				APIIdleConnTimeout: time.Minute,
			})
			t.Cleanup(func() { _ = conn.Close() })
			_, err := conn.ListeningPorts(ctx) // Leaves an idle connection.
			require.NoError(t, err)
			err = tc.call(ctx, conn)
			if tc.name == "GET" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
