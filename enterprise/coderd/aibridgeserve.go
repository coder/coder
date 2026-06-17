package coderd

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/buildinfo"
	aibridgedproto "github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/coderd/aibridgedserver"
	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/websocket"
)

// aiGatewayKeyLastUsedInterval is how often an active DRPC session refreshes
// last_used_at for its authenticating key.
const aiGatewayKeyLastUsedInterval = 60 * time.Second

// aiBridgeServe upgrades the connection to a WebSocket and serves the aibridged
// DRPC services (Recorder, MCPConfigurator, Authorizer) to a remote standalone
// AI Gateway replica, mirroring CreateInMemoryAIBridgeServer for the embedded
// case and provisionerDaemonServe for the transport. AI Gateway key
// authentication is enforced before the WebSocket upgrade. License entitlement
// is enforced by middleware on the route.
//
// @Summary AI Gateway serve
// @ID ai-gateway-serve
// @Security CoderSessionToken
// @Tags Enterprise
// @Success 101
// @Router /api/v2/ai-gateway/serve [get]
func (api *API) aiBridgeServe(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := r.Header.Get(codersdk.AIGatewayKeyHeader)
	if key == "" {
		httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
			Message: "AI Gateway key required.",
		})
		return
	}

	// nolint:gocritic // System must look up the AI Gateway key to authenticate the request.
	keyID, err := api.Database.GetAIGatewayKeyIDByHashedSecret(dbauthz.AsSystemRestricted(ctx), apikey.HashSecret(key))
	if err != nil {
		// The lookup is an exact match, missing row means key is invalid.
		if httpapi.Is404Error(err) {
			httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
				Message: "AI Gateway key invalid.",
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to look up AI Gateway key.",
			Detail:  err.Error(),
		})
		return
	}

	// nolint:gocritic // A standalone AI Gateway acts as the AI Bridge daemon.
	ctx = dbauthz.AsAIBridged(ctx)
	r = r.WithContext(ctx)

	clientAPIVersion := r.URL.Query().Get("version")
	if err := aibridgedproto.CurrentVersion.Validate(clientAPIVersion); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Incompatible or unparsable version",
			Validations: []codersdk.ValidationError{
				{Field: "version", Detail: err.Error()},
			},
		})
		return
	}

	clientCoderVersion := r.Header.Get(codersdk.BuildVersionHeader)
	logger := api.Logger.Named("aibridge-serve").With(
		slog.F("remote_addr", r.RemoteAddr),
		slog.F("gateway_client_api_version", clientAPIVersion),
		slog.F("gateway_client_build_version", clientCoderVersion),
		slog.F("gateway_server_api_version", aibridgedproto.CurrentVersion.String()),
		slog.F("gateway_server_build_version", buildinfo.Version),
	)

	logger = logger.With(slog.F("gateway_key_id", keyID))
	aiGatewayUpdateKeyLastUsed(ctx, api, keyID)

	// Track the websocket so API shutdown waits for it to close.
	func() {
		api.AGPL.WebsocketWaitMutex.Lock()
		defer api.AGPL.WebsocketWaitMutex.Unlock()
		api.AGPL.WebsocketWaitGroup.Add(1)
	}()
	defer api.AGPL.WebsocketWaitGroup.Done()

	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		// Need to disable compression to avoid a data-race.
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		if !xerrors.Is(err, context.Canceled) {
			logger.Error(ctx, "accept aibridge websocket conn", slog.Error(err))
		}
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket connection.",
			Detail:  err.Error(),
		})
		return
	}
	// Align with yamux's default stream window size.
	conn.SetReadLimit(drpcsdk.YamuxDefaultStreamWindowSize)

	// Multiplexes the incoming connection using yamux, allowing multiple DRPC
	// calls to occur over the same connection.
	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	ctx, wsNetConn := codersdk.WebsocketNetConn(ctx, conn, websocket.MessageBinary)
	defer wsNetConn.Close()
	session, err := yamux.Server(wsNetConn, config)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("multiplex server: %s", err))
		return
	}

	srvCtx, srvCancel := context.WithCancel(ctx)
	defer srvCancel()

	// Record liveness for the authenticating key while the session is open.
	go aiGatewayTrackKeyUsage(srvCtx, api, keyID)

	mux := drpcmux.New()
	srv, err := aibridgedserver.NewServer(
		srvCtx,
		api.Database,
		logger,
		api.AccessURL.String(),
		api.DeploymentValues.AI.BridgeConfig,
		api.ExternalAuthConfigs,
		api.AGPL.Experiments,
		api.AGPL.AISeatTracker,
	)
	if err != nil {
		if !xerrors.Is(err, context.Canceled) {
			logger.Error(ctx, "create aibridge server", slog.Error(err))
		}
		_ = conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("create aibridge server: %s", err))
		return
	}
	if err := aibridgedserver.Register(mux, srv); err != nil {
		_ = conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("register aibridge services: %s", err))
		return
	}

	server := drpcserver.NewWithOptions(&tracing.DRPCHandler{Handler: mux},
		drpcserver.Options{
			Manager: drpcsdk.DefaultDRPCOptions(nil),
			Log: func(err error) {
				if xerrors.Is(err, io.EOF) {
					return
				}
				logger.Debug(srvCtx, "drpc server error", slog.Error(err))
			},
		},
	)

	logger.Info(ctx, "opened AI Gateway connection")
	err = server.Serve(srvCtx, session)
	srvCancel()
	logger.Info(ctx, "closed AI Gateway connection", slog.Error(err))
	if err != nil && !xerrors.Is(err, io.EOF) {
		_ = conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("serve: %s", err))
		return
	}
	_ = conn.Close(websocket.StatusGoingAway, "")
}

func aiGatewayUpdateKeyLastUsed(ctx context.Context, api *API, keyID uuid.UUID) {
	// nolint:gocritic // Recording AI Gateway key liveness is an internal system write.
	err := api.Database.UpdateAIGatewayKeyLastUsedAt(dbauthz.AsSystemRestricted(ctx), keyID)
	if err != nil && !xerrors.Is(err, context.Canceled) {
		api.Logger.Debug(ctx, "update aibridge gateway key last used", slog.Error(err), slog.F("key_id", keyID))
	}
}

// trackAIGatewayKeyUsage refreshes last_used_at for keyID until ctx is
// canceled. It records usage on a fixed interval.
func aiGatewayTrackKeyUsage(ctx context.Context, api *API, keyID uuid.UUID) {
	ticker := time.NewTicker(aiGatewayKeyLastUsedInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			aiGatewayUpdateKeyLastUsed(ctx, api, keyID)
		}
	}
}
