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
	"tailscale.com/tailcfg"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
)

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
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
			t.Cleanup(cancel)

			clientID := uuid.New()
			attackerID := uuid.New()
			victimID := uuid.New()
			clientConn, _ := newRedirectTestTailnetConn(t, derpMap, clientID, "client")
			attackerConn, attackerIP := newRedirectTestTailnetConn(t, derpMap, attackerID, "attacker")
			victimConn, victimIP := newRedirectTestTailnetConn(t, derpMap, victimID, "victim")
			coordinateRedirectTestTailnet(t, map[uuid.UUID]*tailnet.Conn{
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
			serveRedirectTestTailnetHTTP(t, victimConn, victimRouter)

			victimBaseURL := fmt.Sprintf("http://[%s]:%d", victimIP, workspacesdk.AgentHTTPAPIServerPort)
			attackerRouter := http.NewServeMux()
			attackerRouter.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
				http.Redirect(rw, r, victimBaseURL+r.URL.RequestURI(), tc.status)
			})
			serveRedirectTestTailnetHTTP(t, attackerConn, attackerRouter)

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

func newRedirectTestTailnetConn(t *testing.T, derpMap *tailcfg.DERPMap, id uuid.UUID, name string) (*tailnet.Conn, netip.Addr) {
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

func serveRedirectTestTailnetHTTP(t *testing.T, conn *tailnet.Conn, handler http.Handler) {
	t.Helper()

	ln, err := conn.Listen("tcp", fmt.Sprintf(":%d", workspacesdk.AgentHTTPAPIServerPort))
	require.NoError(t, err)

	server := &http.Server{Handler: handler}
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

func coordinateRedirectTestTailnet(t *testing.T, conns map[uuid.UUID]*tailnet.Conn) {
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
