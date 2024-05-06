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
	"github.com/coder/coder/v2/tailnet"
)

// IDs used in tests.
var (
	Client1ID = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	Client2ID = uuid.MustParse("00000000-0000-0000-0000-000000000002")
)

// StartServerBasic creates a coordinator and DERP server.
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

// StartClientBasic creates a client connection to the server.
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
