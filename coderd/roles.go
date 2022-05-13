package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
)

// assignableSiteRoles returns all site wide roles that can be assigned.
func (*api) assignableSiteRoles(rw http.ResponseWriter, _ *http.Request) {
	// TODO: @emyrk in the future, allow granular subsets of roles to be returned based on the
	// 	role of the user.
	roles := rbac.SiteRoles()
	httpapi.Write(rw, http.StatusOK, convertRoles(roles))
}

// assignableSiteRoles returns all site wide roles that can be assigned.
func (*api) assignableOrgRoles(rw http.ResponseWriter, r *http.Request) {
	// TODO: @emyrk in the future, allow granular subsets of roles to be returned based on the
	// 	role of the user.
	organization := httpmw.OrganizationParam(r)
	roles := rbac.OrganizationRoles(organization.ID)
	httpapi.Write(rw, http.StatusOK, convertRoles(roles))
}

func (api *api) checkPermissions(rw http.ResponseWriter, r *http.Request) {
	roles := httpmw.UserRoles(r)
	user := httpmw.UserParam(r)
	if user.ID != roles.ID {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			// TODO: @Emyrk in the future we could have an rbac check here.
			//	If the user can masquerade/impersonate as the user passed in,
			//	we could allow this or something like that.
			Message: "only allowed to check permissions on yourself",
		})
		return
	}

	var params codersdk.UserPermissionCheckRequest
	if !httpapi.Read(rw, r, &params) {
		return
	}

	response := make(codersdk.UserPermissionCheckResponse)
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
		err := api.Authorizer.AuthorizeByRoleName(r.Context(), roles.ID.String(), roles.Roles, rbac.Action(v.Action),
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
