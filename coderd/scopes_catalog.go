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
// @ID list-api-key-scopes
// @Tags Authorization
// @Produce json
// @Success 200 {object} codersdk.ExternalAPIKeyScopes
// @Router /auth/scopes [get]
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
