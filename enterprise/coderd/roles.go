package coderd

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

// patchRole will allow creating a custom organization role
//
// @Summary Upsert a custom organization role
// @ID upsert-a-custom-organization-role
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.PatchRoleRequest true "Upsert role request"
// @Tags Members
// @Success 200 {array} codersdk.Role
// @Router /organizations/{organization}/members/roles [patch]
func (api *API) patchOrgRoles(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		db                = api.Database
		auditor           = api.AGPL.Auditor.Load()
		organization      = httpmw.OrganizationParam(r)
		aReq, commitAudit = audit.InitRequest[database.CustomRole](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionWrite,
			OrganizationID: organization.ID,
		})
	)
	defer commitAudit()

	var req codersdk.PatchRoleRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// This check is not ideal, but we cannot enforce a unique role name in the db against
	// the built-in role names.
	if rbac.ReservedRoleName(req.Name) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Reserved role name",
			Detail:  fmt.Sprintf("%q is a reserved role name, and not allowed to be used", req.Name),
		})
		return
	}

	if err := httpapi.NameValid(req.Name); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid role name",
			Detail:  err.Error(),
		})
		return
	}

	// Only organization permissions are allowed to be granted
	if len(req.SitePermissions) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid request, not allowed to assign site wide permissions for an organization role.",
			Detail:  "organization scoped roles may not contain site wide permissions",
		})
		return
	}

	if len(req.UserPermissions) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid request, not allowed to assign user permissions for an organization role.",
			Detail:  "organization scoped roles may not contain user permissions",
		})
		return
	}

	originalRoles, err := db.CustomRoles(ctx, database.CustomRolesParams{
		LookupRoles: []database.NameOrganizationPair{
			{
				Name:           req.Name,
				OrganizationID: organization.ID,
			},
		},
		ExcludeOrgRoles: false,
		OrganizationID:  organization.ID,
	})
	// If it is a 404 (not found) error, ignore it.
	if err != nil && !httpapi.Is404Error(err) {
		httpapi.InternalServerError(rw, err)
		return
	}
	if len(originalRoles) == 1 {
		// For auditing changes to a role.
		aReq.Old = originalRoles[0]
	}

	inserted, err := db.UpsertCustomRole(ctx, database.UpsertCustomRoleParams{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		OrganizationID: uuid.NullUUID{
			UUID:  organization.ID,
			Valid: true,
		},
		SitePermissions: db2sdk.List(req.SitePermissions, sdkPermissionToDB),
		OrgPermissions:  db2sdk.List(req.OrganizationPermissions, sdkPermissionToDB),
		UserPermissions: db2sdk.List(req.UserPermissions, sdkPermissionToDB),
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
	aReq.New = inserted

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.Role(inserted))
}

// deleteOrgRole will remove a custom role from an organization
// TODO: Deleting a custom role will remove permissions from any existing user
// with the role assigned. TBD if we want to also remove the roles from the
// user, or request the admin remove all the user roles first.
//
// @Summary Delete a custom organization role
// @ID delete-a-custom-organization-role
// @Security CoderSessionToken
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param roleName path string true "Role name"
// @Tags Members
// @Success 200 {array} codersdk.Role
// @Router /organizations/{organization}/members/roles{roleName} [delete]
func (api *API) deleteOrgRole(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.AGPL.Auditor.Load()
		organization      = httpmw.OrganizationParam(r)
		aReq, commitAudit = audit.InitRequest[database.CustomRole](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionDelete,
			OrganizationID: organization.ID,
		})
	)
	defer commitAudit()

	rolename := chi.URLParam(r, "roleName")
	roles, err := api.Database.CustomRoles(ctx, database.CustomRolesParams{
		LookupRoles: []database.NameOrganizationPair{
			{
				Name:           rolename,
				OrganizationID: organization.ID,
			},
		},
		ExcludeOrgRoles: false,
		OrganizationID:  organization.ID,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	if len(roles) == 0 {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message:     fmt.Sprintf("No custom role with the name %s found", rolename),
			Detail:      "no role found",
			Validations: nil,
		})
		return
	}
	if len(roles) > 1 {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message:     fmt.Sprintf("Multiple roles with the name %s found", rolename),
			Detail:      "multiple roles found, this should never happen",
			Validations: nil,
		})
		return
	}
	aReq.Old = roles[0]

	err = api.Database.DeleteCustomRole(ctx, database.DeleteCustomRoleParams{
		Name: rolename,
		OrganizationID: uuid.NullUUID{
			UUID:  organization.ID,
			Valid: true,
		},
	})
	if httpapi.IsUnauthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	aReq.New = database.CustomRole{}

	httpapi.Write(ctx, rw, http.StatusNoContent, nil)
}

func sdkPermissionToDB(p codersdk.Permission) database.CustomRolePermission {
	return database.CustomRolePermission{
		Negate:       p.Negate,
		ResourceType: string(p.ResourceType),
		Action:       policy.Action(p.Action),
	}
}
