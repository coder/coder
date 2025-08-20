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

func (api *API) bridgeAIRequest(rw http.ResponseWriter, r *http.Request) {
	srv := api.AIBridgeServer
	if srv == nil {
		http.Error(rw, "no AI bridge daemon running", http.StatusBadGateway)
		return
	}

	ctx := r.Context()

	sessionKey, ok := r.Context().Value(aibridged.ContextKeyBridgeAPIKey{}).(string)
	if sessionKey == "" || !ok {
		http.Error(rw, "unable to retrieve request session key", http.StatusBadRequest)
		return
	}

	initiatorID, ok := r.Context().Value(aibridged.ContextKeyBridgeUserID{}).(uuid.UUID)
	if !ok {
		api.Logger.Error(r.Context(), "missing initiator ID in context")
		http.Error(rw, "unable to retrieve initiator", http.StatusBadRequest)
		return
	}

	r.Header.Set(aibridge.InitiatorHeaderKey, initiatorID.String())

	// Inject the initiator's scope into the scope.
	actor, _, err := httpmw.UserRBACSubject(ctx, api.Database, initiatorID, rbac.ScopeAll)
	if err != nil {
		api.Logger.Error(ctx, "failed to setup user RBAC context", slog.Error(err), slog.F("userID", initiatorID))
		http.Error(rw, "internal server error", http.StatusInternalServerError) // Don't leak reason as this might have security implications.
		return
	}

	ctx = dbauthz.As(ctx, actor)

	bridge, err := srv.Acquire(ctx, sessionKey, initiatorID, srv.Client)
	if err != nil {
		api.Logger.Error(ctx, "failed to acquire aibridge", slog.Error(err))
		http.Error(rw, "failed to acquire aibridge", http.StatusInternalServerError)
		return
	}
	http.StripPrefix("/api/v2/aibridge", bridge.Handler()).ServeHTTP(rw, r)
}
