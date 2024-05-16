package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
	"github.com/coder/coder/v2/codersdk"
)

// patchRole will allow creating a custom role
//
// @Summary Upsert a custom site-wide role
// @ID upsert-a-custom-site-wide-role
// @Security CoderSessionToken
// @Produce json
// @Tags Members
// @Success 200 {array} codersdk.Role
// @Router /users/roles [patch]
func (api *API) patchRole(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req codersdk.Role
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if err := httpapi.NameValid(req.Name); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid role name",
			Detail:  err.Error(),
		})
		return
	}

	if len(req.OrganizationPermissions) > 0 {
		// Org perms should be assigned only in org specific roles. Otherwise,
		// it gets complicated to keep track of who can do what.
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid request, not allowed to assign organization permissions for a site wide role.",
			Detail:  "site wide roles may not contain organization specific permissions",
		})
		return
	}

	// Make sure all permissions inputted are valid according to our policy.
	rbacRole := db2sdk.RoleToRBAC(req)
	args, err := rolestore.ConvertRoleToDB(rbacRole)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid request",
			Detail:  err.Error(),
		})
		return
	}

	inserted, err := api.Database.UpsertCustomRole(ctx, database.UpsertCustomRoleParams{
		Name:            args.Name,
		DisplayName:     args.DisplayName,
		SitePermissions: args.SitePermissions,
		OrgPermissions:  args.OrgPermissions,
		UserPermissions: args.UserPermissions,
	})
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to update role permissions",
			Detail:  err.Error(),
		})
		return
	}

	convertedInsert, err := rolestore.ConvertDBRole(inserted)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Permissions were updated, unable to read them back out of the database.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.Role(convertedInsert))
}
