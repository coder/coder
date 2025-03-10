package coderd

import (
	"fmt"
	"net/http"
	"slices"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/slice"
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
	auditor := *api.AGPL.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[idpsync.GroupSyncSettings](rw, &audit.RequestParams{
		Audit:          auditor,
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: org.ID,
	})
	defer commitAudit()

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
	existing, err := api.IDPSync.GroupSyncSettings(sysCtx, org.ID, api.Database)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	aReq.Old = *existing

	err = api.IDPSync.UpdateGroupSyncSettings(sysCtx, org.ID, api.Database, idpsync.GroupSyncSettings{
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

	aReq.New = *settings
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.GroupSyncSettings{
		Field:             settings.Field,
		Mapping:           settings.Mapping,
		RegexFilter:       settings.RegexFilter,
		AutoCreateMissing: settings.AutoCreateMissing,
		LegacyNameMapping: settings.LegacyNameMapping,
	})
}

// @Summary Update group IdP Sync config
// @ID update-group-idp-sync-config
// @Security CoderSessionToken
// @Produce json
// @Accept json
// @Tags Enterprise
// @Success 200 {object} codersdk.GroupSyncSettings
// @Param organization path string true "Organization ID or name" format(uuid)
// @Param request body codersdk.PatchGroupIDPSyncConfigRequest true "New config values"
// @Router /organizations/{organization}/settings/idpsync/groups/config [patch]
func (api *API) patchGroupIDPSyncConfig(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)
	auditor := *api.AGPL.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[idpsync.GroupSyncSettings](rw, &audit.RequestParams{
		Audit:          auditor,
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: org.ID,
	})
	defer commitAudit()

	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceIdpsyncSettings.InOrg(org.ID)) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.PatchGroupIDPSyncConfigRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	var settings idpsync.GroupSyncSettings
	//nolint:gocritic // Requires system context to update runtime config
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	err := database.ReadModifyUpdate(api.Database, func(tx database.Store) error {
		existing, err := api.IDPSync.GroupSyncSettings(sysCtx, org.ID, tx)
		if err != nil {
			return err
		}
		aReq.Old = *existing

		settings = idpsync.GroupSyncSettings{
			Field:             req.Field,
			RegexFilter:       req.RegexFilter,
			AutoCreateMissing: req.AutoCreateMissing,
			LegacyNameMapping: existing.LegacyNameMapping,
			Mapping:           existing.Mapping,
		}

		err = api.IDPSync.UpdateGroupSyncSettings(sysCtx, org.ID, tx, settings)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	aReq.New = settings
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.GroupSyncSettings{
		Field:             settings.Field,
		RegexFilter:       settings.RegexFilter,
		AutoCreateMissing: settings.AutoCreateMissing,
		LegacyNameMapping: settings.LegacyNameMapping,
		Mapping:           settings.Mapping,
	})
}

