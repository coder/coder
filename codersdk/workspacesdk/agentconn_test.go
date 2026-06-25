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
			assert.NoError(t, err)
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
