package coderd

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"

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
// @Accept json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.GroupSyncSettings true "New settings"
// @Success 200 {object} codersdk.GroupSyncSettings
// @Router /organizations/{organization}/settings/idpsync/groups [patch]
func (api *API) patchGroupIDPSyncSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)

	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceIdpsyncSettings.InOrg(org.ID)) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.GroupSyncSettings
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
	err := api.IDPSync.UpdateGroupSettings(sysCtx, org.ID, api.Database, idpsync.GroupSyncSettings{
		Field:             req.Field,
		Mapping:           req.Mapping,
		RegexFilter:       req.RegexFilter,
		AutoCreateMissing: req.AutoCreateMissing,
		LegacyNameMapping: req.LegacyNameMapping,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	settings, err := api.IDPSync.GroupSyncSettings(sysCtx, org.ID, api.Database)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.GroupSyncSettings{
		Field:             settings.Field,
		Mapping:           settings.Mapping,
		RegexFilter:       settings.RegexFilter,
		AutoCreateMissing: settings.AutoCreateMissing,
		LegacyNameMapping: settings.LegacyNameMapping,
	})
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
// @Accept json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.RoleSyncSettings true "New settings"
// @Success 200 {object} codersdk.RoleSyncSettings
// @Router /organizations/{organization}/settings/idpsync/roles [patch]
func (api *API) patchRoleIDPSyncSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)

	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceIdpsyncSettings.InOrg(org.ID)) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.RoleSyncSettings
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	//nolint:gocritic // Requires system context to update runtime config
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	err := api.IDPSync.UpdateRoleSettings(sysCtx, org.ID, api.Database, idpsync.RoleSyncSettings{
		Field:   req.Field,
		Mapping: req.Mapping,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	settings, err := api.IDPSync.RoleSyncSettings(sysCtx, org.ID, api.Database)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.RoleSyncSettings{
		Field:   settings.Field,
		Mapping: settings.Mapping,
	})
}

// @Summary Get organization IdP Sync settings
// @ID get-organization-idp-sync-settings
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Success 200 {object} codersdk.OrganizationSyncSettings
// @Router /settings/idpsync/organization [get]
func (api *API) organizationIDPSyncSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !api.Authorize(r, policy.ActionRead, rbac.ResourceIdpsyncSettings) {
		httpapi.Forbidden(rw)
		return
	}

	//nolint:gocritic // Requires system context to read runtime config
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	settings, err := api.IDPSync.OrganizationSyncSettings(sysCtx, api.Database)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.OrganizationSyncSettings{
		Field:         settings.Field,
		Mapping:       settings.Mapping,
		AssignDefault: settings.AssignDefault,
	})
}

// @Summary Update organization IdP Sync settings
// @ID update-organization-idp-sync-settings
// @Security CoderSessionToken
// @Produce json
// @Accept json
// @Tags Enterprise
// @Success 200 {object} codersdk.OrganizationSyncSettings
// @Param request body codersdk.OrganizationSyncSettings true "New settings"
// @Router /settings/idpsync/organization [patch]
func (api *API) patchOrganizationIDPSyncSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceIdpsyncSettings) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.OrganizationSyncSettings
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	//nolint:gocritic // Requires system context to update runtime config
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	err := api.IDPSync.UpdateOrganizationSettings(sysCtx, api.Database, idpsync.OrganizationSyncSettings{
		Field: req.Field,
		// We do not check if the mappings point to actual organizations.
		Mapping:       req.Mapping,
		AssignDefault: req.AssignDefault,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	settings, err := api.IDPSync.OrganizationSyncSettings(sysCtx, api.Database)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.OrganizationSyncSettings{
		Field:         settings.Field,
		Mapping:       settings.Mapping,
		AssignDefault: settings.AssignDefault,
	})
}

// @Summary Get the available organization idp sync claim fields
// @ID get-the-available-organization-idp-sync-claim-fields
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {array} string
// @Router /organizations/{organization}/settings/idpsync/available-fields [get]
func (api *API) organizationIDPSyncClaimFields(rw http.ResponseWriter, r *http.Request) {
	org := httpmw.OrganizationParam(r)
	api.idpSyncClaimFields(org.ID, rw, r)
}

// @Summary Get the available idp sync claim fields
// @ID get-the-available-idp-sync-claim-fields
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {array} string
// @Router /settings/idpsync/available-fields [get]
func (api *API) deploymentIDPSyncClaimFields(rw http.ResponseWriter, r *http.Request) {
	// nil uuid implies all organizations
	api.idpSyncClaimFields(uuid.Nil, rw, r)
}

func (api *API) idpSyncClaimFields(orgID uuid.UUID, rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	fields, err := api.Database.OIDCClaimFields(ctx, orgID)
	if httpapi.IsUnauthorizedError(err) {
		// Give a helpful error. The user could read the org, so this does not
		// leak anything.
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "You do not have permission to view the available IDP fields",
			Detail:  fmt.Sprintf("%s.read permission is required", rbac.ResourceIdpsyncSettings.Type),
		})
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, fields)
}
