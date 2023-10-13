package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
)

// assignableSiteRoles returns all site wide roles that can be assigned.
//
// @Summary Get site member roles
// @ID get-site-member-roles
// @Security CoderSessionToken
// @Produce json
// @Tags Members
// @Success 200 {array} codersdk.AssignableRoles
// @Router /users/roles [get]
func (api *API) assignableSiteRoles(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actorRoles := httpmw.UserAuthorization(r)
	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceRoleAssignment) {
		httpapi.Forbidden(rw)
		return
	}

	roles := rbac.SiteRoles()
	httpapi.Write(ctx, rw, http.StatusOK, assignableRoles(actorRoles.Actor.Roles, roles))
}

// assignableSiteRoles returns all org wide roles that can be assigned.
//
// @Summary Get member roles by organization
// @ID get-member-roles-by-organization
// @Security CoderSessionToken
// @Produce json
// @Tags Members
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {array} codersdk.AssignableRoles
// @Router /organizations/{organization}/members/roles [get]
func (api *API) assignableOrgRoles(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)
	actorRoles := httpmw.UserAuthorization(r)

	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceOrgRoleAssignment.InOrg(organization.ID)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	roles := rbac.OrganizationRoles(organization.ID)
	httpapi.Write(ctx, rw, http.StatusOK, assignableRoles(actorRoles.Actor.Roles, roles))
}

func assignableRoles(actorRoles rbac.ExpandableRoles, roles []rbac.Role) []codersdk.AssignableRoles {
	assignable := make([]codersdk.AssignableRoles, 0)
	for _, role := range roles {
		// The member role is implied, and not assignable.
		// If there is no display name, then the role is also unassigned.
		// This is not the ideal logic, but works for now.
		if role.Name == rbac.RoleMember() || (role.DisplayName == "") {
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
