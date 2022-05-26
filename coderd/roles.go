package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
)

// assignableSiteRoles returns all site wide roles that can be assigned.
func (api *API) assignableSiteRoles(rw http.ResponseWriter, r *http.Request) {
	// TODO: @emyrk in the future, allow granular subsets of roles to be returned based on the
	// 	role of the user.

	if !api.Authorize(rw, r, rbac.ActionRead, rbac.ResourceRoleAssignment) {
		return
	}

	roles := rbac.SiteRoles()
	httpapi.Write(rw, http.StatusOK, convertRoles(roles))
}

// assignableSiteRoles returns all site wide roles that can be assigned.
func (api *API) assignableOrgRoles(rw http.ResponseWriter, r *http.Request) {
	// TODO: @emyrk in the future, allow granular subsets of roles to be returned based on the
	// 	role of the user.
	organization := httpmw.OrganizationParam(r)

	if !api.Authorize(rw, r, rbac.ActionRead, rbac.ResourceOrgRoleAssignment.InOrg(organization.ID)) {
		return
	}

	roles := rbac.OrganizationRoles(organization.ID)
	httpapi.Write(rw, http.StatusOK, convertRoles(roles))
}

func (api *API) checkPermissions(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)

	if !api.Authorize(rw, r, rbac.ActionRead, rbac.ResourceUser.WithOwner(user.ID.String())) {
		return
	}

	// use the roles of the user specified, not the person making the request.
	roles, err := api.Database.GetAllUserRoles(r.Context(), user.ID)
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
			httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
				Message: "'resource_type' must be defined",
			})
			return
		}

		if v.Object.OwnerID == "me" {
			v.Object.OwnerID = roles.ID.String()
		}
		err := api.Authorizer.ByRoleName(r.Context(), roles.ID.String(), roles.Roles, rbac.Action(v.Action),
			rbac.Object{
				ResourceID: v.Object.ResourceID,
				Owner:      v.Object.OwnerID,
				OrgID:      v.Object.OrganizationID,
				Type:       v.Object.ResourceType,
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

func convertRoles(roles []rbac.Role) []codersdk.Role {
	converted := make([]codersdk.Role, 0, len(roles))
	for _, role := range roles {
		converted = append(converted, convertRole(role))
	}
	return converted
}
