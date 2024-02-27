package httpmw

import (
	"context"
	"crypto/subtle"
	"net/http"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

type provisionerDaemonContextKey struct{}

func ProvisionerDaemonAuthenticated(r *http.Request) bool {
	proxy, ok := r.Context().Value(workspaceProxyContextKey{}).(bool)
	return ok && proxy
}

type ExtractProvisionerAuthConfig struct {
	DB       database.Store
	Optional bool
}

func ExtractProvisionerDaemonAuthenticated(opts ExtractProvisionerAuthConfig, psk string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if psk == "" {
				if opts.Optional {
					next.ServeHTTP(w, r)
					return
				}
				// No psk means external provisioner daemons are not allowed.
				// So their auth is not valid.
				httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
					Message: "External provisioner daemons not enabled",
				})
				return
			}

			token := r.Header.Get(codersdk.ProvisionerDaemonPSK)
			if token == "" {
				if opts.Optional {
					next.ServeHTTP(w, r)
					return
				}
				httpapi.Write(ctx, w, http.StatusUnauthorized, codersdk.Response{
					Message: "provisioner daemon auth token required",
				})
				return
			}

			if subtle.ConstantTimeCompare([]byte(token), []byte(psk)) != 1 {
				httpapi.Write(ctx, w, http.StatusUnauthorized, codersdk.Response{
					Message: "provisioner daemon auth token invalid",
				})
				return
			}

			// The PSK does not indicate a specific provisioner daemon. So just
			// store a boolean so the caller can check if the request is from an
			// authenticated provisioner daemon.
			ctx = context.WithValue(ctx, provisionerDaemonContextKey{}, true)
			ctx = dbauthz.AsProvisionerd(ctx)
			subj, ok := dbauthz.ActorFromContext(ctx)
			if !ok {
				// This should never happen
				httpapi.InternalServerError(w, xerrors.New("developer error: ExtractProvisionerDaemonAuth missing rbac actor"))
			}

			// Use the same subject for the userAuthKey
			ctx = context.WithValue(ctx, userAuthKey{}, Authorization{
				Actor:     subj,
				ActorName: "provisioner_daemon",
			})

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
