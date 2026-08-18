package httpmw

import (
	"context"
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

type mcpServerConfigParamContextKey struct{}

// MCPServerConfigParam returns the MCP server config from the
// ExtractMCPServerConfigParam handler.
func MCPServerConfigParam(r *http.Request) database.MCPServerConfig {
	config, ok := r.Context().Value(mcpServerConfigParamContextKey{}).(database.MCPServerConfig)
	if !ok {
		panic("developer error: mcp server config param middleware not provided")
	}
	return config
}

// ExtractMCPServerConfigParam reads the "mcpserverconfig" URL parameter.
// Callers with no read, update, or delete access are concealed as not found,
// so denied and missing rows both return 404.
func ExtractMCPServerConfigParam(
	db database.Store,
	auth func(r *http.Request, action policy.Action, object rbac.Objecter) bool,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			configID, parsed := ParseUUIDParam(rw, r, "mcpserverconfig")
			if !parsed {
				return
			}

			// Authorization follows the raw lookup because mutation-only callers
			// may lack the read access enforced by the database wrapper.
			//nolint:gocritic // The explicit action checks below own authorization.
			config, err := db.GetMCPServerConfigByID(dbauthz.AsSystemRestricted(ctx), configID)
			if httpapi.Is404Error(err) {
				httpapi.ResourceNotFound(rw)
				return
			}
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching MCP server config.",
					Detail:  err.Error(),
				})
				return
			}
			if config.OrganizationID != OrganizationParam(r).ID ||
				(!auth(r, policy.ActionRead, config) &&
					!auth(r, policy.ActionUpdate, config) &&
					!auth(r, policy.ActionDelete, config)) {
				httpapi.ResourceNotFound(rw)
				return
			}

			ctx = context.WithValue(ctx, mcpServerConfigParamContextKey{}, config)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
