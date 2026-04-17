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

// @Summary Get chat sharing settings for organization
// @ID get-chat-sharing-settings-for-organization
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {object} codersdk.ChatSharingSettings
// @Router /organizations/{organization}/settings/chat-sharing [get]
func (api *API) chatSharingSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)

	if !api.Authorize(r, policy.ActionRead, org) {
		httpapi.Forbidden(rw)
		return
	}

	globallyDisabled := bool(api.DeploymentValues.DisableChatSharing)
	owners := codersdk.ShareableChatOwners(org.ShareableChatOwners)
	if globallyDisabled {
		owners = codersdk.ShareableChatOwnersNone
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatSharingSettings{
		SharingGloballyDisabled: globallyDisabled,
		ShareableChatOwners:     owners,
	})
}

// @Summary Update chat sharing settings for organization
// @ID update-chat-sharing-settings-for-organization
// @Security CoderSessionToken
// @Produce json
// @Accept json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.UpdateChatSharingSettingsRequest true "Chat sharing settings"
// @Success 200 {object} codersdk.ChatSharingSettings
// @Router /organizations/{organization}/settings/chat-sharing [patch]
func (api *API) patchChatSharingSettings(rw http.ResponseWriter, r *http.Request) {
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

	var req codersdk.UpdateChatSharingSettingsRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	allowedOwners := req.ShareableChatOwners
	if allowedOwners == "" {
		allowedOwners = codersdk.ShareableChatOwnersEveryone
	}

	if !database.ShareableChatOwners(allowedOwners).Valid() {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid shareable chat owners value.",
			Validations: []codersdk.ValidationError{{
				Field: "shareable_chat_owners",
				Detail: fmt.Sprintf("invalid value %q, must be one of [%s]",
					allowedOwners,
					strings.Join(slice.ToStrings(database.AllShareableChatOwnersValues()), ", ")),
			}},
		})
		return
	}

	err := api.Database.InTx(func(tx database.Store) error {
		// nolint:gocritic // System context required to reconcile system roles; callers only need organization:update.
		sysCtx := dbauthz.AsSystemRestricted(ctx)

		// Serialize system-role reconciliation across coderd instances.
		err := tx.AcquireLock(ctx, database.LockIDReconcileSystemRoles)
		if err != nil {
			return xerrors.Errorf("acquire system roles reconciliation lock: %w", err)
		}

		org, err = tx.UpdateOrganizationChatSharingSettings(ctx, database.UpdateOrganizationChatSharingSettingsParams{
			ID:                  org.ID,
			ShareableChatOwners: database.ShareableChatOwners(allowedOwners),
			UpdatedAt:           dbtime.Now(),
		})
		if err != nil {
			return xerrors.Errorf("update chat sharing settings for organization %s: %w",
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

		// In service_accounts mode we retain ACLs on chats owned by service accounts.
		if org.ShareableChatOwners != database.ShareableChatOwnersEveryone {
			err = tx.DeleteChatACLsByOrganization(sysCtx, database.DeleteChatACLsByOrganizationParams{
				OrganizationID:         org.ID,
				ExcludeServiceAccounts: org.ShareableChatOwners == database.ShareableChatOwnersServiceAccounts,
			})
			if err != nil {
				return xerrors.Errorf("delete chat ACLs for organization %s: %w",
					org.ID, err)
			}
		}

		return nil
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating chat sharing settings.",
			Detail:  err.Error(),
		})
		return
	}

	aReq.New = org
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatSharingSettings{
		ShareableChatOwners: codersdk.ShareableChatOwners(org.ShareableChatOwners),
	})
}
