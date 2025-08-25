package coderd

import (
	"net/http"

	"cdr.dev/slog"

	"github.com/google/uuid"

	"github.com/coder/aibridge"
	"github.com/coder/coder/v2/aibridged"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
)

// bridgeAIRequest handles requests destined for an upstream AI provider; aibridged intercepts these requests
// and applies a governance layer.
func (api *API) bridgeAIRequest(rw http.ResponseWriter, r *http.Request) {
	srv := api.AIBridgeServer
	if srv == nil {
		http.Error(rw, "no AI bridge daemon running", http.StatusBadGateway)
		return
	}

	ctx := r.Context()

	sessionKey, ok := ctx.Value(aibridged.ContextKeyBridgeAPIKey{}).(string)
	if sessionKey == "" || !ok {
		http.Error(rw, "unable to retrieve request session key", http.StatusBadRequest)
		return
	}

	initiatorID, ok := ctx.Value(aibridged.ContextKeyBridgeUserID{}).(uuid.UUID)
	if !ok {
		api.Logger.Error(ctx, "missing initiator ID in context")
		http.Error(rw, "unable to retrieve initiator", http.StatusBadRequest)
		return
	}

	// Identify
	r.Header.Set(aibridge.InitiatorHeaderKey, initiatorID.String())

	// Inject the initiator's RBAC subject into the scope so all actions occur on their behalf.
	actor, _, err := httpmw.UserRBACSubject(ctx, api.Database, initiatorID, rbac.ScopeAll)
	if err != nil {
		api.Logger.Error(ctx, "failed to setup user RBAC context", slog.Error(err), slog.F("userID", initiatorID))
		http.Error(rw, "internal server error", http.StatusInternalServerError) // Don't leak reason as this might have security implications.
		return
	}
	ctx = dbauthz.As(ctx, actor)

	handler, err := srv.GetRequestHandler(ctx, aibridged.Request{
		SessionKey:  sessionKey,
		InitiatorID: initiatorID,
	})
	if err != nil {
		api.Logger.Error(ctx, "failed to handle request", slog.Error(err))
		http.Error(rw, "failed to handle request", http.StatusInternalServerError)
		return
	}
	http.StripPrefix("/api/v2/aibridge", handler).ServeHTTP(rw, r)
}
