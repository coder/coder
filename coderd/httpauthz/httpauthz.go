package httpauthz

import (
	"net/http"

	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

// AuthorizationFilter takes a list of objects and returns the filtered list of
// objects that the user is authorized to perform the given action on.
// This is faster than calling Authorize() on each object.
func AuthorizationFilter[O rbac.Objecter](h *HTTPAuthorizer, r *http.Request, action policy.Action, objects []O) ([]O, error) {
	roles := httpmw.UserAuthorization(r.Context())
	objects, err := rbac.Filter(r.Context(), h.Authorizer, roles, action, objects)
	if err != nil {
		// Log the error as Filter should not be erroring.
		h.Logger.Error(r.Context(), "authorization filter failed",
			slog.Error(err),
			slog.F("user_id", roles.ID),
			slog.F("username", roles),
			slog.F("roles", roles.SafeRoleNames()),
			slog.F("scope", roles.SafeScopeName()),
			slog.F("route", r.URL.Path),
			slog.F("action", action),
		)
		return nil, err
	}
	return objects, nil
}

type HTTPAuthorizer struct {
	Authorizer rbac.Authorizer
	Logger     slog.Logger
}

// AuthorizeSQLFilter returns an authorization filter that can used in a
// SQL 'WHERE' clause. If the filter is used, the resulting rows returned
// from postgres are already authorized, and the caller does not need to
// call 'Authorize()' on the returned objects.
// Note the authorization is only for the given action and object type.
func (h *HTTPAuthorizer) AuthorizeSQLFilter(r *http.Request, action policy.Action, objectType string) (rbac.PreparedAuthorized, error) {
	roles := httpmw.UserAuthorization(r.Context())
	prepared, err := h.Authorizer.Prepare(r.Context(), roles, action, objectType)
	if err != nil {
		return nil, xerrors.Errorf("prepare filter: %w", err)
	}

	return prepared, nil
}

// Authorize will return false if the user is not authorized to do the action.
// This function will log appropriately, but the caller must return an
// error to the api client.
// Eg:
//
//	if !h.Authorize(...) {
//		httpapi.Forbidden(rw)
//		return
//	}
func (h *HTTPAuthorizer) Authorize(r *http.Request, action policy.Action, object rbac.Objecter) bool {
	roles := httpmw.UserAuthorization(r.Context())
	err := h.Authorizer.Authorize(r.Context(), roles, action, object.RBACObject())
	if err != nil {
		// Log the errors for debugging
		internalError := new(rbac.UnauthorizedError)
		logger := h.Logger
		if xerrors.As(err, internalError) {
			logger = h.Logger.With(slog.F("internal_error", internalError.Internal()))
		}
		// Log information for debugging. This will be very helpful
		// in the early days
		logger.Warn(r.Context(), "requester is not authorized to access the object",
			slog.F("roles", roles.SafeRoleNames()),
			slog.F("actor_id", roles.ID),
			slog.F("actor_name", roles),
			slog.F("scope", roles.SafeScopeName()),
			slog.F("route", r.URL.Path),
			slog.F("action", action),
			slog.F("object", object),
		)

		return false
	}
	return true
}
