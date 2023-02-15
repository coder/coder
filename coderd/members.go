package coderd

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/rbac"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

// @Summary Assign role to organization member
// @ID assign-role-to-organization-member
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Members
// @Param organization path string true "Organization ID"
// @Param user path string true "User ID, name, or me"
// @Param request body codersdk.UpdateRoles true "Update roles request"
// @Success 200 {object} codersdk.OrganizationMember
// @Router /organizations/{organization}/members/{user}/roles [put]
func (api *API) putMemberRoles(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx          = r.Context()
		user         = httpmw.UserParam(r)
		organization = httpmw.OrganizationParam(r)
		member       = httpmw.OrganizationMemberParam(r)
		apiKey       = httpmw.APIKey(r)
		actorRoles   = httpmw.UserAuthorization(r)
	)

	if apiKey.UserID == member.UserID {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "You cannot change your own organization roles.",
		})
		return
	}

	var params codersdk.UpdateRoles
	if !httpapi.Read(ctx, rw, r, &params) {
		return
	}

	// The org-member role is always implied.
	impliedTypes := append(params.Roles, rbac.RoleOrgMember(organization.ID))
	added, removed := rbac.ChangeRoleSet(member.Roles, impliedTypes)

	// Assigning a role requires the create permission.
	if len(added) > 0 && !api.Authorize(r, rbac.ActionCreate, rbac.ResourceOrgRoleAssignment.InOrg(organization.ID)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	// Removing a role requires the delete permission.
	if len(removed) > 0 && !api.Authorize(r, rbac.ActionDelete, rbac.ResourceOrgRoleAssignment.InOrg(organization.ID)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	// Just treat adding & removing as "assigning" for now.
	for _, roleName := range append(added, removed...) {
		if !rbac.CanAssignRole(actorRoles.Actor.Roles, roleName) {
			httpapi.ResourceNotFound(rw)
			return
		}
	}

	updatedUser, err := api.updateOrganizationMemberRoles(ctx, database.UpdateMemberRolesParams{
		GrantedRoles: params.Roles,
		UserID:       user.ID,
		OrgID:        organization.ID,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertOrganizationMember(updatedUser))
}

func (api *API) updateOrganizationMemberRoles(ctx context.Context, args database.UpdateMemberRolesParams) (database.OrganizationMember, error) {
	// Enforce only site wide roles
	for _, r := range args.GrantedRoles {
		// Must be an org role for the org in the args
		orgID, ok := rbac.IsOrgRole(r)
		if !ok {
			return database.OrganizationMember{}, xerrors.Errorf("must only update organization roles")
		}

		roleOrg, err := uuid.Parse(orgID)
		if err != nil {
			return database.OrganizationMember{}, xerrors.Errorf("Role must have proper UUIDs for organization, %q does not", r)
		}

		if roleOrg != args.OrgID {
			return database.OrganizationMember{}, xerrors.Errorf("Must only pass roles for org %q", args.OrgID.String())
		}

		if _, err := rbac.RoleByName(r); err != nil {
			return database.OrganizationMember{}, xerrors.Errorf("%q is not a supported role", r)
		}
	}

	updatedUser, err := api.Database.UpdateMemberRoles(ctx, args)
	if err != nil {
		return database.OrganizationMember{}, xerrors.Errorf("Update site roles: %w", err)
	}
	return updatedUser, nil
}

func convertOrganizationMember(mem database.OrganizationMember) codersdk.OrganizationMember {
	convertedMember := codersdk.OrganizationMember{
		UserID:         mem.UserID,
		OrganizationID: mem.OrganizationID,
		CreatedAt:      mem.CreatedAt,
		UpdatedAt:      mem.UpdatedAt,
		Roles:          make([]codersdk.Role, 0, len(mem.Roles)),
	}

	for _, roleName := range mem.Roles {
		rbacRole, _ := rbac.RoleByName(roleName)
		convertedMember.Roles = append(convertedMember.Roles, convertRole(rbacRole))
	}
	return convertedMember
}
