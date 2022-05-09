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

const (
	// userErrorMessage is a constant so that no information about the state
	// of the queried user can be gained. We return the same error if the user
	// does not exist, or if the input is just garbage.
	userErrorMessage = "\"user\" must be an existing uuid or username"
)

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
					httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
						Message: userErrorMessage,
					})
					return
				}
			} else {
				// Try as a username last
				user, err = db.GetUserByEmailOrUsername(r.Context(), database.GetUserByEmailOrUsernameParams{
					Username: userQuery,
				})
				if err != nil {
					httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
						Message: userErrorMessage,
					})
					return
				}
			}

			ctx := context.WithValue(r.Context(), userParamContextKey{}, user)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