// @Summary Update group IdP Sync mapping
// @ID update-group-idp-sync-mapping
// @Security CoderSessionToken
// @Produce json
// @Accept json
// @Tags Enterprise
// @Success 200 {object} codersdk.GroupSyncSettings
// @Param organization path string true "Organization ID or name" format(uuid)
// @Param request body codersdk.PatchGroupIDPSyncMappingRequest true "Description of the mappings to add and remove"
// @Router /organizations/{organization}/settings/idpsync/groups/mapping [patch]
func (api *API) patchGroupIDPSyncMapping(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)
	auditor := *api.AGPL.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[idpsync.GroupSyncSettings](rw, &audit.RequestParams{
		Audit:          auditor,
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: org.ID,
	})
	defer commitAudit()

	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceIdpsyncSettings.InOrg(org.ID)) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.PatchGroupIDPSyncMappingRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	var settings idpsync.GroupSyncSettings
	//nolint:gocritic // Requires system context to update runtime config
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	err := database.ReadModifyUpdate(api.Database, func(tx database.Store) error {
		existing, err := api.IDPSync.GroupSyncSettings(sysCtx, org.ID, tx)
		if err != nil {
			return err
		}
		aReq.Old = *existing

		newMapping := applyIDPSyncMappingDiff(existing.Mapping, req.Add, req.Remove)
		settings = idpsync.GroupSyncSettings{
			Field:             existing.Field,
			RegexFilter:       existing.RegexFilter,
			AutoCreateMissing: existing.AutoCreateMissing,
			LegacyNameMapping: existing.LegacyNameMapping,
			Mapping:           newMapping,
		}

		err = api.IDPSync.UpdateGroupSyncSettings(sysCtx, org.ID, tx, settings)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	aReq.New = settings
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.GroupSyncSettings{
		Field:             settings.Field,
		RegexFilter:       settings.RegexFilter,
		AutoCreateMissing: settings.AutoCreateMissing,
		LegacyNameMapping: settings.LegacyNameMapping,
		Mapping:           settings.Mapping,
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
	auditor := *api.AGPL.Auditor.Load()

	aReq, commitAudit := audit.InitRequest[idpsync.RoleSyncSettings](rw, &audit.RequestParams{
		Audit:          auditor,
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: org.ID,
	})
	defer commitAudit()

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
	existing, err := api.IDPSync.RoleSyncSettings(sysCtx, org.ID, api.Database)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	aReq.Old = *existing

	err = api.IDPSync.UpdateRoleSyncSettings(sysCtx, org.ID, api.Database, idpsync.RoleSyncSettings{
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

	aReq.New = *settings
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.RoleSyncSettings{
		Field:   settings.Field,
		Mapping: settings.Mapping,
	})
}

// @Summary Update role IdP Sync config
// @ID update-role-idp-sync-config
// @Security CoderSessionToken
// @Produce json
// @Accept json
// @Tags Enterprise
// @Success 200 {object} codersdk.RoleSyncSettings
// @Param organization path string true "Organization ID or name" format(uuid)
// @Param request body codersdk.PatchRoleIDPSyncConfigRequest true "New config values"
// @Router /organizations/{organization}/settings/idpsync/roles/config [patch]
func (api *API) patchRoleIDPSyncConfig(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)
	auditor := *api.AGPL.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[idpsync.RoleSyncSettings](rw, &audit.RequestParams{
		Audit:          auditor,
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: org.ID,
	})
	defer commitAudit()

	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceIdpsyncSettings.InOrg(org.ID)) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.PatchRoleIDPSyncConfigRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	var settings idpsync.RoleSyncSettings
	//nolint:gocritic // Requires system context to update runtime config
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	err := database.ReadModifyUpdate(api.Database, func(tx database.Store) error {
		existing, err := api.IDPSync.RoleSyncSettings(sysCtx, org.ID, tx)
		if err != nil {
			return err
		}
		aReq.Old = *existing

		settings = idpsync.RoleSyncSettings{
			Field:   req.Field,
			Mapping: existing.Mapping,
		}

		err = api.IDPSync.UpdateRoleSyncSettings(sysCtx, org.ID, tx, settings)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	aReq.New = settings
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.RoleSyncSettings{
		Field:   settings.Field,
		Mapping: settings.Mapping,
	})
}

// @Summary Update role IdP Sync mapping
// @ID update-role-idp-sync-mapping
// @Security CoderSessionToken
// @Produce json
// @Accept json
// @Tags Enterprise
// @Success 200 {object} codersdk.RoleSyncSettings
// @Param organization path string true "Organization ID or name" format(uuid)
// @Param request body codersdk.PatchRoleIDPSyncMappingRequest true "Description of the mappings to add and remove"
// @Router /organizations/{organization}/settings/idpsync/roles/mapping [patch]
func (api *API) patchRoleIDPSyncMapping(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)
	auditor := *api.AGPL.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[idpsync.RoleSyncSettings](rw, &audit.RequestParams{
		Audit:          auditor,
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: org.ID,
	})
	defer commitAudit()

	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceIdpsyncSettings.InOrg(org.ID)) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.PatchRoleIDPSyncMappingRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	var settings idpsync.RoleSyncSettings
	//nolint:gocritic // Requires system context to update runtime config
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	err := database.ReadModifyUpdate(api.Database, func(tx database.Store) error {
		existing, err := api.IDPSync.RoleSyncSettings(sysCtx, org.ID, tx)
		if err != nil {
			return err
		}
		aReq.Old = *existing

		newMapping := applyIDPSyncMappingDiff(existing.Mapping, req.Add, req.Remove)
		settings = idpsync.RoleSyncSettings{
			Field:   existing.Field,
			Mapping: newMapping,
		}

		err = api.IDPSync.UpdateRoleSyncSettings(sysCtx, org.ID, tx, settings)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	aReq.New = settings
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
	auditor := *api.AGPL.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[idpsync.OrganizationSyncSettings](rw, &audit.RequestParams{
		Audit:   auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionWrite,
	})
	defer commitAudit()

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
	existing, err := api.IDPSync.OrganizationSyncSettings(sysCtx, api.Database)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	aReq.Old = *existing

	err = api.IDPSync.UpdateOrganizationSyncSettings(sysCtx, api.Database, idpsync.OrganizationSyncSettings{
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

	aReq.New = *settings
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.OrganizationSyncSettings{
		Field:         settings.Field,
		Mapping:       settings.Mapping,
		AssignDefault: settings.AssignDefault,
	})
}

// @Summary Update organization IdP Sync config
// @ID update-organization-idp-sync-config
// @Security CoderSessionToken
// @Produce json
// @Accept json
// @Tags Enterprise
// @Success 200 {object} codersdk.OrganizationSyncSettings
// @Param request body codersdk.PatchOrganizationIDPSyncConfigRequest true "New config values"
// @Router /settings/idpsync/organization/config [patch]
func (api *API) patchOrganizationIDPSyncConfig(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	auditor := *api.AGPL.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[idpsync.OrganizationSyncSettings](rw, &audit.RequestParams{
		Audit:   auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionWrite,
	})
	defer commitAudit()

	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceIdpsyncSettings) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.PatchOrganizationIDPSyncConfigRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	var settings idpsync.OrganizationSyncSettings
	//nolint:gocritic // Requires system context to update runtime config
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	err := database.ReadModifyUpdate(api.Database, func(tx database.Store) error {
		existing, err := api.IDPSync.OrganizationSyncSettings(sysCtx, tx)
		if err != nil {
			return err
		}
		aReq.Old = *existing

		settings = idpsync.OrganizationSyncSettings{
			Field:         req.Field,
			AssignDefault: req.AssignDefault,
			Mapping:       existing.Mapping,
		}

		err = api.IDPSync.UpdateOrganizationSyncSettings(sysCtx, tx, settings)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	aReq.New = settings
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.OrganizationSyncSettings{
		Field:         settings.Field,
		Mapping:       settings.Mapping,
		AssignDefault: settings.AssignDefault,
	})
}

// @Summary Update organization IdP Sync mapping
// @ID update-organization-idp-sync-mapping
// @Security CoderSessionToken
// @Produce json
// @Accept json
// @Tags Enterprise
// @Success 200 {object} codersdk.OrganizationSyncSettings
// @Param request body codersdk.PatchOrganizationIDPSyncMappingRequest true "Description of the mappings to add and remove"
// @Router /settings/idpsync/organization/mapping [patch]
func (api *API) patchOrganizationIDPSyncMapping(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	auditor := *api.AGPL.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[idpsync.OrganizationSyncSettings](rw, &audit.RequestParams{
		Audit:   auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionWrite,
	})
	defer commitAudit()

	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceIdpsyncSettings) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.PatchOrganizationIDPSyncMappingRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	var settings idpsync.OrganizationSyncSettings
	//nolint:gocritic // Requires system context to update runtime config
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	err := database.ReadModifyUpdate(api.Database, func(tx database.Store) error {
		existing, err := api.IDPSync.OrganizationSyncSettings(sysCtx, tx)
		if err != nil {
			return err
		}
		aReq.Old = *existing

		newMapping := applyIDPSyncMappingDiff(existing.Mapping, req.Add, req.Remove)
		settings = idpsync.OrganizationSyncSettings{
			Field:         existing.Field,
			Mapping:       newMapping,
			AssignDefault: existing.AssignDefault,
		}

		err = api.IDPSync.UpdateOrganizationSyncSettings(sysCtx, tx, settings)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	aReq.New = settings
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

// @Summary Get the organization idp sync claim field values
// @ID get-the-organization-idp-sync-claim-field-values
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Param claimField query string true "Claim Field" format(string)
// @Success 200 {array} string
// @Router /organizations/{organization}/settings/idpsync/field-values [get]
func (api *API) organizationIDPSyncClaimFieldValues(rw http.ResponseWriter, r *http.Request) {
	org := httpmw.OrganizationParam(r)
	api.idpSyncClaimFieldValues(org.ID, rw, r)
}

// @Summary Get the idp sync claim field values
// @ID get-the-idp-sync-claim-field-values
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Param claimField query string true "Claim Field" format(string)
// @Success 200 {array} string
// @Router /settings/idpsync/field-values [get]
func (api *API) deploymentIDPSyncClaimFieldValues(rw http.ResponseWriter, r *http.Request) {
	// nil uuid implies all organizations
	api.idpSyncClaimFieldValues(uuid.Nil, rw, r)
}

func (api *API) idpSyncClaimFieldValues(orgID uuid.UUID, rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	claimField := r.URL.Query().Get("claimField")
	if claimField == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "claimField query parameter is required",
		})
		return
	}
	fieldValues, err := api.Database.OIDCClaimFieldValues(ctx, database.OIDCClaimFieldValuesParams{
		OrganizationID: orgID,
		ClaimField:     claimField,
	})

	if httpapi.IsUnauthorizedError(err) {
		// Give a helpful error. The user could read the org, so this does not
		// leak anything.
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "You do not have permission to view the IDP claim field values",
			Detail:  fmt.Sprintf("%s.read permission is required", rbac.ResourceIdpsyncSettings.Type),
		})
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, fieldValues)
}

func applyIDPSyncMappingDiff[IDType uuid.UUID | string](
	previous map[string][]IDType,
	add, remove []codersdk.IDPSyncMapping[IDType],
) map[string][]IDType {
	next := make(map[string][]IDType)

	// Copy existing mapping
	for key, ids := range previous {
		next[key] = append(next[key], ids...)
	}

	// Add unique entries
	for _, mapping := range add {
		if !slice.Contains(next[mapping.Given], mapping.Gets) {
			next[mapping.Given] = append(next[mapping.Given], mapping.Gets)
		}
	}

	// Remove entries
	for _, mapping := range remove {
		next[mapping.Given] = slices.DeleteFunc(next[mapping.Given], func(u IDType) bool {
			return u == mapping.Gets
		})
	}

	return next
}
