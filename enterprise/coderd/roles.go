package coderd

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

type enterpriseCustomRoleHandler struct {
	Enabled bool
}

func (h enterpriseCustomRoleHandler) PatchOrganizationRole(ctx context.Context, db database.Store, rw http.ResponseWriter, orgID uuid.UUID, role codersdk.Role) (codersdk.Role, bool) {
	if !h.Enabled {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Custom roles are not enabled",
		})
		return codersdk.Role{}, false
	}

	if err := httpapi.NameValid(role.Name); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid role name",
			Detail:  err.Error(),
		})
		return codersdk.Role{}, false
	}

	// Only organization permissions are allowed to be granted
	if len(role.SitePermissions) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid request, not allowed to assign site wide permissions for an organization role.",
			Detail:  "organization scoped roles may not contain site wide permissions",
		})
		return codersdk.Role{}, false
	}

	if len(role.UserPermissions) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid request, not allowed to assign user permissions for an organization role.",
			Detail:  "organization scoped roles may not contain user permissions",
		})
		return codersdk.Role{}, false
	}

	if role.OrganizationID != orgID.String() {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid request, organization in role and url must match",
			Detail:  fmt.Sprintf("role organization=%q does not match URL=%q", role.OrganizationID, orgID.String()),
		})
		return codersdk.Role{}, false
	}

	inserted, err := db.UpsertCustomRole(ctx, database.UpsertCustomRoleParams{
		Name:        role.Name,
		DisplayName: role.DisplayName,
		OrganizationID: uuid.NullUUID{
			UUID:  orgID,
			Valid: true,
		},
		SitePermissions: db2sdk.List(role.SitePermissions, sdkPermissionToDB),
		OrgPermissions:  db2sdk.List(role.OrganizationPermissions, sdkPermissionToDB),
		UserPermissions: db2sdk.List(role.UserPermissions, sdkPermissionToDB),
	})
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return codersdk.Role{}, false
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to update role permissions",
			Detail:  err.Error(),
		})
		return codersdk.Role{}, false
	}

	return db2sdk.Role(inserted), true
}

func sdkPermissionToDB(p codersdk.Permission) database.CustomRolePermission {
	return database.CustomRolePermission{
		Negate:       p.Negate,
		ResourceType: string(p.ResourceType),
		Action:       policy.Action(p.Action),
	}
}
