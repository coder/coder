package httpmw

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
)

// User roles are the 'subject' field of Authorize()
type userRolesKey struct{}

type AuthorizationScope struct {
	ID       uuid.UUID
	Username string
	Roles    []string
	Scope    database.ApiKeyScope
}

// UserRoles returns the API key from the ExtractUserRoles handler.
func UserRoles(r *http.Request) AuthorizationScope {
	apiKey, ok := r.Context().Value(userRolesKey{}).(AuthorizationScope)
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

			authScope := AuthorizationScope{
				ID:       role.ID,
				Username: role.Username,
				Roles:    role.Roles,
				Scope:    apiKey.Scope,
			}

			ctx := context.WithValue(r.Context(), userRolesKey{}, authScope)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
