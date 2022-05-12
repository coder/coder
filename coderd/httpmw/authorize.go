package httpmw

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
)

// Authorize will enforce if the user roles can complete the action on the RBACObject.
// The organization and owner are found using the ExtractOrganization and
// ExtractUser middleware if present.
func Authorize(logger slog.Logger, auth rbac.Authorizer, actions ...rbac.Action) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			roles := UserRoles(r)
			authObject := rbacObject(r)
			object := authObject.Object

			if object.Type == "" {
				panic("developer error: auth object has no type")
			}

			// First extract the object's owner and organization if present.
			unknownOrg := r.Context().Value(organizationParamContextKey{})
			if organization, castOK := unknownOrg.(database.Organization); unknownOrg != nil {
				if !castOK {
					panic("developer error: organization param middleware not provided for authorize")
				}
				object = object.InOrg(organization.ID)
			}

			if authObject.WithOwner != nil {
				owner := authObject.WithOwner(r)
				object = object.WithOwner(owner.String())
			} else {
				// Attempt to find the resource owner id
				unknownOwner := r.Context().Value(userParamContextKey{})
				if owner, castOK := unknownOwner.(database.User); unknownOwner != nil {
					if !castOK {
						panic("developer error: user param middleware not provided for authorize")
					}
					object = object.WithOwner(owner.ID.String())
				}
			}

			for _, action := range actions {
				err := auth.ByRoleName(r.Context(), roles.ID.String(), roles.Roles, action, object)
				if err != nil {
					internalError := new(rbac.UnauthorizedError)
					if xerrors.As(err, internalError) {
						logger = logger.With(slog.F("internal", internalError.Internal()))
					}
					// Log information for debugging. This will be very helpful
					// in the early days if we over secure endpoints.
					logger.Warn(r.Context(), "unauthorized",
						slog.F("roles", roles.Roles),
						slog.F("user_id", roles.ID),
						slog.F("username", roles.Username),
						slog.F("route", r.URL.Path),
						slog.F("action", action),
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

type RBACObject struct {
	Object rbac.Object

	WithOwner func(r *http.Request) uuid.UUID
}

// APIKey returns the API key from the ExtractAPIKey handler.
func rbacObject(r *http.Request) RBACObject {
	obj, ok := r.Context().Value(authObjectKey{}).(RBACObject)
	if !ok {
		panic("developer error: auth object middleware not provided")
	}
	return obj
}

func WithAPIKeyAsOwner() func(http.Handler) http.Handler {
	return WithOwner(func(r *http.Request) uuid.UUID {
		key := APIKey(r)
		return key.UserID
	})
}

// WithOwner sets the object owner for 'Authorize()' for all routes handled
// by this middleware.
func WithOwner(withOwner func(r *http.Request) uuid.UUID) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			obj, ok := r.Context().Value(authObjectKey{}).(RBACObject)
			if ok {
				obj.WithOwner = withOwner
			} else {
				obj = RBACObject{WithOwner: withOwner}
			}

			ctx := context.WithValue(r.Context(), authObjectKey{}, obj)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

// WithRBACObject sets the object for 'Authorize()' for all routes handled
// by this middleware. The important field to set is 'Type'
func WithRBACObject(object rbac.Object) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			obj, ok := r.Context().Value(authObjectKey{}).(RBACObject)
			if ok {
				obj.Object = object
			} else {
				obj = RBACObject{Object: object}
			}

			ctx := context.WithValue(r.Context(), authObjectKey{}, obj)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

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
