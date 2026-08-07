package httpmw

import (
	"context"
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
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

// ExtractMCPServerConfigParam grabs an MCP server config from the
// "mcpserverconfig" URL parameter. dbauthz conceals unauthorized reads
// as not-found, so read-denied callers receive the same 404 as a
// missing row.
func ExtractMCPServerConfigParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			configID, parsed := ParseUUIDParam(rw, r, "mcpserverconfig")
			if !parsed {
				return
			}

			config, err := db.GetMCPServerConfigByID(ctx, configID)
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

			ctx = context.WithValue(ctx, mcpServerConfigParamContextKey{}, config)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
