package aibridged

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/recorder"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
)

var _ http.Handler = &Server{}

var (
	ErrNoAuthKey             = xerrors.New("no authentication key provided")
	ErrConnect               = xerrors.New("could not connect to coderd")
	ErrUnauthorized          = xerrors.New("unauthorized")
	ErrAcquireRequestHandler = xerrors.New("failed to acquire request handler")
	ErrBudgetCheck           = xerrors.New("internal server error checking user AI budget")
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

	// When the request arrived via the in-process transport, the caller
	// has placed a delegated API key ID on the context. We trust that the
	// caller already established the user's identity and only validate
	// liveness; the caller does not have (and cannot send) the key secret.
	// Delegation is orthogonal to BYOK: a delegated request still carries
	// the user's own LLM credentials in Authorization/X-Api-Key when BYOK
	// is in effect.
	var (
		authReq *proto.IsAuthorizedRequest
	)

	delegatedID, delegated := agplaibridge.DelegatedAPIKeyIDFromContext(ctx)

	key := strings.TrimSpace(agplaibridge.ExtractAuthToken(r.Header))

	// When a BYOK header is present, a key is ALWAYS required.
	// Delegated auth only requires a key when using BYOK.
	if key == "" && !delegated {
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

	if delegated {
		authReq = &proto.IsAuthorizedRequest{KeyId: delegatedID}
	} else {
		authReq = &proto.IsAuthorizedRequest{Key: key}
	}

	// Strip every header that may carry the Coder token so it is never
	// forwarded to upstream providers. Runs for both header-auth and
	// delegated requests: a delegated caller may forward the user's BYOK
	// headers, and we still want to scrub any Coder-specific credentials
	// that may have leaked through. After stripping, the aibridge library
	// can treat the request as a normal LLM API call with no
	// Coder-specific information.
	if byok {
		// In BYOK mode the Coder token is in X-Coder-AI-Governance-Token;
		// Authorization and X-Api-Key carry the user's own LLM
		// credentials and must be preserved.
		r.Header.Del(agplaibridge.HeaderCoderToken)
	} else {
		// In centralized mode the Coder token may be in Authorization
		// (the documented path) or X-Api-Key (legacy clients that set
		// ANTHROPIC_API_KEY to their Coder token). Both are stripped.
		r.Header.Del("Authorization")
		r.Header.Del("X-Api-Key")
	}

	client, err := s.ClientContext(ctx)
	if err != nil {
		logger.Warn(ctx, "failed to connect to coderd", slog.Error(err))
		http.Error(rw, ErrConnect.Error(), http.StatusServiceUnavailable)
		return
	}

	// Attach auth attributes used by all log lines below. "source" is the
	// transport origin (e.g., "agents" for in-process callers, empty for
	// network callers); "auth_delegated" distinguishes header-based from
	// context-delegated authentication.
	logger = logger.With(
		slog.F("source", string(agplaibridge.SourceFromContext(ctx))),
		slog.F("auth_mode", authMode),
		slog.F("auth_delegated", delegated),
	)

	resp, err := client.IsAuthorized(ctx, authReq)
	if err != nil {
		logger.Warn(ctx, "key authorization check failed", slog.Error(err))
		http.Error(rw, ErrUnauthorized.Error(), http.StatusForbidden)
		return
	}

	id, err := uuid.Parse(resp.GetOwnerId())
	if err != nil {
		logger.Warn(ctx, "failed to parse user ID", slog.Error(err), slog.F("id", resp.GetOwnerId()))
		http.Error(rw, ErrUnauthorized.Error(), http.StatusForbidden)
		return
	}
	logger = logger.With(slog.F("user_id", id))

	budgetResp, err := client.IsBudgetExceeded(ctx, &proto.IsBudgetExceededRequest{
		UserId: id.String(),
	})
	if err != nil {
		logger.Warn(ctx, "user AI budget check failed", slog.Error(err))
		http.Error(rw, ErrBudgetCheck.Error(), http.StatusInternalServerError)
		return
	}
	if budgetResp.GetExceeded() {
		http.Error(rw, fmt.Sprintf(
			"AI budget of US$%.2f exceeded. Please contact an administrator for more details.",
			float64(budgetResp.GetSpendLimitMicros())/1_000_000,
		), http.StatusForbidden)
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
