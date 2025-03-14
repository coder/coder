package httpmw
import (
	"errors"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"net/http"
	"strings"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)
const (
	// WorkspaceProxyAuthTokenHeader is the auth header used for requests from
	// external workspace proxies.
	//
	// The format of an external proxy token is:
	//     <proxy id>:<proxy secret>
	//
	//nolint:gosec
	WorkspaceProxyAuthTokenHeader = "Coder-External-Proxy-Token"
)
type workspaceProxyContextKey struct{}
// WorkspaceProxyOptional may return the workspace proxy from the ExtractWorkspaceProxy
// middleware.
func WorkspaceProxyOptional(r *http.Request) (database.WorkspaceProxy, bool) {
	proxy, ok := r.Context().Value(workspaceProxyContextKey{}).(database.WorkspaceProxy)
	return proxy, ok
}
// WorkspaceProxy returns the workspace proxy from the ExtractWorkspaceProxy
// middleware.
func WorkspaceProxy(r *http.Request) database.WorkspaceProxy {
	proxy, ok := WorkspaceProxyOptional(r)
	if !ok {
		panic("developer error: ExtractWorkspaceProxy middleware not provided")
	}
	return proxy
}
type ExtractWorkspaceProxyConfig struct {
	DB database.Store
	// Optional indicates whether the middleware should be optional. If true,
	// any requests without the external proxy auth token header will be
	// allowed to continue and no workspace proxy will be set on the request
	// context.
	Optional bool
}
// ExtractWorkspaceProxy extracts the external workspace proxy from the request
// using the external proxy auth token header.
func ExtractWorkspaceProxy(opts ExtractWorkspaceProxyConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			token := r.Header.Get(WorkspaceProxyAuthTokenHeader)
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
			if errors.Is(err, sql.ErrNoRows) {
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
			ctx = context.WithValue(ctx, workspaceProxyContextKey{}, proxy)
			//nolint:gocritic // Workspace proxies have full permissions. The
			// workspace proxy auth middleware is not mounted to every route, so
			// they can still only access the routes that the middleware is
			// mounted to.
			ctx = dbauthz.AsSystemRestricted(ctx)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
type workspaceProxyParamContextKey struct{}
// WorkspaceProxyParam returns the workspace proxy from the ExtractWorkspaceProxyParam handler.
func WorkspaceProxyParam(r *http.Request) database.WorkspaceProxy {
	user, ok := r.Context().Value(workspaceProxyParamContextKey{}).(database.WorkspaceProxy)
	if !ok {
		panic("developer error: workspace proxy parameter middleware not provided")
	}
	return user
}
// ExtractWorkspaceProxyParam extracts a workspace proxy from an ID/name in the {workspaceproxy} URL
// parameter.
//
//nolint:revive
func ExtractWorkspaceProxyParam(db database.Store, deploymentID string, fetchPrimaryProxy func(ctx context.Context) (database.WorkspaceProxy, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			proxyQuery := chi.URLParam(r, "workspaceproxy")
			if proxyQuery == "" {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "\"workspaceproxy\" must be provided.",
				})
				return
			}
			var proxy database.WorkspaceProxy
			var dbErr error
			if proxyQuery == "primary" || proxyQuery == deploymentID {
				// Requesting primary proxy
				proxy, dbErr = fetchPrimaryProxy(ctx)
			} else if proxyID, err := uuid.Parse(proxyQuery); err == nil {
				// Request proxy by id
				proxy, dbErr = db.GetWorkspaceProxyByID(ctx, proxyID)
			} else {
				// Request proxy by name
				proxy, dbErr = db.GetWorkspaceProxyByName(ctx, proxyQuery)
			}
			if httpapi.Is404Error(dbErr) {
				httpapi.ResourceNotFound(rw)
				return
			}
			if dbErr != nil {
				httpapi.InternalServerError(rw, dbErr)
				return
			}
			ctx = context.WithValue(ctx, workspaceProxyParamContextKey{}, proxy)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
