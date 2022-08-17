package coderd

import (
	"fmt"
	"net/http"

	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
)

// assignableSiteRoles returns all site wide roles that can be assigned.
func (api *API) assignableSiteRoles(rw http.ResponseWriter, r *http.Request) {
	actorRoles := httpmw.AuthorizationUserRoles(r)
	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceRoleAssignment) {
		httpapi.Forbidden(rw)
		return
	}

	roles := rbac.SiteRoles()
	httpapi.Write(rw, http.StatusOK, assignableRoles(actorRoles.Roles, roles))
}

// assignableSiteRoles returns all site wide roles that can be assigned.
func (api *API) assignableOrgRoles(rw http.ResponseWriter, r *http.Request) {
	organization := httpmw.OrganizationParam(r)
	actorRoles := httpmw.AuthorizationUserRoles(r)

	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceOrgRoleAssignment.InOrg(organization.ID)) {
		httpapi.Forbidden(rw)
		return
	}

	roles := rbac.OrganizationRoles(organization.ID)
	httpapi.Write(rw, http.StatusOK, assignableRoles(actorRoles.Roles, roles))
}

func (api *API) checkPermissions(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)

	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceUser) {
		httpapi.ResourceNotFound(rw)
		return
	}

	// use the roles of the user specified, not the person making the request.
	roles, err := api.Database.GetAuthorizationUserRoles(r.Context(), user.ID)
	if err != nil {
		httpapi.Forbidden(rw)
		return
	}

	var params codersdk.UserAuthorizationRequest
	if !httpapi.Read(rw, r, &params) {
		return
	}

	response := make(codersdk.UserAuthorizationResponse)
	for k, v := range params.Checks {
		if v.Object.ResourceType == "" {
			httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Object's \"resource_type\" field must be defined for key %q.", k),
			})
			return
		}

		if v.Object.OwnerID == "me" {
			v.Object.OwnerID = roles.ID.String()
		}
		err := api.Authorizer.ByRoleName(r.Context(), roles.ID.String(), roles.Roles, rbac.Action(v.Action),
			rbac.Object{
				Owner: v.Object.OwnerID,
				OrgID: v.Object.OrganizationID,
				Type:  v.Object.ResourceType,
			})
		response[k] = err == nil
	}

	httpapi.Write(rw, http.StatusOK, response)
}

func convertRole(role rbac.Role) codersdk.Role {
	return codersdk.Role{
		DisplayName: role.DisplayName,
		Name:        role.Name,
	}
}

func assignableRoles(actorRoles []string, roles []rbac.Role) []codersdk.AssignableRoles {
	assignable := make([]codersdk.AssignableRoles, 0)
	for _, role := range roles {
		if role.DisplayName == "" {
			continue
		}
		assignable = append(assignable, codersdk.AssignableRoles{
			Role: codersdk.Role{
				Name:        role.Name,
				DisplayName: role.DisplayName,
			},
			Assignable: rbac.CanAssignRole(actorRoles, role.Name),
		})
	}
	return assignable
}
