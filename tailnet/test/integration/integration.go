//go:build linux
// +build linux

package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"tailscale.com/derp"
	"tailscale.com/derp/derphttp"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

// IDs used in tests.
var (
	Client1ID = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	Client2ID = uuid.MustParse("00000000-0000-0000-0000-000000000002")
)

type ServerOptions struct {
	// FailUpgradeDERP will make the DERP server fail to handle the initial DERP
	// upgrade in a way that causes the client to fallback to
	// DERP-over-WebSocket fallback automatically.
	// Incompatible with DERPWebsocketOnly.
	FailUpgradeDERP bool
	// DERPWebsocketOnly will make the DERP server only accept WebSocket
	// connections. If a DERP request is received that is not using WebSocket
	// fallback, the test will fail.
	// Incompatible with FailUpgradeDERP.
	DERPWebsocketOnly bool
}

//nolint:revive
func (o ServerOptions) Router(t *testing.T, logger slog.Logger) *chi.Mux {
	coord := tailnet.NewCoordinator(logger)
	var coordPtr atomic.Pointer[tailnet.Coordinator]
	coordPtr.Store(&coord)
	t.Cleanup(func() { _ = coord.Close() })

	csvc, err := tailnet.NewClientService(logger, &coordPtr, 10*time.Minute, func() *tailcfg.DERPMap {
		return &tailcfg.DERPMap{
			// Clients will set their own based on their custom access URL.
			Regions: map[int]*tailcfg.DERPRegion{},
		}
	})
	require.NoError(t, err)

	derpServer := derp.NewServer(key.NewNode(), tailnet.Logger(logger.Named("derp")))
	derpHandler, derpCloseFunc := tailnet.WithWebsocketSupport(derpServer, derphttp.Handler(derpServer))
	t.Cleanup(derpCloseFunc)

	r := chi.NewRouter()
	r.Use(
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				logger.Debug(r.Context(), "start "+r.Method, slog.F("path", r.URL.Path), slog.F("remote_ip", r.RemoteAddr))
				next.ServeHTTP(w, r)
			})
		},
		tracing.StatusWriterMiddleware,
		httpmw.Logger(logger),
	)

	r.Route("/derp", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			logger.Info(r.Context(), "start derp request", slog.F("path", r.URL.Path), slog.F("remote_ip", r.RemoteAddr))

			upgrade := strings.ToLower(r.Header.Get("Upgrade"))
			if upgrade != "derp" && upgrade != "websocket" {
				http.Error(w, "invalid DERP upgrade header", http.StatusBadRequest)
				t.Errorf("invalid DERP upgrade header: %s", upgrade)
				return
			}

			if o.FailUpgradeDERP && upgrade == "derp" {
				// 4xx status codes will cause the client to fallback to
				// DERP-over-WebSocket.
				http.Error(w, "test derp upgrade failure", http.StatusBadRequest)
				return
			}
			if o.DERPWebsocketOnly && upgrade != "websocket" {
				logger.Error(r.Context(), "non-websocket DERP request received", slog.F("path", r.URL.Path), slog.F("remote_ip", r.RemoteAddr))
				http.Error(w, "non-websocket DERP request received", http.StatusBadRequest)
				t.Error("non-websocket DERP request received")
				return
			}

			derpHandler.ServeHTTP(w, r)
		})
		r.Get("/latency-check", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	r.Get("/api/v2/workspaceagents/{id}/coordinate", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		idStr := chi.URLParamFromCtx(ctx, "id")
		id, err := uuid.Parse(idStr)
		if err != nil {
			logger.Warn(ctx, "bad agent ID passed in URL params", slog.F("id_str", idStr), slog.Error(err))
			httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
				Message: "Bad agent id.",
				Detail:  err.Error(),
			})
			return
		}

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			logger.Warn(ctx, "failed to accept websocket", slog.Error(err))
			httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to accept websocket.",
				Detail:  err.Error(),
			})
			return
		}

		ctx, wsNetConn := codersdk.WebsocketNetConn(ctx, conn, websocket.MessageBinary)
		defer wsNetConn.Close()

		err = csvc.ServeConnV2(ctx, wsNetConn, tailnet.StreamID{
			Name: "client-" + id.String(),
			ID:   id,
			Auth: tailnet.SingleTailnetCoordinateeAuth{},
		})
		if err != nil && !xerrors.Is(err, io.EOF) && !xerrors.Is(err, context.Canceled) {
			logger.Warn(ctx, "failed to serve conn", slog.Error(err))
			_ = conn.Close(websocket.StatusInternalError, err.Error())
			return
		}
	})

	return r
}

