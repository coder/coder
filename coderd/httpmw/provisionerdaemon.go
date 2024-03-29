package httpmw

import (
	"context"
	"crypto/subtle"
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

type provisionerDaemonContextKey struct{}

func ProvisionerDaemonAuthenticated(r *http.Request) bool {
	proxy, ok := r.Context().Value(provisionerDaemonContextKey{}).(bool)
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

			handleOptional := func(code int, response codersdk.Response) {
				if opts.Optional {
					next.ServeHTTP(w, r)
					return
				}
				httpapi.Write(ctx, w, code, response)
			}

			if psk == "" {
				// No psk means external provisioner daemons are not allowed.
				// So their auth is not valid.
				handleOptional(http.StatusBadRequest, codersdk.Response{
					Message: "External provisioner daemons not enabled",
				})
				return
			}

			token := r.Header.Get(codersdk.ProvisionerDaemonPSK)
			if token == "" {
				handleOptional(http.StatusUnauthorized, codersdk.Response{
					Message: "provisioner daemon auth token required",
				})
				return
			}

			if subtle.ConstantTimeCompare([]byte(token), []byte(psk)) != 1 {
				handleOptional(http.StatusUnauthorized, codersdk.Response{
					Message: "provisioner daemon auth token invalid",
				})
				return
			}

			// The PSK does not indicate a specific provisioner daemon. So just
			// store a boolean so the caller can check if the request is from an
			// authenticated provisioner daemon.
			ctx = context.WithValue(ctx, provisionerDaemonContextKey{}, true)
			// nolint:gocritic // Authenticating as a provisioner daemon.
			ctx = dbauthz.AsProvisionerd(ctx)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
