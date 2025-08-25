package coderd

import (
	"net/http"

	"cdr.dev/slog"

	"github.com/google/uuid"

	"github.com/coder/aibridge"
	"github.com/coder/coder/v2/aibridged"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpmw"
)

// bridgeAIRequest handles requests destined for an upstream AI provider; aibridged intercepts these requests
// and applies a governance layer.
//
// See also: aibridged/middleware.go.
func (api *API) bridgeAIRequest(rw http.ResponseWriter, r *http.Request) {
	srv := api.AIBridgeServer
	if srv == nil {
		http.Error(rw, "no AI bridge daemon running", http.StatusBadGateway)
		return
	}

	ctx := r.Context()

	actor, set := dbauthz.ActorFromContext(ctx)
	if !set {
		api.Logger.Error(ctx, "missing dbauthz actor in context")
		http.Error(rw, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Identify the initiator using a header known to aibridge lib.
	r.Header.Set(aibridge.InitiatorHeaderKey, actor.ID)

	userID, err := uuid.Parse(actor.ID)
	if err != nil {
		api.Logger.Error(ctx, "actor ID is not a uuid", slog.Error(err), slog.F("user_id", actor.ID))
		http.Error(rw, "internal server error", http.StatusInternalServerError)
		return
	}

	sessionKey, ok := ctx.Value(aibridged.ContextKeyBridgeAPIKey{}).(string)
	if sessionKey == "" || !ok {
		http.Error(rw, "unable to retrieve request session key", http.StatusBadRequest)
		return
	}

	handler, err := srv.GetRequestHandler(ctx, aibridged.Request{
		SessionKey:  sessionKey,
		InitiatorID: userID,
		RequestID:   httpmw.RequestID(r),
	})
	if err != nil {
		api.Logger.Error(ctx, "failed to handle request", slog.Error(err))
		http.Error(rw, "failed to handle request", http.StatusInternalServerError)
		return
	}
	http.StripPrefix("/api/v2/aibridge", handler).ServeHTTP(rw, r)
}
