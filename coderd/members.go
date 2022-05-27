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

func (api *API) putMemberRoles(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)
	organization := httpmw.OrganizationParam(r)
	member := httpmw.OrganizationMemberParam(r)

	var params codersdk.UpdateRoles
	if !httpapi.Read(rw, r, &params) {
		return
	}

	added, removed := rbac.ChangeRoleSet(member.Roles, params.Roles)
	for _, roleName := range added {
		// Assigning a role requires the create permission.
		if !api.Authorize(rw, r, rbac.ActionCreate, rbac.ResourceOrgRoleAssignment.WithID(roleName).InOrg(organization.ID)) {
			return
		}
	}
	for _, roleName := range removed {
		// Removing a role requires the delete permission.
		if !api.Authorize(rw, r, rbac.ActionDelete, rbac.ResourceOrgRoleAssignment.WithID(roleName).InOrg(organization.ID)) {
			return
		}
	}

	updatedUser, err := api.updateOrganizationMemberRoles(r.Context(), database.UpdateMemberRolesParams{
		GrantedRoles: params.Roles,
		UserID:       user.ID,
		OrgID:        organization.ID,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertOrganizationMember(updatedUser))
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
			return database.OrganizationMember{}, xerrors.Errorf("role must have proper uuids for organization, %q does not", r)
		}

		if roleOrg != args.OrgID {
			return database.OrganizationMember{}, xerrors.Errorf("must only pass roles for org %q", args.OrgID.String())
		}

		if _, err := rbac.RoleByName(r); err != nil {
			return database.OrganizationMember{}, xerrors.Errorf("%q is not a supported role", r)
		}
	}

	updatedUser, err := api.Database.UpdateMemberRoles(ctx, args)
	if err != nil {
		return database.OrganizationMember{}, xerrors.Errorf("update site roles: %w", err)
	}
	return updatedUser, nil
}

func convertOrganizationMember(mem database.OrganizationMember) codersdk.OrganizationMember {
	return codersdk.OrganizationMember{
		UserID:         mem.UserID,
		OrganizationID: mem.OrganizationID,
		CreatedAt:      mem.CreatedAt,
		UpdatedAt:      mem.UpdatedAt,
		Roles:          mem.Roles,
	}
}
