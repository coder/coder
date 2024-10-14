package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get group IdP Sync settings by organization
// @ID get-group-idp-sync-settings-by-organization
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {object} codersdk.GroupSyncSettings
// @Router /organizations/{organization}/settings/idpsync/groups [get]
func (api *API) groupIDPSyncSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)

	if !api.Authorize(r, policy.ActionRead, rbac.ResourceIdpsyncSettings.InOrg(org.ID)) {
		httpapi.Forbidden(rw)
		return
	}

	//nolint:gocritic // Requires system context to read runtime config
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	settings, err := api.IDPSync.GroupSyncSettings(sysCtx, org.ID, api.Database)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, settings)
}

// @Summary Update group IdP Sync settings by organization
// @ID update-group-idp-sync-settings-by-organization
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {object} codersdk.GroupSyncSettings
// @Router /organizations/{organization}/settings/idpsync/groups [patch]
func (api *API) patchGroupIDPSyncSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)

	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceIdpsyncSettings.InOrg(org.ID)) {
		httpapi.Forbidden(rw)
		return
	}

	var req idpsync.GroupSyncSettings
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if len(req.LegacyNameMapping) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Unexpected field 'legacy_group_name_mapping'. Field not allowed, set to null or remove it.",
			Detail:  "legacy_group_name_mapping is deprecated, use mapping instead",
			Validations: []codersdk.ValidationError{
				{
					Field:  "legacy_group_name_mapping",
					Detail: "field is not allowed",
				},
			},
		})
		return
	}

	//nolint:gocritic // Requires system context to update runtime config
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	err := api.IDPSync.UpdateGroupSettings(sysCtx, org.ID, api.Database, req)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	settings, err := api.IDPSync.GroupSyncSettings(sysCtx, org.ID, api.Database)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, settings)
}

// @Summary Get role IdP Sync settings by organization
// @ID get-role-idp-sync-settings-by-organization
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {object} codersdk.RoleSyncSettings
// @Router /organizations/{organization}/settings/idpsync/roles [get]
func (api *API) roleIDPSyncSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)

	if !api.Authorize(r, policy.ActionRead, rbac.ResourceIdpsyncSettings.InOrg(org.ID)) {
		httpapi.Forbidden(rw)
		return
	}

	//nolint:gocritic // Requires system context to read runtime config
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	settings, err := api.IDPSync.RoleSyncSettings(sysCtx, org.ID, api.Database)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, settings)
}

// @Summary Update role IdP Sync settings by organization
// @ID update-role-idp-sync-settings-by-organization
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {object} codersdk.RoleSyncSettings
// @Router /organizations/{organization}/settings/idpsync/roles [patch]
func (api *API) patchRoleIDPSyncSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)

	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceIdpsyncSettings.InOrg(org.ID)) {
		httpapi.Forbidden(rw)
		return
	}

	var req idpsync.RoleSyncSettings
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	//nolint:gocritic // Requires system context to update runtime config
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	err := api.IDPSync.UpdateRoleSettings(sysCtx, org.ID, api.Database, req)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	settings, err := api.IDPSync.RoleSyncSettings(sysCtx, org.ID, api.Database)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, settings)
}
