package coderd

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
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
	objects, err := rbac.Filter(r.Context(), h.Authorizer, roles.Actor, action, objects)
	if err != nil {
		// Log the error as Filter should not be erroring.
		h.Logger.Error(r.Context(), "filter failed",
			slog.Error(err),
			slog.F("user_id", roles.Actor.ID),
			slog.F("username", roles.Username),
			slog.F("roles", roles.Actor.SafeRoleNames()),
			slog.F("scope", roles.Actor.SafeScopeName()),
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
	// The experiment does not replace ALL rbac checks, but does replace most.
	// This statement aborts early on the checks that will be removed in the
	// future when this experiment is default.
	if api.Experiments.Enabled(codersdk.ExperimentAuthzQuerier) {
		// Some resource types do not interact with the persistent layer and
		// we need to keep these checks happening in the API layer.
		switch object.RBACObject().Type {
		case rbac.ResourceWorkspaceExecution.Type:
			// This is not a db resource, always in API layer
		case rbac.ResourceDeploymentConfig.Type:
			// For metric cache items like DAU, we do not hit the DB.
			// Some db actions are in asserted in the authz layer.
		case rbac.ResourceReplicas.Type:
			// Replica rbac is checked for adding and removing replicas.
		case rbac.ResourceProvisionerDaemon.Type:
			// Provisioner rbac is checked for adding and removing provisioners.
		case rbac.ResourceDebugInfo.Type:
			// This is not a db resource, always in API layer.
		default:
			return true
		}
	}
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
	err := h.Authorizer.Authorize(r.Context(), roles.Actor, action, object.RBACObject())
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
			slog.F("roles", roles.Actor.SafeRoleNames()),
			slog.F("user_id", roles.Actor.ID),
			slog.F("username", roles.Username),
			slog.F("scope", roles.Actor.SafeScopeName()),
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
func (h *HTTPAuthorizer) AuthorizeSQLFilter(r *http.Request, action rbac.Action, objectType string) (rbac.PreparedAuthorized, error) {
	roles := httpmw.UserAuthorization(r)
	prepared, err := h.Authorizer.Prepare(r.Context(), roles.Actor, action, objectType)
	if err != nil {
		return nil, xerrors.Errorf("prepare filter: %w", err)
	}

	return prepared, nil
}

// checkAuthorization returns if the current API key can use the given
// permissions, factoring in the current user's roles and the API key scopes.
//
// @Summary Check authorization
// @ID check-authorization
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Authorization
// @Param request body codersdk.AuthorizationRequest true "Authorization request"
// @Success 200 {object} codersdk.AuthorizationResponse
// @Router /authcheck [post]
func (api *API) checkAuthorization(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	auth := httpmw.UserAuthorization(r)

	var params codersdk.AuthorizationRequest
	if !httpapi.Read(ctx, rw, r, &params) {
		return
	}

	api.Logger.Debug(ctx, "check-auth",
		slog.F("my_id", httpmw.APIKey(r).UserID),
		slog.F("got_id", auth.Actor.ID),
		slog.F("name", auth.Username),
		slog.F("roles", auth.Actor.SafeRoleNames()),
		slog.F("scope", auth.Actor.SafeScopeName()),
	)

	response := make(codersdk.AuthorizationResponse)
	// Prevent using too many resources by ID. This prevents database abuse
	// from this endpoint. This also prevents misuse of this endpoint, as
	// resource_id should be used for single objects, not for a list of them.
	var (
		idFetch  int
		maxFetch = 10
	)
	for _, v := range params.Checks {
		if v.Object.ResourceID != "" {
			idFetch++
		}
	}
	if idFetch > maxFetch {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf(
				"Endpoint only supports using \"resource_id\" field %d times, found %d usages. Remove %d objects with this field set.",
				maxFetch, idFetch, idFetch-maxFetch,
			),
		})
		return
	}

	for k, v := range params.Checks {
		if v.Object.ResourceType == "" {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Object's \"resource_type\" field must be defined for key %q.", k),
			})
			return
		}

		obj := rbac.Object{
			Owner: v.Object.OwnerID,
			OrgID: v.Object.OrganizationID,
			Type:  v.Object.ResourceType,
		}
		if obj.Owner == "me" {
			obj.Owner = auth.Actor.ID
		}

		// If a resource ID is specified, fetch that specific resource.
		if v.Object.ResourceID != "" {
			id, err := uuid.Parse(v.Object.ResourceID)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message:     fmt.Sprintf("Object %q id is not a valid uuid.", v.Object.ResourceID),
					Validations: []codersdk.ValidationError{{Field: "resource_id", Detail: err.Error()}},
				})
				return
			}

			var dbObj rbac.Objecter
			var dbErr error
			// Only support referencing some resources by ID.
			switch v.Object.ResourceType {
			case rbac.ResourceWorkspaceExecution.Type:
				wrkSpace, err := api.Database.GetWorkspaceByID(ctx, id)
				if err == nil {
					dbObj = wrkSpace.ExecutionRBAC()
				}
				dbErr = err
			case rbac.ResourceWorkspace.Type:
				dbObj, dbErr = api.Database.GetWorkspaceByID(ctx, id)
			case rbac.ResourceTemplate.Type:
				dbObj, dbErr = api.Database.GetTemplateByID(ctx, id)
			case rbac.ResourceUser.Type:
				dbObj, dbErr = api.Database.GetUserByID(ctx, id)
			case rbac.ResourceGroup.Type:
				dbObj, dbErr = api.Database.GetGroupByID(ctx, id)
			default:
				msg := fmt.Sprintf("Object type %q does not support \"resource_id\" field.", v.Object.ResourceType)
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message:     msg,
					Validations: []codersdk.ValidationError{{Field: "resource_type", Detail: msg}},
				})
				return
			}
			if dbErr != nil {
				// 404 or unauthorized is false
				response[k] = false
				continue
			}
			obj = dbObj.RBACObject()
		}

		err := api.Authorizer.Authorize(ctx, auth.Actor, rbac.Action(v.Action), obj)
		response[k] = err == nil
	}

	httpapi.Write(ctx, rw, http.StatusOK, response)
}
