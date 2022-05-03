package httpmw

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
)

type AuthObject struct {
	// WithUser sets the owner of the object to the value returned by the func
	WithUser func(r *http.Request) uuid.UUID

	// InOrg sets the org owner of the object to the value returned by the func
	InOrg func(r *http.Request) uuid.UUID

	// WithOwner sets the object id to the value returned by the func
	WithOwner func(r *http.Request) uuid.UUID

	// Object is that base static object the above functions can modify.
	Object rbac.Object
	//// Actions are the various actions the middleware will check can be done on the object.
	//Actions []rbac.Action
}

func WithOwner(owner func(r *http.Request) database.User) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ao := GetAuthObject(r)
			ao.WithOwner = func(r *http.Request) uuid.UUID {
				return owner(r).ID
			}

			ctx := context.WithValue(r.Context(), authObjectKey{}, ao)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

func InOrg(org func(r *http.Request) database.Organization) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ao := GetAuthObject(r)
			ao.InOrg = func(r *http.Request) uuid.UUID {
				return org(r).ID
			}

			ctx := context.WithValue(r.Context(), authObjectKey{}, ao)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

// Authorize allows for static object & action authorize checking. If the object is a static object, this is an easy way
// to enforce the route.
func Authorize(logger slog.Logger, auth *rbac.RegoAuthorizer, actions ...rbac.Action) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			roles := UserRoles(r)
			args := GetAuthObject(r)

			object := args.Object
			if args.InOrg != nil {
				object.InOrg(args.InOrg(r))
			}
			if args.WithUser != nil {
				object.WithOwner(args.InOrg(r).String())
			}
			if args.WithOwner != nil {
				object.WithID(args.InOrg(r).String())
			}

			// Error on the first action that fails
			for _, act := range actions {
				err := auth.AuthorizeByRoleName(r.Context(), roles.ID.String(), roles.Roles, act, object)
				if err != nil {
					var internalError *rbac.UnauthorizedError
					if xerrors.As(err, internalError) {
						logger = logger.With(slog.F("internal", internalError.Internal()))
					}
					logger.Warn(r.Context(), "unauthorized",
						slog.F("roles", roles.Roles),
						slog.F("user_id", roles.ID),
						slog.F("username", roles.Username),
						slog.F("route", r.URL.Path),
						slog.F("action", act),
						slog.F("object", object),
					)
					httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
						Message: err.Error(),
					})
					return
				}
			}
			next.ServeHTTP(rw, r)
		})
	}
}

type authObjectKey struct{}

// APIKey returns the API key from the ExtractAPIKey handler.
func GetAuthObject(r *http.Request) AuthObject {
	obj, ok := r.Context().Value(authObjectKey{}).(AuthObject)
	if !ok {
		return AuthObject{}
	}
	return obj
}

func Object(object rbac.Object) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ao := GetAuthObject(r)
			ao.Object = object

			ctx := context.WithValue(r.Context(), authObjectKey{}, ao)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

// User roles are the 'subject' field of Authorize()
type userRolesKey struct{}

// APIKey returns the API key from the ExtractAPIKey handler.
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
					Message: fmt.Sprintf("roles not found", AuthCookie),
				})
				return
			}

			ctx := context.WithValue(r.Context(), userRolesKey{}, role)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
