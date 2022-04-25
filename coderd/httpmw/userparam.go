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
			var userID uuid.UUID
			if chi.URLParam(r, "user") == "me" {
				userID = APIKey(r).UserID
			} else {
				var ok bool
				userID, ok = parseUUID(rw, r, "user")
				if !ok {
					return
				}
			}

			apiKey := APIKey(r)
			if apiKey.UserID != userID {
				httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
					Message: "getting non-personal users isn't supported yet",
				})
				return
			}

			user, err := db.GetUserByID(r.Context(), userID)
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
