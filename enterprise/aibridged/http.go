package aibridged

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/aibridge"
	"github.com/coder/aibridge/recorder"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/enterprise/aibridged/proto"
)

var _ http.Handler = &Server{}

var (
	ErrNoAuthKey             = xerrors.New("no authentication key provided")
	ErrConnect               = xerrors.New("could not connect to coderd")
	ErrUnauthorized          = xerrors.New("unauthorized")
	ErrAcquireRequestHandler = xerrors.New("failed to acquire request handler")
)

// ServeHTTP is the entrypoint for requests which will be intercepted by AI Bridge.
// This function will validate that the given API key may be used to perform the request.
//
// An [aibridge.RequestBridge] instance is acquired from a pool based on the API key's
// owner (referred to as the "initiator"); this instance is responsible for the
// AI Bridge-specific handling of the request.
//
// A [DRPCClient] is provided to the [aibridge.RequestBridge] instance so that data can
// be passed up to a [DRPCServer] for persistence.
func (s *Server) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	logger := s.logger.With(
		slog.F("method", r.Method),
		slog.F("path", r.URL.Path),
	)

	// Extract and strip proxy request ID for cross-service log
	// correlation. Absent for direct requests not routed through
	// aibridgeproxyd.
	if proxyReqID := r.Header.Get(agplaibridge.HeaderCoderRequestID); proxyReqID != "" {
		// Inject into context so downstream loggers include it.
		ctx = slog.With(ctx, slog.F("aibridgeproxy_id", proxyReqID))
		logger = logger.With(slog.F("aibridgeproxy_id", proxyReqID))
	}
	r.Header.Del(agplaibridge.HeaderCoderRequestID)

	byok := agplaibridge.IsBYOK(r.Header)
	authMode := "centralized"
	if byok {
		authMode = "byok"
	}

	key := strings.TrimSpace(agplaibridge.ExtractAuthToken(r.Header))
	if key == "" {
		// Some clients (e.g. Claude) send a HEAD request
		// without credentials to check connectivity.
		if r.Method == http.MethodHead {
			logger.Info(ctx, "unauthenticated HEAD request")
		} else {
			logger.Warn(ctx, "no auth key provided")
		}
		http.Error(rw, ErrNoAuthKey.Error(), http.StatusBadRequest)
		return
	}

	// Strip every header that may carry the Coder token so it is
	// never forwarded to upstream providers. After stripping, the
	// aibridge library can treat the request as a normal LLM API call
	// with no Coder-specific information.
	if byok {
		// In BYOK mode the token is in X-Coder-AI-Governance-Token;
		// Authorization and X-Api-Key carry the user's own LLM credentials
		// and must be preserved.
		r.Header.Del(agplaibridge.HeaderCoderToken)
	} else {
		// In centralized mode the token may be in Authorization (the
		// documented path) or X-Api-Key (legacy clients that set
		// ANTHROPIC_API_KEY to their Coder token). Both are
		// stripped.
		r.Header.Del("Authorization")
		r.Header.Del("X-Api-Key")
	}

	client, err := s.Client()
	if err != nil {
		logger.Warn(ctx, "failed to connect to coderd", slog.Error(err))
		http.Error(rw, ErrConnect.Error(), http.StatusServiceUnavailable)
		return
	}

	resp, err := client.IsAuthorized(ctx, &proto.IsAuthorizedRequest{Key: key})
	if err != nil {
		logger.Warn(ctx, "key authorization check failed", slog.Error(err), slog.F("auth_mode", authMode))
		http.Error(rw, ErrUnauthorized.Error(), http.StatusForbidden)
		return
	}

	// Rewire request context to include actor.
	//
	// [NOTE]
	// The metadata provided here must NOT be sensitive as it could be included
	// in requests to upstream services.
	r = r.WithContext(aibridge.AsActor(ctx, resp.GetOwnerId(), recorder.Metadata{
		"Username": resp.GetUsername(),
	}))

	id, err := uuid.Parse(resp.GetOwnerId())
	if err != nil {
		logger.Warn(ctx, "failed to parse user ID", slog.Error(err), slog.F("id", resp.GetOwnerId()))
		http.Error(rw, ErrUnauthorized.Error(), http.StatusForbidden)
		return
	}

	handler, err := s.GetRequestHandler(ctx, Request{
		SessionKey:  key,
		APIKeyID:    resp.ApiKeyId,
		InitiatorID: id,
	})
	if err != nil {
		logger.Warn(ctx, "failed to acquire request handler", slog.Error(err))
		http.Error(rw, ErrAcquireRequestHandler.Error(), http.StatusInternalServerError)
		return
	}

	handler.ServeHTTP(rw, r)
}
