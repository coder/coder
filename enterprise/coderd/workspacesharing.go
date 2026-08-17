package coderd

import (
	"fmt"
	"net/http"
	"strings"

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
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get workspace sharing settings for organization
// @ID get-workspace-sharing-settings-for-organization
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {object} codersdk.WorkspaceSharingSettings
// @Router /api/v2/organizations/{organization}/settings/workspace-sharing [get]
func (api *API) workspaceSharingSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)

	if !api.Authorize(r, policy.ActionRead, org) {
		httpapi.Forbidden(rw)
		return
	}

	disabled := org.ShareableWorkspaceOwners == database.ShareableWorkspaceOwnersNone
	globallyDisabled := bool(api.DeploymentValues.DisableWorkspaceSharing)
	owners := codersdk.ShareableWorkspaceOwners(org.ShareableWorkspaceOwners)
	if globallyDisabled {
		owners = codersdk.ShareableWorkspaceOwnersNone
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.WorkspaceSharingSettings{
		SharingGloballyDisabled:  globallyDisabled,
		SharingDisabled:          disabled || globallyDisabled,
		ShareableWorkspaceOwners: owners,
	})
}

// @Summary Update workspace sharing settings for organization
// @ID update-workspace-sharing-settings-for-organization
// @Security CoderSessionToken
// @Produce json
// @Accept json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.UpdateWorkspaceSharingSettingsRequest true "Workspace sharing settings"
// @Success 200 {object} codersdk.WorkspaceSharingSettings
// @Router /api/v2/organizations/{organization}/settings/workspace-sharing [patch]
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

	if !api.Authorize(r, policy.ActionUpdate, org) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.UpdateWorkspaceSharingSettingsRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// Resolve the effective enum value. Prefer the new field; fall
	// back to the deprecated boolean for older clients (e.g
	// tf-provider-coderd v0.0.16)
	allowedOwners := req.ShareableWorkspaceOwners
	if allowedOwners == "" {
		if req.SharingDisabled {
			allowedOwners = codersdk.ShareableWorkspaceOwnersNone
		} else {
			allowedOwners = codersdk.ShareableWorkspaceOwnersEveryone
		}
	}

	if !database.ShareableWorkspaceOwners(allowedOwners).Valid() {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid shareable workspace owners value.",
			Validations: []codersdk.ValidationError{{
				Field: "shareable_workspace_owners",
				Detail: fmt.Sprintf("invalid value %q, must be one of [%s]",
					allowedOwners,
					strings.Join(slice.ToStrings(database.AllShareableWorkspaceOwnersValues()), ", ")),
			}},
		})
		return
	}

	err := api.Database.InTx(func(tx database.Store) error {
		//nolint:gocritic // System context required to look up and reconcile the
		// system roles; callers only need `organization:update`
		sysCtx := dbauthz.AsSystemRestricted(ctx)

		// Serialize organization workspace-sharing updates with system role
		// reconciliation across coderd instances (e.g. during rolling restarts).
		// This prevents conflicting writes to the system roles.
		// TODO(geokat): Consider finer-grained locks as we add more system roles.
		err := tx.AcquireLock(ctx, database.LockIDReconcileSystemRoles)
		if err != nil {
			return xerrors.Errorf("acquire system roles reconciliation lock: %w", err)
		}

		org, err = tx.UpdateOrganizationWorkspaceSharingSettings(ctx, database.UpdateOrganizationWorkspaceSharingSettingsParams{
			ID:                       org.ID,
			ShareableWorkspaceOwners: database.ShareableWorkspaceOwners(allowedOwners),
			UpdatedAt:                dbtime.Now(),
		})
		if err != nil {
			return xerrors.Errorf("update workspace sharing settings for organization %s: %w",
				org.ID, err)
		}

		roles, err := tx.CustomRoles(sysCtx, database.CustomRolesParams{
			LookupRoles: []database.NameOrganizationPair{
				{
					Name:           rbac.RoleOrgMember(),
					OrganizationID: org.ID,
				},
				{
					Name:           rbac.RoleOrgServiceAccount(),
					OrganizationID: org.ID,
				},
			},
			// Satisfy linter that requires all fields to be set.
			OrganizationID:     org.ID,
			ExcludeOrgRoles:    false,
			IncludeSystemRoles: true,
		})
		if err != nil || len(roles) != 2 {
			return xerrors.Errorf("get member and service-account roles for organization %s: %w",
				org.ID, err)
		}

		for _, role := range roles {
			_, _, err = rolestore.ReconcileSystemRole(sysCtx, tx, role, org)
			if err != nil {
				return xerrors.Errorf("reconcile %s role for organization %s: %w",
					role.Name, org.ID, err)
			}
		}

		// If sharing is not enabled, delete workspace ACLs to prevent
		// ongoing shared use. In "service_accounts" mode, preserve
		// ACLs on SA workspaces.
		if org.ShareableWorkspaceOwners != database.ShareableWorkspaceOwnersEveryone {
			err = tx.DeleteWorkspaceACLsByOrganization(sysCtx, database.DeleteWorkspaceACLsByOrganizationParams{
				OrganizationID:         org.ID,
				ExcludeServiceAccounts: org.ShareableWorkspaceOwners == database.ShareableWorkspaceOwnersServiceAccounts,
			})
			if err != nil {
				return xerrors.Errorf("delete workspace ACLs for organization %s: %w",
					org.ID, err)
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
		SharingDisabled:          org.ShareableWorkspaceOwners == database.ShareableWorkspaceOwnersNone,
		ShareableWorkspaceOwners: codersdk.ShareableWorkspaceOwners(org.ShareableWorkspaceOwners),
	})
}
