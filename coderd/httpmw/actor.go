package httpmw

import (
	"net/http"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

// RequireAPIKeyOrExternalProxyAuth is middleware that should be inserted after
// optional ExtractAPIKey and ExtractExternalProxy middlewares to ensure one of
// the two authentication methods is provided.
//
// If both are provided, an error is returned to avoid misuse.
func RequireAPIKeyOrExternalProxyAuth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, hasAPIKey := APIKeyOptional(r)
			_, hasExternalProxy := ExternalProxyOptional(r)

			if hasAPIKey && hasExternalProxy {
				httpapi.Write(r.Context(), w, http.StatusBadRequest, codersdk.Response{
					Message: "API key and external proxy authentication provided, but only one is allowed",
				})
				return
			}
			if !hasAPIKey && !hasExternalProxy {
				httpapi.Write(r.Context(), w, http.StatusUnauthorized, codersdk.Response{
					Message: "API key or external proxy authentication required, but none provided",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Actor is a function that returns the request authorization. If the request is
// unauthenticated, the second return value is false.
//
// If the request was authenticated with an API key, the actor will be the user
// associated with the API key as well as the API key permissions.
//
// If the request was authenticated with an external proxy token, the actor will
// be a fake system actor with full permissions.
func Actor(r *http.Request) (Authorization, bool) {
	userAuthz, ok := UserAuthorizationOptional(r)
	if ok {
		return userAuthz, true
	}

	proxy, ok := ExternalProxyOptional(r)
	if ok {
		return Authorization{
			Actor: rbac.Subject{
				ID: "proxy:" + proxy.ID.String(),
				// We don't have a system role currently so just use owner for now.
				// TODO: add a system role
				Roles:  rbac.RoleNames{rbac.RoleOwner()},
				Groups: []string{},
				Scope:  rbac.ScopeAll,
			},
			ActorName: "proxy_" + proxy.Name,
		}, true
	}

	return Authorization{}, false
}
