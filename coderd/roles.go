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
	ctx := r.Context()
	actorRoles := httpmw.UserAuthorization(r)
	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceRoleAssignment) {
		httpapi.Forbidden(rw)
		return
	}

	roles := rbac.SiteRoles()
	httpapi.Write(ctx, rw, http.StatusOK, assignableRoles(actorRoles.Roles, roles))
}

// @Summary Get member roles by organization
// @ID get-member-roles-by-organization
// @Security CoderSessionToken
// @Produce json
// @Tags Members
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {array} codersdk.AssignableRoles
// @Router /organizations/{organization}/members/roles [get]
//
// assignableSiteRoles returns all site wide roles that can be assigned.
func (api *API) assignableOrgRoles(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)
	actorRoles := httpmw.UserAuthorization(r)

	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceOrgRoleAssignment.InOrg(organization.ID)) {
		httpapi.Forbidden(rw)
		return
	}

	roles := rbac.OrganizationRoles(organization.ID)
	httpapi.Write(ctx, rw, http.StatusOK, assignableRoles(actorRoles.Roles, roles))
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
