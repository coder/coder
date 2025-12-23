package coderd

import (
	"net/http"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get workspace sharing settings for organization
// @ID get-workspace-sharing-settings
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {object} codersdk.WorkspaceSharingSettings
// @Router /organizations/{organization}/settings/workspace-sharing [get]
func (api *API) workspaceSharingSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)

	// TODO(geokat): Do we need an rbac.ResourceWorkspaceSharingSettings?
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceOrganization.InOrg(org.ID)) {
		httpapi.Forbidden(rw)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.WorkspaceSharingSettings{
		SharingDisabled: org.WorkspaceSharingDisabled,
	})
}

// @Summary Update workspace sharing settings for organization
// @ID update-workspace-sharing-settings
// @Security CoderSessionToken
// @Produce json
// @Accept json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.WorkspaceSharingSettings true "Workspace sharing settings"
// @Success 200 {object} codersdk.WorkspaceSharingSettings
// @Router /organizations/{organization}/settings/workspace-sharing [patch]
func (api *API) patchWorkspaceSharingSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)
	auditor := *api.AGPL.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.Organization](rw, &audit.RequestParams{
		Audit:          auditor,
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: org.ID,
	})
	aReq.Old = org
	defer commitAudit()

	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceOrganization.InOrg(org.ID)) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.WorkspaceSharingSettings
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	err := api.Database.InTx(func(tx database.Store) error {
		//nolint:gocritic // We need to manage a system role.
		sysCtx := dbauthz.AsSystemRestricted(ctx)

		// Serialize organization workspace-sharing updates with system role
		// reconciliation across coderd instances (e.g. during rolling restarts).
		// This prevents conflicting writes to the organization-member system role.
		// TODO(geokat): Consider finer-grained locks as we add more system roles.
		err := tx.AcquireLock(sysCtx, database.LockIDReconcileSystemRoles)
		if err != nil {
			return xerrors.Errorf("acquire system roles reconciliation lock: %w", err)
		}

		org, err = tx.UpdateOrganizationWorkspaceSharingSettings(ctx, database.UpdateOrganizationWorkspaceSharingSettingsParams{
			ID:                       org.ID,
			WorkspaceSharingDisabled: req.SharingDisabled,
			UpdatedAt:                dbtime.Now(),
		})
		if err != nil {
			return xerrors.Errorf("update organization workspace sharing settings: %w", err)
		}

		role, err := database.ExpectOne(tx.CustomRoles(sysCtx, database.CustomRolesParams{
			LookupRoles: []database.NameOrganizationPair{
				{
					Name:           rbac.RoleOrgMember(),
					OrganizationID: org.ID,
				},
			},
			// Satisfy linter that requires all fields to be set.
			OrganizationID:     org.ID,
			ExcludeOrgRoles:    false,
			IncludeSystemRoles: true,
		}))
		if err != nil {
			return xerrors.Errorf("get organization-member role: %w", err)
		}

		_, err = rolestore.ReconcileOrgMemberRole(sysCtx, tx, role, req.SharingDisabled)
		if err != nil {
			return xerrors.Errorf("reconcile organization-member role: %w", err)
		}

		if req.SharingDisabled {
			err = tx.DeleteWorkspaceACLsByOrganization(sysCtx, org.ID)
			if err != nil {
				return xerrors.Errorf("delete workspace ACLs by organization: %w", err)
			}
		}

		return nil
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating workspace sharing settings.",
			Detail:  err.Error(),
		})
		return
	}

	aReq.New = org
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.WorkspaceSharingSettings{
		SharingDisabled: org.WorkspaceSharingDisabled,
	})
}
