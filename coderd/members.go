package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/rbac"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
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
		organization = httpmw.OrganizationParam(r)
		member       = httpmw.OrganizationMemberParam(r)
		apiKey       = httpmw.APIKey(r)
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

	updatedUser, err := api.Database.UpdateMemberRoles(ctx, database.UpdateMemberRolesParams{
		GrantedRoles: params.Roles,
		UserID:       member.UserID,
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

func convertOrganizationMember(mem database.OrganizationMember) codersdk.OrganizationMember {
	convertedMember := codersdk.OrganizationMember{
		UserID:         mem.UserID,
		OrganizationID: mem.OrganizationID,
		CreatedAt:      mem.CreatedAt,
		UpdatedAt:      mem.UpdatedAt,
		Roles:          make([]codersdk.SlimRole, 0, len(mem.Roles)),
	}

	for _, roleName := range mem.Roles {
		rbacRole, _ := rbac.RoleByName(rbac.RoleIdentifier{Name: roleName, OrganizationID: mem.OrganizationID})
		convertedMember.Roles = append(convertedMember.Roles, db2sdk.SlimRole(rbacRole))
	}
	return convertedMember
}
