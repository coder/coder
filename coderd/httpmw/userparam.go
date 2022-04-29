package httpmw

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
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
			var user database.User
			var err error

			// userQuery is either a uuid, a username, or 'me'
			userQuery := chi.URLParam(r, "user")
			if userQuery == "" {
				httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
					Message: "\"user\" must be provided",
				})
				return
			}

			if userQuery == "me" {
				user, err = db.GetUserByID(r.Context(), APIKey(r).UserID)
				if err != nil {
					httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
						Message: fmt.Sprintf("get user: %s", err.Error()),
					})
					return
				}
			} else if userID, err := uuid.Parse(userQuery); err == nil {
				// If the userQuery is a valid uuid
				user, err = db.GetUserByID(r.Context(), userID)
				if err != nil {
					httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
						Message: fmt.Sprintf("get user: %s", err.Error()),
					})
					return
				}
			} else {
				// Try as a username last
				user, err = db.GetUserByEmailOrUsername(r.Context(), database.GetUserByEmailOrUsernameParams{
					Username: userQuery,
				})
				if err != nil {
					// If the error is no rows, they might have inputted something
					// that is not a username or uuid. Regardless, let's not indicate if
					// the user exists or not. Just lump all these errors into
					// something generic.
					httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
						Message: "\"user\" must be a uuid or username",
					})
					return
				}
			}

			apiKey := APIKey(r)
			if apiKey.UserID != user.ID {
				httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
					Message: "getting non-personal users isn't supported yet",
				})
				return
			}

			ctx := context.WithValue(r.Context(), userParamContextKey{}, user)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
