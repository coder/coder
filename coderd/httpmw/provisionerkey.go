package httpmw

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

type provisionerKeyParamContextKey struct{}

// ProvisionerKeyParam returns the user from the ExtractProvisionerKeyParam handler.
func ProvisionerKeyParam(r *http.Request) database.ProvisionerKey {
	user, ok := r.Context().Value(provisionerKeyParamContextKey{}).(database.ProvisionerKey)
	if !ok {
		panic("developer error: provisioner key parameter middleware not provided")
	}
	return user
}

// ExtractProvisionerKeyParam extracts a provisioner key from a name in the {provisionerKey} URL
// parameter.
func ExtractProvisionerKeyParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			organization := OrganizationParam(r)

			provisionerKeyQuery := chi.URLParam(r, "provisionerkey")
			if provisionerKeyQuery == "" {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "\"provisionerkey\" must be provided.",
				})
				return
			}

			provisionerKey, err := db.GetProvisionerKeyByName(ctx, database.GetProvisionerKeyByNameParams{
				OrganizationID: organization.ID,
				Name:           provisionerKeyQuery,
			})
			if httpapi.Is404Error(err) {
				httpapi.ResourceNotFound(rw)
				return
			}
			if err != nil {
				httpapi.InternalServerError(rw, err)
				return
			}

			ctx = context.WithValue(ctx, provisionerKeyParamContextKey{}, provisionerKey)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
