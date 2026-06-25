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
	"github.com/coder/coder/v2/coderd/httpmw/loggermw"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/websocket"
)

// aiGatewayKeyLastUsedInterval defines how often an active DRPC session refreshes
// last_used_at for its authenticating key.
const aiGatewayKeyLastUsedInterval = 60 * time.Second

// aiBridgeServe upgrades the connection to a WebSocket and serves the aibridged
// DRPC services (Recorder, MCPConfigurator, Authorizer) to a remote standalone
// AI Gateway replica, mirroring the embedded case. AI Gateway key
// authentication is enforced before the WebSocket upgrade. License entitlement
// is enforced by middleware on the route.
//
// @Summary AI Gateway serve
// @ID ai-gateway-serve
// @Security AIGatewayKey
// @Tags Enterprise
// @Success 101
// @Router /api/v2/ai-gateway/serve [get]
func (api *API) aiBridgeServe(rw http.ResponseWriter, r *http.Request) {
	key := r.Header.Get(codersdk.AIGatewayKeyHeader)
	if key == "" {
		httpapi.Write(r.Context(), rw, http.StatusUnauthorized, codersdk.Response{
			Message: "AI Gateway key required.",
		})
		return
	}

	// nolint:gocritic // AI Gateway doesn't have Coder identity.System must look up the AI Gateway key to authenticate the request.
	gatewayKey, err := api.Database.GetAIGatewayKeyByHashedSecret(dbauthz.AsSystemRestricted(r.Context()), apikey.HashSecret(key))
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.Write(r.Context(), rw, http.StatusUnauthorized, codersdk.Response{
				Message: "AI Gateway key invalid.",
			})
			return
		}
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to look up AI Gateway key.",
		})
		return
	}

	clientAPIVersion := r.URL.Query().Get("version")
	clientCoderVersion := r.Header.Get(codersdk.BuildVersionHeader)
	logger := api.Logger.Named("aibridge-serve").With(
		slog.F("remote_addr", r.RemoteAddr),
		slog.F("ai_gateway_client_api_version", clientAPIVersion),
		slog.F("ai_gateway_client_build_version", clientCoderVersion),
		slog.F("ai_gateway_server_api_version", aibridgedproto.CurrentVersion.String()),
		slog.F("ai_gateway_server_build_version", buildinfo.Version),
		slog.F("ai_gateway_key_id", gatewayKey.ID),
		slog.F("ai_gateway_key_name", gatewayKey.Name),
		slog.F("ai_gateway_key_prefix", gatewayKey.SecretPrefix),
	)

	// keyCtx has the lifetime of the authenticated session.
	// It is canceled when the request ends or when the
	// authenticating key is deleted (from aiGatewayTrackKeyUsage).
	// The connection / DRPC server context derive from it,
	// canceling keyCtx tears down the session.
	keyCtx, keyCtxCancel := context.WithCancel(r.Context())
	defer keyCtxCancel()

	// Mark key as used as soon as the request is authenticated.
	if _, err := aiGatewayUpdateKeyLastUsed(keyCtx, api, gatewayKey.ID); err != nil {
		logger.Warn(keyCtx, "update ai gateway key last used", slog.Error(err))
	}
	go aiGatewayTrackKeyUsage(keyCtx, keyCtxCancel, api, gatewayKey.ID, logger)

	if err := aibridgedproto.CurrentVersion.Validate(clientAPIVersion); err != nil {
		httpapi.Write(keyCtx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Incompatible or unparsable version",
			Validations: []codersdk.ValidationError{
				{Field: "version", Detail: err.Error()},
			},
		})
		return
	}

	// Track the websocket so API shutdown waits for it to close.
	api.AGPL.WebsocketWaitMutex.Lock()
	api.AGPL.WebsocketWaitGroup.Add(1)
	api.AGPL.WebsocketWaitMutex.Unlock()
	defer api.AGPL.WebsocketWaitGroup.Done()

	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		// Need to disable compression to avoid a data-race, yamux reads and writes concurrently.
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		if !xerrors.Is(err, context.Canceled) {
			logger.Error(keyCtx, "accept aibridge websocket conn", slog.Error(err))
		}
		httpapi.Write(keyCtx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket connection.",
			Detail:  err.Error(),
		})
		return
	}

	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	connCtx, wsNetConn := codersdk.WebsocketNetConn(keyCtx, conn, websocket.MessageBinary)
	conn.SetReadLimit(drpcsdk.YamuxDefaultStreamWindowSize)
	defer wsNetConn.Close()
	session, err := yamux.Server(wsNetConn, config)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("multiplex server: %s", err))
		return
	}

	mux := drpcmux.New()
	srv, err := aibridgedserver.NewServer(
		connCtx,
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
			logger.Error(connCtx, "create aibridge server", slog.Error(err))
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
				logger.Debug(connCtx, "drpc server error", slog.Error(err))
			},
		},
	)

	// Log the request immediately instead of after it completes.
	if rl := loggermw.RequestLoggerFromContext(connCtx); rl != nil {
		rl.WriteLog(connCtx, http.StatusAccepted)
	}

	logger.Info(connCtx, "opened AI Gateway connection")
	err = server.Serve(connCtx, session)
	logger.Info(connCtx, "closed AI Gateway connection", slog.Error(err))
	if err != nil && !xerrors.Is(err, io.EOF) {
		_ = conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("serve: %s", err))
		return
	}
	_ = conn.Close(websocket.StatusGoingAway, "")
}

func aiGatewayUpdateKeyLastUsed(ctx context.Context, api *API, keyID uuid.UUID) (bool, error) {
	// nolint:gocritic // Recording AI Gateway key liveness is an internal system write.
	rows, err := api.Database.UpdateAIGatewayKeyLastUsedAt(dbauthz.AsSystemRestricted(ctx), keyID)
	if err != nil {
		return true, err
	}
	return rows > 0, nil
}

// aiGatewayTrackKeyUsage refreshes last_used_at for keyID on a fixed interval until ctx is canceled.
func aiGatewayTrackKeyUsage(ctx context.Context, ctxCancel context.CancelFunc, api *API, keyID uuid.UUID, logger slog.Logger) {
	ticker, done := api.NewTicker(aiGatewayKeyLastUsedInterval)
	defer done()

	consecutiveFailures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker:
		}

		active, err := aiGatewayUpdateKeyLastUsed(ctx, api, keyID)
		if err != nil {
			if xerrors.Is(err, context.Canceled) {
				return
			}
			consecutiveFailures++
			// Log failures with exponential backoff (1, 2, 4, 8...).
			// First failure logged at Debug, next failures escalate to Warn.
			if consecutiveFailures&(consecutiveFailures-1) == 0 {
				if consecutiveFailures == 1 {
					logger.Debug(ctx, "update ai gateway key last used", slog.Error(err), slog.F("consecutive_failures", consecutiveFailures))
				} else {
					logger.Warn(ctx, "update ai gateway key last used", slog.Error(err), slog.F("consecutive_failures", consecutiveFailures))
				}
			}
			continue
		}
		if consecutiveFailures > 1 {
			logger.Info(ctx, "ai gateway key last used update recovered",
				slog.F("consecutive_failures", consecutiveFailures))
		}
		consecutiveFailures = 0
		if !active {
			logger.Info(ctx, "ai gateway key no longer exists, closing connection")
			ctxCancel()
			return
		}
	}
}
