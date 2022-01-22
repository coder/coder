package httpmw

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi"

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
)

type userParamContextKey struct{}

// UserParam returns the user from the ExtractUserParam handler.
func UserParam(r *http.Request) database.User {
	user, ok := r.Context().Value(userParamContextKey{}).(database.User)
	if !ok {
		panic("developer error: user parameter middleware not provided")
	}
	return user
}

// ExtractUserParam extracts a user from an ID/username in the {user} URL parameter.
func ExtractUserParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			userID := chi.URLParam(r, "user")
			if userID == "" {
				httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
					Message: "user id or name must be provided",
				})
				return
			}
			if userID != "me" {
				httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
					Message: "getting non-personal users isn't supported yet",
				})
				return
			}
			apiKey := APIKey(r)
			user, err := db.GetUserByID(r.Context(), apiKey.UserID)
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: fmt.Sprintf("get user: %s", err.Error()),
				})
			}

			ctx := context.WithValue(r.Context(), userParamContextKey{}, user)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
