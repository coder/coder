package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

// listExternalScopes returns the curated list of API key scopes (resource:action)
// requestable via the API.
//
// @Summary List API key scopes
// @Description Returns the names of the API key scopes you can request when you create a token, including `all`, `application_connect`, the `coder:*` composite scopes, and the `resource:action` scopes.
// @Description Use it to find valid values for `scopes` before you call `POST /api/v2/users/{user}/keys/tokens`.
// @Description Doesn't require a session token.
// @ID list-api-key-scopes
// @Tags Authorization
// @Produce json
// @Success 200 {object} codersdk.ExternalAPIKeyScopes
// @Router /api/v2/auth/scopes [get]
func (*API) listExternalScopes(rw http.ResponseWriter, r *http.Request) {
	scopes := rbac.ExternalScopeNames()
	external := make([]codersdk.APIKeyScope, 0, len(scopes))
	for _, scope := range scopes {
		external = append(external, codersdk.APIKeyScope(scope))
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.ExternalAPIKeyScopes{
		External: external,
	})
}
