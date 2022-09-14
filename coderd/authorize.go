package coderd

import (
	"net/http"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
)

func AuthorizeFilter[O rbac.Objecter](h *HTTPAuthorizer, r *http.Request, action rbac.Action, objects []O) ([]O, error) {
	roles := httpmw.UserAuthorization(r)
	objects, err := rbac.Filter(r.Context(), h.Authorizer, roles.ID.String(), roles.Roles, roles.Scope.ToRBAC(), action, objects)
	if err != nil {
		// Log the error as Filter should not be erroring.
		h.Logger.Error(r.Context(), "filter failed",
			slog.Error(err),
			slog.F("user_id", roles.ID),
			slog.F("username", roles.Username),
			slog.F("scope", roles.Scope),
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

// Authorize will return false if the user is not authorized to do the action.
// This function will log appropriately, but the caller must return an
// error to the api client.
// Eg:
//
//	if !api.Authorize(...) {
//		httpapi.Forbidden(rw)
//		return
//	}
func (api *API) Authorize(r *http.Request, action rbac.Action, object rbac.Objecter) bool {
	return api.httpAuth.Authorize(r, action, object)
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
func (h *HTTPAuthorizer) Authorize(r *http.Request, action rbac.Action, object rbac.Objecter) bool {
	roles := httpmw.UserAuthorization(r)
	err := h.Authorizer.ByRoleName(r.Context(), roles.ID.String(), roles.Roles, roles.Scope.ToRBAC(), action, object.RBACObject())
	if err != nil {
		// Log the errors for debugging
		internalError := new(rbac.UnauthorizedError)
		logger := h.Logger
		if xerrors.As(err, internalError) {
			logger = h.Logger.With(slog.F("internal", internalError.Internal()))
		}
		// Log information for debugging. This will be very helpful
		// in the early days
		logger.Warn(r.Context(), "unauthorized",
			slog.F("roles", roles.Roles),
			slog.F("user_id", roles.ID),
			slog.F("username", roles.Username),
			slog.F("scope", roles.Scope),
			slog.F("route", r.URL.Path),
			slog.F("action", action),
			slog.F("object", object),
		)

		return false
	}
	return true
}
