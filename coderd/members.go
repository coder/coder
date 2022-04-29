package coderd

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/rbac"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

func (api *api) putMemberRoles(rw http.ResponseWriter, r *http.Request) {
	// User is the user to modify
	// TODO: Until rbac authorize is implemented, only be able to change your
	//	own roles. This also means you can grant yourself whatever roles you want.
	user := httpmw.UserParam(r)
	apiKey := httpmw.APIKey(r)
	organization := httpmw.OrganizationParam(r)
	// TODO: @emyrk add proper `Authorize()` check here instead of a uuid match.
	//	Proper authorize should check the granted roles are able to given within
	//	the selected organization. Until then, allow anarchy
	if apiKey.UserID != user.ID {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: fmt.Sprintf("modifying other users is not supported at this time"),
		})
		return
	}

	var params codersdk.UpdateRoles
	if !httpapi.Read(rw, r, &params) {
		return
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

func (api *api) updateOrganizationMemberRoles(ctx context.Context, args database.UpdateMemberRolesParams) (database.OrganizationMember, error) {
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
