package coderd

import (
	"fmt"
	"net/http"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

// AuthorizeFilter takes a list of objects and returns the filtered list of
// objects that the user is authorized to perform the given action on.
// This is faster than calling Authorize() on each object.
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
	return api.HTTPAuth.Authorize(r, action, object)
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

// AuthorizeSQLFilter returns an authorization filter that can used in a
// SQL 'WHERE' clause. If the filter is used, the resulting rows returned
// from postgres are already authorized, and the caller does not need to
// call 'Authorize()' on the returned objects.
// Note the authorization is only for the given action and object type.
func (h *HTTPAuthorizer) AuthorizeSQLFilter(r *http.Request, action rbac.Action, objectType string) (rbac.AuthorizeFilter, error) {
	roles := httpmw.UserAuthorization(r)
	prepared, err := h.Authorizer.PrepareByRoleName(r.Context(), roles.ID.String(), roles.Roles, roles.Scope.ToRBAC(), action, objectType)
	if err != nil {
		return nil, xerrors.Errorf("prepare filter: %w", err)
	}

	filter, err := prepared.Compile()
	if err != nil {
		return nil, xerrors.Errorf("compile filter: %w", err)
	}

	return filter, nil
}

// checkAuthorization returns if the current API key can use the given
// permissions, factoring in the current user's roles and the API key scopes.
func (api *API) checkAuthorization(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	auth := httpmw.UserAuthorization(r)

	var params codersdk.AuthorizationRequest
	if !httpapi.Read(ctx, rw, r, &params) {
		return
	}

	api.Logger.Debug(ctx, "check-auth",
		slog.F("my_id", httpmw.APIKey(r).UserID),
		slog.F("got_id", auth.ID),
		slog.F("name", auth.Username),
		slog.F("roles", auth.Roles), slog.F("scope", auth.Scope),
	)

	response := make(codersdk.AuthorizationResponse)
	for k, v := range params.Checks {
		if v.Object.ResourceType == "" {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Object's \"resource_type\" field must be defined for key %q.", k),
			})
			return
		}

		if v.Object.OwnerID == "me" {
			v.Object.OwnerID = auth.ID.String()
		}
		err := api.Authorizer.ByRoleName(r.Context(), auth.ID.String(), auth.Roles, auth.Scope.ToRBAC(), rbac.Action(v.Action),
			rbac.Object{
				Owner: v.Object.OwnerID,
				OrgID: v.Object.OrganizationID,
				Type:  v.Object.ResourceType,
			})
		response[k] = err == nil
	}

	httpapi.Write(ctx, rw, http.StatusOK, response)
}
