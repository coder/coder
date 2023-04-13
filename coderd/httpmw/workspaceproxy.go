package httpmw

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

const (
	// ExternalProxyAuthTokenHeader is the auth header used for requests from
	// external workspace proxies.
	//
	// The format of an external proxy token is:
	//     <proxy id>:<proxy secret>
	//
	//nolint:gosec
	ExternalProxyAuthTokenHeader = "Coder-External-Proxy-Token"
)

type externalProxyContextKey struct{}

// ExternalProxy may return the workspace proxy from the ExtractExternalProxy
// middleware.
func ExternalProxyOptional(r *http.Request) (database.WorkspaceProxy, bool) {
	proxy, ok := r.Context().Value(externalProxyContextKey{}).(database.WorkspaceProxy)
	return proxy, ok
}

// ExternalProxy returns the workspace proxy from the ExtractExternalProxy
// middleware.
func ExternalProxy(r *http.Request) database.WorkspaceProxy {
	proxy, ok := ExternalProxyOptional(r)
	if !ok {
		panic("developer error: ExtractExternalProxy middleware not provided")
	}
	return proxy
}

type ExtractExternalProxyConfig struct {
	DB database.Store
	// Optional indicates whether the middleware should be optional. If true,
	// any requests without the external proxy auth token header will be
	// allowed to continue and no workspace proxy will be set on the request
	// context.
	Optional bool
}

// ExtractExternalProxy extracts the external workspace proxy from the request
// using the external proxy auth token header.
func ExtractExternalProxy(opts ExtractExternalProxyConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			token := r.Header.Get(ExternalProxyAuthTokenHeader)
			if token == "" {
				if opts.Optional {
					next.ServeHTTP(w, r)
					return
				}

				httpapi.Write(ctx, w, http.StatusUnauthorized, codersdk.Response{
					Message: "Missing required external proxy token",
				})
				return
			}

			// Split the token and lookup the corresponding workspace proxy.
			parts := strings.Split(token, ":")
			if len(parts) != 2 {
				httpapi.Write(ctx, w, http.StatusUnauthorized, codersdk.Response{
					Message: "Invalid external proxy token",
				})
				return
			}
			proxyID, err := uuid.Parse(parts[0])
			if err != nil {
				httpapi.Write(ctx, w, http.StatusUnauthorized, codersdk.Response{
					Message: "Invalid external proxy token",
				})
				return
			}
			secret := parts[1]
			if len(secret) != 64 {
				httpapi.Write(ctx, w, http.StatusUnauthorized, codersdk.Response{
					Message: "Invalid external proxy token",
				})
				return
			}

			// Get the proxy.
			// nolint:gocritic // Get proxy by ID to check auth token
			proxy, err := opts.DB.GetWorkspaceProxyByID(dbauthz.AsSystemRestricted(ctx), proxyID)
			if xerrors.Is(err, sql.ErrNoRows) {
				// Proxy IDs are public so we don't care about leaking them via
				// timing attacks.
				httpapi.Write(ctx, w, http.StatusUnauthorized, codersdk.Response{
					Message: "Invalid external proxy token",
					Detail:  "Proxy not found.",
				})
				return
			}
			if err != nil {
				httpapi.InternalServerError(w, err)
				return
			}
			if proxy.Deleted {
				httpapi.Write(ctx, w, http.StatusUnauthorized, codersdk.Response{
					Message: "Invalid external proxy token",
					Detail:  "Proxy has been deleted.",
				})
				return
			}

			// Do a subtle constant time comparison of the hash of the secret.
			hashedSecret := sha256.Sum256([]byte(secret))
			if subtle.ConstantTimeCompare(proxy.TokenHashedSecret, hashedSecret[:]) != 1 {
				httpapi.Write(ctx, w, http.StatusUnauthorized, codersdk.Response{
					Message: "Invalid external proxy token",
					Detail:  "Invalid proxy token secret.",
				})
				return
			}

			ctx = r.Context()
			ctx = context.WithValue(ctx, externalProxyContextKey{}, proxy)
			//nolint:gocritic // Workspace proxies have full permissions. The
			// workspace proxy auth middleware is not mounted to every route, so
			// they can still only access the routes that the middleware is
			// mounted to.
			ctx = dbauthz.AsSystemRestricted(ctx)
			subj, ok := dbauthz.ActorFromContext(ctx)
			if !ok {
				// This should never happen
				httpapi.InternalServerError(w, xerrors.New("developer error: ExtractExternalProxy missing rbac actor"))
				return
			}
			// Use the same subject for the userAuthKey
			ctx = context.WithValue(ctx, userAuthKey{}, Authorization{
				Actor:     subj,
				ActorName: "proxy_" + proxy.Name,
			})

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
