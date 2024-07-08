package httpmw

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// RequireAPIKeyOrWorkspaceProxyAuth is middleware that should be inserted after
// optional ExtractAPIKey and ExtractWorkspaceProxy middlewares to ensure one of
// the two authentication methods is provided.
//
// If both are provided, an error is returned to avoid misuse.
func RequireAPIKeyOrWorkspaceProxyAuth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, hasAPIKey := APIKeyOptional(r)
			_, hasWorkspaceProxy := WorkspaceProxyOptional(r)

			if hasAPIKey && hasWorkspaceProxy {
				httpapi.Write(r.Context(), w, http.StatusBadRequest, codersdk.Response{
					Message: "API key and external proxy authentication provided, but only one is allowed",
				})
				return
			}
			if !hasAPIKey && !hasWorkspaceProxy {
				httpapi.Write(r.Context(), w, http.StatusUnauthorized, codersdk.Response{
					Message: "API key or external proxy authentication required, but none provided",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAPIKeyOrWorkspaceAgent is middleware that should be inserted after
// optional ExtractAPIKey and ExtractWorkspaceAgent middlewares to ensure one of
// the two is provided.
//
// If both are provided an error is returned to avoid misuse.
func RequireAPIKeyOrWorkspaceAgent() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, hasAPIKey := APIKeyOptional(r)
			_, hasWorkspaceAgent := WorkspaceAgentOptional(r)

			if hasAPIKey && hasWorkspaceAgent {
				httpapi.Write(r.Context(), w, http.StatusBadRequest, codersdk.Response{
					Message: "API key and workspace agent token provided, but only one is allowed",
				})
				return
			}
			if !hasAPIKey && !hasWorkspaceAgent {
				httpapi.Write(r.Context(), w, http.StatusUnauthorized, codersdk.Response{
					Message: "API key or workspace agent token required, but none provided",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAPIKeyOrProvisionerDaemonAuth is middleware that should be inserted
// after optional ExtractAPIKey and ExtractProvisionerDaemonAuthenticated
// middlewares to ensure one of the two authentication methods is provided.
//
// If both are provided, an error is returned to avoid misuse.
func RequireAPIKeyOrProvisionerDaemonAuth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, hasAPIKey := APIKeyOptional(r)
			hasProvisionerDaemon := ProvisionerDaemonAuthenticated(r)

			if hasAPIKey && hasProvisionerDaemon {
				httpapi.Write(r.Context(), w, http.StatusBadRequest, codersdk.Response{
					Message: "API key and external provisioner authentication provided, but only one is allowed",
				})
				return
			}
			if !hasAPIKey && !hasProvisionerDaemon {
				httpapi.Write(r.Context(), w, http.StatusUnauthorized, codersdk.Response{
					Message: "API key or external provisioner authentication required, but none provided",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
