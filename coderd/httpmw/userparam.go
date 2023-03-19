package httpmw

import (
	"context"
	"database/sql"
	"net/http"

	"golang.org/x/xerrors"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
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

// ExtractUserParam extracts a user from an ID/username in the {user} URL
// parameter.
//
//nolint:revive
func ExtractUserParam(db database.Store, redirectToLoginOnMe bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			var (
				ctx  = r.Context()
				user database.User
				err  error
			)

			// userQuery is either a uuid, a username, or 'me'
			userQuery := chi.URLParam(r, "user")
			if userQuery == "" {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "\"user\" must be provided.",
				})
				return
			}

			if userQuery == "me" {
				apiKey, ok := APIKeyOptional(r)
				if !ok {
					if redirectToLoginOnMe {
						RedirectToLogin(rw, r, SignedOutErrorMessage)
						return
					}

					httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
						Message: "Cannot use \"me\" without a valid session.",
					})
					return
				}
				//nolint:gocritic // System needs to be able to get user from param.
				user, err = db.GetUserByID(dbauthz.AsSystemRestricted(ctx), apiKey.UserID)
				if xerrors.Is(err, sql.ErrNoRows) {
					httpapi.ResourceNotFound(rw)
					return
				}
				if err != nil {
					httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
						Message: "Internal error fetching user.",
						Detail:  err.Error(),
					})
					return
				}
			} else if userID, err := uuid.Parse(userQuery); err == nil {
				//nolint:gocritic // If the userQuery is a valid uuid
				user, err = db.GetUserByID(dbauthz.AsSystemRestricted(ctx), userID)
				if err != nil {
					httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
						Message: userErrorMessage,
					})
					return
				}
			} else {
				// nolint:gocritic // Try as a username last
				user, err = db.GetUserByEmailOrUsername(dbauthz.AsSystemRestricted(ctx), database.GetUserByEmailOrUsernameParams{
					Username: userQuery,
				})
				if err != nil {
					httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
						Message: userErrorMessage,
					})
					return
				}
			}

			ctx = context.WithValue(ctx, userParamContextKey{}, user)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
