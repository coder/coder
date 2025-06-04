package httpmw

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

type userParamContextKey struct{}

const (
	// userErrorMessage is a constant so that no information about the state
	// of the queried user can be gained. We return the same error if the user
	// does not exist, or if the input is just garbage.
	userErrorMessage = "\"user\" must be an existing uuid or username."
)

// UserParam returns the user from the ExtractUserParam handler.
func UserParam(r *http.Request) database.User {
	user, ok := r.Context().Value(userParamContextKey{}).(database.User)
	if !ok {
		panic("developer error: user parameter middleware not provided")
	}
	return user
}

func UserParamOptional(r *http.Request) (database.User, bool) {
	user, ok := r.Context().Value(userParamContextKey{}).(database.User)
	return user, ok
}

// ExtractUserParam extracts a user from an ID/username in the {user} URL
// parameter.
func ExtractUserParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			user, ok := ExtractUserContext(ctx, db, rw, r)
			if !ok {
				// response already handled
				return
			}
			ctx = context.WithValue(ctx, userParamContextKey{}, user)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

// ExtractUserParamOptional does not fail if no user is present.
func ExtractUserParamOptional(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			user, ok := ExtractUserContext(ctx, db, &httpapi.NoopResponseWriter{}, r)
			if ok {
				ctx = context.WithValue(ctx, userParamContextKey{}, user)
			}

			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

// ExtractUserContext queries the database for the parameterized `{user}` from the request URL.
func ExtractUserContext(ctx context.Context, db database.Store, rw http.ResponseWriter, r *http.Request) (user database.User, ok bool) {
	// userQuery is either a uuid, a username, or 'me'
	userQuery := chi.URLParam(r, "user")
	if userQuery == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "\"user\" must be provided.",
		})
		return database.User{}, false
	}

	if userQuery == "me" {
		apiKey, ok := APIKeyOptional(r)
		if !ok {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Cannot use \"me\" without a valid session.",
			})
			return database.User{}, false
		}
		user, err := db.GetUserByID(ctx, apiKey.UserID)
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return database.User{}, false
		}
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching user.",
				Detail:  err.Error(),
			})
			return database.User{}, false
		}
		return user, true
	}

	if userID, err := uuid.Parse(userQuery); err == nil {
		user, err = db.GetUserByID(ctx, userID)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: userErrorMessage,
				Detail:  fmt.Sprintf("queried user=%q", userQuery),
			})
			return database.User{}, false
		}
		return user, true
	}

	// Try as a username last
	user, err := db.GetUserByEmailOrUsername(ctx, database.GetUserByEmailOrUsernameParams{
		Username: userQuery,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: userErrorMessage,
			Detail:  fmt.Sprintf("queried user=%q", userQuery),
		})
		return database.User{}, false
	}
	return user, true
}