// StartClientDERP creates a client connection to the server for coordination
// and creates a tailnet.Conn which will only use DERP to connect to the peer.
func StartClientDERP(t *testing.T, logger slog.Logger, serverURL *url.URL, myID, peerID uuid.UUID) *tailnet.Conn {
	return startClientOptions(t, logger, serverURL, myID, peerID, &tailnet.Options{
		Addresses:           []netip.Prefix{netip.PrefixFrom(tailnet.IPFromUUID(myID), 128)},
		DERPMap:             basicDERPMap(t, serverURL),
		BlockEndpoints:      true,
		Logger:              logger,
		DERPForceWebSockets: false,
		// These tests don't have internet connection, so we need to force
		// magicsock to do anything.
		ForceNetworkUp: true,
	})
}

// StartClientDERPWebSockets does the same thing as StartClientDERP but will
// only use DERP WebSocket fallback.
func StartClientDERPWebSockets(t *testing.T, logger slog.Logger, serverURL *url.URL, myID, peerID uuid.UUID) *tailnet.Conn {
	return startClientOptions(t, logger, serverURL, myID, peerID, &tailnet.Options{
		Addresses:           []netip.Prefix{netip.PrefixFrom(tailnet.IPFromUUID(myID), 128)},
		DERPMap:             basicDERPMap(t, serverURL),
		BlockEndpoints:      true,
		Logger:              logger,
		DERPForceWebSockets: true,
		// These tests don't have internet connection, so we need to force
		// magicsock to do anything.
		ForceNetworkUp: true,
	})
}

// StartClientDirect does the same thing as StartClientDERP but disables
// BlockEndpoints (which enables Direct connections), and waits for a direct
// connection to be established between the two peers.
func StartClientDirect(t *testing.T, logger slog.Logger, serverURL *url.URL, myID, peerID uuid.UUID) *tailnet.Conn {
	conn := startClientOptions(t, logger, serverURL, myID, peerID, &tailnet.Options{
		Addresses:           []netip.Prefix{netip.PrefixFrom(tailnet.IPFromUUID(myID), 128)},
		DERPMap:             basicDERPMap(t, serverURL),
		BlockEndpoints:      false,
		Logger:              logger,
		DERPForceWebSockets: true,
		// These tests don't have internet connection, so we need to force
		// magicsock to do anything.
		ForceNetworkUp: true,
	})

	// Wait for direct connection to be established.
	peerIP := tailnet.IPFromUUID(peerID)
	require.Eventually(t, func() bool {
		t.Log("attempting ping to peer to judge direct connection")
		ctx := testutil.Context(t, testutil.WaitShort)
		_, p2p, pong, err := conn.Ping(ctx, peerIP)
		if err != nil {
			t.Logf("ping failed: %v", err)
			return false
		}
		if !p2p {
			t.Log("ping succeeded, but not direct yet")
			return false
		}
		t.Logf("ping succeeded, direct connection established via %s", pong.Endpoint)
		return true
	}, testutil.WaitLong, testutil.IntervalMedium)

	return conn
}

type ClientStarter struct {
	Options *tailnet.Options
}

func startClientOptions(t *testing.T, logger slog.Logger, serverURL *url.URL, myID, peerID uuid.UUID, options *tailnet.Options) *tailnet.Conn {
	u, err := serverURL.Parse(fmt.Sprintf("/api/v2/workspaceagents/%s/coordinate", myID.String()))
	require.NoError(t, err)
	//nolint:bodyclose
	ws, _, err := websocket.Dial(context.Background(), u.String(), nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ws.Close(websocket.StatusNormalClosure, "closing websocket")
	})

	client, err := tailnet.NewDRPCClient(
		websocket.NetConn(context.Background(), ws, websocket.MessageBinary),
		logger,
	)
	require.NoError(t, err)

	coord, err := client.Coordinate(context.Background())
	require.NoError(t, err)

	conn, err := tailnet.NewConn(options)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})

	coordination := tailnet.NewRemoteCoordination(logger, coord, conn, peerID)
	t.Cleanup(func() {
		_ = coordination.Close()
	})

	return conn
}

func basicDERPMap(t *testing.T, serverURL *url.URL) *tailcfg.DERPMap {
	portStr := serverURL.Port()
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err, "parse server port")

	hostname := serverURL.Hostname()
	ipv4 := ""
	ip, err := netip.ParseAddr(hostname)
	if err == nil {
		hostname = ""
		ipv4 = ip.String()
	}

	return &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			1: {
				RegionID:   1,
				RegionCode: "test",
				RegionName: "test server",
				Nodes: []*tailcfg.DERPNode{
					{
						Name:             "test0",
						RegionID:         1,
						HostName:         hostname,
						IPv4:             ipv4,
						IPv6:             "none",
						DERPPort:         port,
						STUNPort:         -1,
						ForceHTTP:        true,
						InsecureForTests: true,
					},
				},
			},
		},
	}
}
