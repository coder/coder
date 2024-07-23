package httpmw

import (
	"context"
	"crypto/subtle"
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/provisionerkey"
	"github.com/coder/coder/v2/codersdk"
)

type provisionerDaemonContextKey struct{}

func ProvisionerDaemonAuthenticated(r *http.Request) bool {
	proxy, ok := r.Context().Value(provisionerDaemonContextKey{}).(bool)
	return ok && proxy
}

type ExtractProvisionerAuthConfig struct {
	DB              database.Store
	Optional        bool
	PSK             string
	MultiOrgEnabled bool
}

func ExtractProvisionerDaemonAuthenticated(opts ExtractProvisionerAuthConfig) func(next http.Handler) http.Handler {
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

			if !opts.MultiOrgEnabled {
				if opts.PSK == "" {
					handleOptional(http.StatusUnauthorized, codersdk.Response{
						Message: "External provisioner daemons not enabled",
					})
					return
				}

				fallbackToPSK(ctx, opts.PSK, next, w, r, handleOptional)
				return
			}

			key := r.Header.Get(codersdk.ProvisionerDaemonKey)
			if key == "" {
				if opts.PSK == "" {
					handleOptional(http.StatusUnauthorized, codersdk.Response{
						Message: "provisioner daemon key required",
					})
					return
				}

				fallbackToPSK(ctx, opts.PSK, next, w, r, handleOptional)
				return
			}

			id, keyValue, err := provisionerkey.Parse(key)
			if err != nil {
				handleOptional(http.StatusUnauthorized, codersdk.Response{
					Message: "provisioner daemon key invalid",
				})
				return
			}

			// nolint:gocritic // System must check if the provisioner key is valid.
			pk, err := opts.DB.GetProvisionerKeyByID(dbauthz.AsSystemRestricted(ctx), id)
			if err != nil {
				if httpapi.Is404Error(err) {
					handleOptional(http.StatusUnauthorized, codersdk.Response{
						Message: "provisioner daemon key invalid",
					})
					return
				}

				handleOptional(http.StatusInternalServerError, codersdk.Response{
					Message: "get provisioner daemon key: " + err.Error(),
				})
				return
			}

			if subtle.ConstantTimeCompare(pk.HashedSecret, provisionerkey.HashSecret(keyValue)) != 1 {
				handleOptional(http.StatusUnauthorized, codersdk.Response{
					Message: "provisioner daemon key invalid",
				})
				return
			}

			// The PSK does not indicate a specific provisioner daemon. So just
			// store a boolean so the caller can check if the request is from an
			// authenticated provisioner daemon.
			ctx = context.WithValue(ctx, provisionerDaemonContextKey{}, true)
			// nolint:gocritic // Authenticating as a provisioner daemon.
			ctx = dbauthz.AsOrganizationProvisionerd(ctx, pk.OrganizationID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func fallbackToPSK(ctx context.Context, psk string, next http.Handler, w http.ResponseWriter, r *http.Request, handleOptional func(code int, response codersdk.Response)) {
	token := r.Header.Get(codersdk.ProvisionerDaemonPSK)
	if subtle.ConstantTimeCompare([]byte(token), []byte(psk)) != 1 {
		handleOptional(http.StatusUnauthorized, codersdk.Response{
			Message: "provisioner daemon psk invalid",
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
}
