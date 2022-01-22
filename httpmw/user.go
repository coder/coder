package httpmw

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
)

type userContextKey struct{}

// User returns the user from the ExtractUser handler.
func User(r *http.Request) database.User {
	user, ok := r.Context().Value(userContextKey{}).(database.User)
	if !ok {
		panic("developer error: user middleware not provided")
	}
	return user
}

// ExtractUser consumes an API key and queries the user attached to it.
// It attaches the user to the request context.
func ExtractUser(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// The user handler depends on API Key to get the authenticated user.
			apiKey := APIKey(r)

			user, err := db.GetUserByID(r.Context(), apiKey.UserID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
						Message: "user not found for api key",
					})
					return
				}
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: fmt.Sprintf("couldn't fetch user for api key: %s", err.Error()),
				})
				return
			}

			ctx := context.WithValue(r.Context(), userContextKey{}, user)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
