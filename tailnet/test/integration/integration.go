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
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/tailnet"
)

// IDs used in tests.
var (
	Client1ID = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	Client2ID = uuid.MustParse("00000000-0000-0000-0000-000000000002")
)

type TestTopology struct {
	Name string
	// SetupNetworking creates interfaces and network namespaces for the test.
	// The most simple implementation is NetworkSetupDefault, which only creates
	// a network namespace shared for all tests.
	SetupNetworking func(t *testing.T, logger slog.Logger) TestNetworking

	// StartServer gets called in the server subprocess. It's expected to start
	// the coordinator server in the background and return.
	StartServer func(t *testing.T, logger slog.Logger, listenAddr string)
	// StartClient gets called in each client subprocess. It's expected to
	// create the tailnet.Conn and ensure connectivity to it's peer.
	StartClient func(t *testing.T, logger slog.Logger, serverURL *url.URL, myID uuid.UUID, peerID uuid.UUID) *tailnet.Conn

	// RunTests is the main test function. It's called in each of the client
	// subprocesses. If tests can only run once, they should check the client ID
	// and return early if it's not the expected one.
	RunTests func(t *testing.T, logger slog.Logger, serverURL *url.URL, myID uuid.UUID, peerID uuid.UUID, conn *tailnet.Conn)
}

type TestNetworking struct {
	// ServerListenAddr is the IP address and port that the server listens on,
	// passed to StartServer.
	ServerListenAddr string
	// ServerAccessURLClient1 is the hostname and port that the first client
	// uses to access the server.
	ServerAccessURLClient1 string
	// ServerAccessURLClient2 is the hostname and port that the second client
	// uses to access the server.
	ServerAccessURLClient2 string

	// Networking settings for each subprocess.
	ProcessServer  TestNetworkingProcess
	ProcessClient1 TestNetworkingProcess
	ProcessClient2 TestNetworkingProcess
}

type TestNetworkingProcess struct {
	// NetNS to enter. If zero, the current network namespace is used.
	NetNSFd int
}

func SetupNetworkingLoopback(t *testing.T, _ slog.Logger) TestNetworking {
	netNSName := "codertest_netns_"
	randStr, err := cryptorand.String(4)
	require.NoError(t, err, "generate random string for netns name")
	netNSName += randStr

	// Create a single network namespace for all tests so we can have an
	// isolated loopback interface.
	netNSFile, err := createNetNS(netNSName)
	require.NoError(t, err, "create network namespace")
	t.Cleanup(func() {
		_ = netNSFile.Close()
	})

	var (
		listenAddr = "127.0.0.1:8080"
		process    = TestNetworkingProcess{
			NetNSFd: int(netNSFile.Fd()),
		}
	)
	return TestNetworking{
		ServerListenAddr:       listenAddr,
		ServerAccessURLClient1: "http://" + listenAddr,
		ServerAccessURLClient2: "http://" + listenAddr,
		ProcessServer:          process,
		ProcessClient1:         process,
		ProcessClient2:         process,
	}
}

func StartServerBasic(t *testing.T, logger slog.Logger, listenAddr string) {
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

	// We have a custom listen address.
	srv := http.Server{
		Addr:        listenAddr,
		Handler:     r,
		ReadTimeout: 10 * time.Second,
	}
	serveDone := make(chan struct{})
	go func() {
		defer close(serveDone)
		err := srv.ListenAndServe()
		if err != nil && !xerrors.Is(err, http.ErrServerClosed) {
			t.Error("HTTP server error:", err)
		}
	}()
	t.Cleanup(func() {
		_ = srv.Close()
		<-serveDone
	})
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
						ForceHTTP:        true,
						InsecureForTests: true,
					},
				},
			},
		},
	}
}

func StartClientBasic(t *testing.T, logger slog.Logger, serverURL *url.URL, myID uuid.UUID, peerID uuid.UUID) *tailnet.Conn {
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

	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses:      []netip.Prefix{netip.PrefixFrom(tailnet.IPFromUUID(myID), 128)},
		DERPMap:        basicDERPMap(t, serverURL),
		BlockEndpoints: true,
		Logger:         logger,
		// These tests don't have internet connection, so we need to force
		// magicsock to do anything.
		ForceNetworkUp: true,
	})
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
