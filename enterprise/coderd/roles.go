package coderd

import (
	"fmt"
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
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
// @Router /users/roles/ [patch]
func (api *API) patchRole(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserAuthorization(r)
	var req codersdk.RolePermissions
	if !httpapi.Read(ctx, rw, r, &req) {
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
	rbacRole := db2sdk.RolePermissionsDB(req)
	err := rbacRole.Valid()
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid request, at least 1 permissions is invalid",
			Detail:  err.Error(),
		})
		return
	}

	// Before we continue, make sure the caller has a superset of permissions.
	// If they do not, then creating this role is an escalation.
	for _, sitePerm := range rbacRole.Site {
		if !api.escalationCheck(r, rw, sitePerm, rbac.Object{Type: sitePerm.ResourceType}) {
			return
		}
	}

	for _, sitePerm := range rbacRole.User {
		// This feels a bit weak, since all users have all perms on their own resources.
		// So this check is not very strong.
		if !api.escalationCheck(r, rw, sitePerm, rbac.Object{Type: sitePerm.ResourceType, Owner: user.ID}) {
			return
		}
	}

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

func (api *API) escalationCheck(r *http.Request, rw http.ResponseWriter, perm rbac.Permission, object rbac.Object) bool {
	ctx := r.Context()
	if perm.Negate {
		// This is just an arbitrary choice to make things more simple for today.
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid request, negative permissions are not allowed in custom roles",
			Detail:  fmt.Sprintf("permission action %q, permission type %q", perm.Action, perm.ResourceType),
		})
		return false
	}

	// It is possible to check for supersets with wildcards, but wildcards can also
	// include resources and actions that do not exist. Custom roles should only be allowed
	// to include permissions for existing resources.
	if perm.Action == policy.WildcardSymbol || perm.ResourceType == policy.WildcardSymbol {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid request, wildcard symbols are not allows in custom roles",
			Detail:  fmt.Sprintf("permission action %q, permission type %q", perm.Action, perm.ResourceType),
		})
		return false
	}

	// Site wide resources only need the type.
	if !api.Authorize(r, perm.Action, object) {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Invalid request, caller permissions do not contain all request permissions",
			Detail:  fmt.Sprintf("not allowed to assign action %q on resource type %q", perm.Action, perm.ResourceType),
		})
	}
}
