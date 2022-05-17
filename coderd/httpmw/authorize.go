package httpmw

import (
	"context"
	"net/http"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
)

// User roles are the 'subject' field of Authorize()
type userRolesKey struct{}

// UserRoles returns the API key from the ExtractUserRoles handler.
func UserRoles(r *http.Request) database.GetAllUserRolesRow {
	apiKey, ok := r.Context().Value(userRolesKey{}).(database.GetAllUserRolesRow)
	if !ok {
		panic("developer error: user roles middleware not provided")
	}
	return apiKey
}

// ExtractUserRoles requires authentication using a valid API key.
func ExtractUserRoles(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			apiKey := APIKey(r)
			role, err := db.GetAllUserRoles(r.Context(), apiKey.UserID)
			if err != nil {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: "roles not found",
				})
				return
			}

			ctx := context.WithValue(r.Context(), userRolesKey{}, role)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
