package httpmw

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

type provisionerDaemonContextKey struct{}

// ProvisionerDaemon returns the daemon from the ExtractProvisionerDaemon handler.
func ProvisionerDaemon(r *http.Request) database.ProvisionerDaemon {
	user, ok := r.Context().Value(provisionerDaemonContextKey{}).(database.ProvisionerDaemon)
	if !ok {
		panic("developer error: provisioner daemon middleware not provided")
	}
	return user
}

// ExtractWorkspaceAgent requires authentication using a valid provisioner token.
func ExtractProvisionerDaemon(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(codersdk.SessionTokenKey)
			if err != nil {
				httpapi.Write(rw, http.StatusUnauthorized, codersdk.Response{
					Message: fmt.Sprintf("Cookie %q must be provided.", codersdk.SessionTokenKey),
				})
				return
			}
			token, err := uuid.Parse(cookie.Value)
			if err != nil {
				httpapi.Write(rw, http.StatusUnauthorized, codersdk.Response{
					Message: "Provisioner token is invalid.",
				})
				return
			}
			provisioner, err := db.GetProvisionerDaemonByAuthToken(r.Context(), uuid.NullUUID{
				Valid: true,
				UUID:  token,
			})
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					httpapi.Write(rw, http.StatusUnauthorized, codersdk.Response{
						Message: "Provisioner token is invalid.",
					})
					return
				}

				httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching provisioner daemon.",
					Detail:  err.Error(),
				})
				return
			}

			ctx := context.WithValue(r.Context(), provisionerDaemonContextKey{}, provisioner)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
