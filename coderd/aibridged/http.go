package aibridged

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/intercept"
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

	client, err := s.Client(ctx)
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

	// Direct and delegated carriers are mutually exclusive and request-scoped.
	// Delegated requests carry attribution stamped by the in-process caller
	// (chatd); direct requests carry workspace attribution parsed from the
	// server-minted token name by IsAuthorized.
	var attribution agplaibridge.Attribution
	if delegated {
		attribution, _ = agplaibridge.DelegatedAttributionFromContext(ctx)
	} else {
		attribution, err = attributionFromAuthorization(resp)
		if err != nil {
			logger.Warn(ctx, "invalid authorization attribution", slog.Error(err))
			http.Error(rw, ErrUnauthorized.Error(), http.StatusForbidden)
			return
		}
	}
	ctx = agplaibridge.WithAttribution(ctx, attribution)
	// coderd decides the user-scoped experiment for the key owner, because
	// the gateway cannot evaluate per-user experiment rules itself.
	ctx = agplaibridge.WithResponsesWebSocketEnabled(ctx, resp.GetResponsesWebsocketEnabled())

	// A Responses WebSocket the request opens checks each of its creates
	// like a separate HTTP request.
	ctx = aibridge.WithCreateAdmission(ctx, s.createAdmission(logger, id, agplaibridge.RateLimitConsumerFromContext(ctx)))
	// A Responses WebSocket holds a lease of the server's socket capacity.
	ctx = aibridge.WithSocketAcquirer(ctx, s.acquireSocket)

	refusal, err := budgetRefusal(ctx, client, id)
	if err != nil {
		logger.Warn(ctx, "user AI budget check failed", slog.Error(err))
		http.Error(rw, ErrBudgetCheck.Error(), http.StatusInternalServerError)
		return
	}
	if refusal != "" {
		http.Error(rw, refusal, http.StatusForbidden)
		return
	}

	// Rewire request context to include actor.
	//
	// [NOTE]
	// The metadata provided here must NOT be sensitive as it is recorded and
	// could be included in requests to upstream services. Email is passed
	// separately so it is only forwarded when configured.
	r = r.WithContext(aibridge.AsActor(ctx, resp.GetOwnerId(), resp.GetEmail(), recorder.Metadata{
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

// Refusals of a Responses WebSocket response.create. They match the HTTP
// refusals in status and message; the OpenAI error type and code name the
// cause for clients that parse them.
const (
	rateLimitedMessage = "You've been rate limited. Please try again later."
	// OpenAI reports an exhausted quota with this type and code.
	budgetExceededErrType = "insufficient_quota"
	budgetExceededErrCode = "insufficient_quota"
)

// budgetRefusal checks the AI budget of the user and returns the message a
// request is refused with when it is exceeded, or "" when it is not.
func budgetRefusal(ctx context.Context, client DRPCClient, userID uuid.UUID) (string, error) {
	resp, err := client.IsBudgetExceeded(ctx, &proto.IsBudgetExceededRequest{
		UserId: userID.String(),
	})
	if err != nil {
		return "", err
	}
	if !resp.GetExceeded() {
		return "", nil
	}
	return fmt.Sprintf(
		"AI budget of US$%.2f exceeded. Please contact an administrator for more details.",
		float64(resp.GetSpendLimitMicros())/1_000_000,
	), nil
}

// createAdmission returns the admission every Responses WebSocket
// response.create of a request runs: the create counts against the request's
// rate limiter bucket, when the request is rate limited, and then the user's
// AI budget is checked, as for a separate HTTP request. A refusal is an
// *intercept.ResponseError, so the client receives an error event and keeps
// its connection. A failed budget check refuses the create.
func (s *Server) createAdmission(logger slog.Logger, userID uuid.UUID, consumeRate agplaibridge.RateLimitConsumer) aibridge.CreateAdmissionFunc {
	return func(ctx context.Context, _ string) error {
		if consumeRate != nil {
			if err := consumeRate(); err != nil {
				return intercept.NewResponseError(rateLimitedMessage, intercept.OpenAIErrTypeRateLimit, intercept.OpenAIErrCodeRateLimit, http.StatusTooManyRequests, 0)
			}
		}
		checkFailed := func(err error) error {
			logger.Warn(ctx, "user AI budget check failed", slog.Error(err))
			return intercept.NewResponseError(ErrBudgetCheck.Error(), intercept.OpenAIErrTypeAPI, intercept.OpenAIErrCodeServer, http.StatusInternalServerError, 0)
		}
		// The connection to coderd may have been replaced since the request
		// was authorized, so each create acquires the current client.
		client, err := s.Client(ctx)
		if err != nil {
			return checkFailed(err)
		}
		refusal, err := budgetRefusal(ctx, client, userID)
		if err != nil {
			return checkFailed(err)
		}
		if refusal != "" {
			return intercept.NewResponseError(refusal, budgetExceededErrType, budgetExceededErrCode, http.StatusForbidden, 0)
		}
		return nil
	}
}

// acquireSocket adapts the socket registry to [aibridge.SocketAcquirer]. A
// refusal is an *intercept.ResponseError carrying the HTTP status the
// upgrade is refused with, since the bridge cannot know this package's
// errors.
func (s *Server) acquireSocket(ctx context.Context, actorID, provider string) (aibridge.SocketLease, error) {
	lease, err := s.Sockets().Acquire(ctx, actorID, provider)
	switch {
	case err == nil:
		return lease, nil
	case errors.Is(err, ErrSocketActorLimit), errors.Is(err, ErrSocketReplicaLimit):
		return nil, intercept.NewResponseError(err.Error(), intercept.OpenAIErrTypeRateLimit, intercept.OpenAIErrCodeRateLimit, http.StatusTooManyRequests, 0)
	case errors.Is(err, ErrShutdown):
		return nil, intercept.NewResponseError("AI Gateway is shutting down", intercept.OpenAIErrTypeAPI, intercept.OpenAIErrCodeServer, http.StatusServiceUnavailable, 0)
	default:
		return nil, err
	}
}

// attributionFromAuthorization extracts the workspace ID from the
// IsAuthorizedResponse. The proto carries only workspace_id for attribution;
// organization and workspace name are not returned.
func attributionFromAuthorization(resp *proto.IsAuthorizedResponse) (agplaibridge.Attribution, error) {
	if resp.GetWorkspaceId() == "" {
		return agplaibridge.Attribution{}, nil
	}
	workspaceID, err := uuid.Parse(resp.GetWorkspaceId())
	if err != nil {
		return agplaibridge.Attribution{}, xerrors.Errorf("parse workspace ID: %w", err)
	}
	return agplaibridge.Attribution{WorkspaceID: workspaceID}, nil
}
