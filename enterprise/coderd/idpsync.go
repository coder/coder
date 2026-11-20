package coderd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

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
// @Success 200 {object} codersdk.GroupSyncSettingsResponse
// @Router /api/v2/organizations/{organization}/settings/idpsync/groups [get]
func (api *API) groupIDPSyncSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)

	if !api.Authorize(r, policy.ActionRead, rbac.ResourceIdpsyncSettings.InOrg(org.ID)) {
		httpapi.Forbidden(rw)
		return
	}

	//nolint:gocritic // Requires system context to read and repair runtime config
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	var fetched groupSyncFetch
	err := database.ReadModifyUpdate(api.Database, func(tx database.Store) error {
		var err error
		fetched, err = api.fetchGroupSyncSettings(sysCtx, tx, org.ID)
		return err
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	api.auditGroupSyncRepair(ctx, r, org.ID, fetched)

	httpapi.Write(ctx, rw, http.StatusOK, fetched.review.response())
}

// @Summary Update group IdP Sync settings by organization
// @ID update-group-idp-sync-settings-by-organization
// @Security CoderSessionToken
// @Produce json
// @Accept json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.GroupSyncSettings true "New settings"
// @Success 200 {object} codersdk.GroupSyncSettingsResponse
// @Router /api/v2/organizations/{organization}/settings/idpsync/groups [patch]
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
	var review groupSyncReview
	err := database.ReadModifyUpdate(api.Database, func(tx database.Store) error {
		existing, err := api.IDPSync.GroupSyncSettings(sysCtx, org.ID, tx)
		if err != nil {
			return err
		}
		aReq.Old = *existing

		review, err = reviewGroupSyncSettings(sysCtx, tx, org.ID, req, idpsync.GroupSyncRouteFullReplace)
		if err != nil {
			return err
		}
		if review.rejected() {
			return errGroupSyncRejected
		}
		return api.IDPSync.UpdateGroupSyncSettings(sysCtx, org.ID, tx, idpsync.GroupSyncSettings(review.result.Settings))
	})
	if errors.Is(err, errGroupSyncRejected) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, review.rejection())
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	aReq.New = idpsync.GroupSyncSettings(review.result.Settings)
	httpapi.Write(ctx, rw, http.StatusOK, review.response())
}

// @Summary Update group IdP Sync config
// @ID update-group-idp-sync-config
// @Security CoderSessionToken
// @Produce json
// @Accept json
// @Tags Enterprise
// @Success 200 {object} codersdk.GroupSyncSettingsResponse
// @Param organization path string true "Organization ID or name" format(uuid)
// @Param request body codersdk.PatchGroupIDPSyncConfigRequest true "New config values"
// @Router /api/v2/organizations/{organization}/settings/idpsync/groups/config [patch]
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

	api.patchGroupSyncSettingsByMerge(rw, r, org.ID, aReq, func(existing codersdk.GroupSyncSettings) codersdk.GroupSyncSettings {
		return codersdk.GroupSyncSettings{
			Field:             req.Field,
			RegexFilter:       req.RegexFilter,
			AutoCreateMissing: req.AutoCreateMissing,
			LegacyNameMapping: existing.LegacyNameMapping,
			Mapping:           existing.Mapping,
		}
	})
}

// @Summary Update group IdP Sync mapping
// @ID update-group-idp-sync-mapping
// @Security CoderSessionToken
// @Produce json
// @Accept json
// @Tags Enterprise
// @Success 200 {object} codersdk.GroupSyncSettingsResponse
// @Param organization path string true "Organization ID or name" format(uuid)
// @Param request body codersdk.PatchGroupIDPSyncMappingRequest true "Description of the mappings to add and remove"
// @Router /api/v2/organizations/{organization}/settings/idpsync/groups/mapping [patch]
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

	api.patchGroupSyncSettingsByMerge(rw, r, org.ID, aReq, func(existing codersdk.GroupSyncSettings) codersdk.GroupSyncSettings {
		return codersdk.GroupSyncSettings{
			Field:             existing.Field,
			RegexFilter:       existing.RegexFilter,
			AutoCreateMissing: existing.AutoCreateMissing,
			LegacyNameMapping: existing.LegacyNameMapping,
			Mapping:           applyIDPSyncMappingDiff(existing.Mapping, req.Add, req.Remove),
		}
	})
}

// patchGroupSyncSettingsByMerge carries out a PATCH that merges a request
// into the stored settings. The stored settings are read through the fetch
// route, so a stale entry is repaired instead of blocking the request; merge
// builds the merged settings from them; and the merged settings are checked
// on the merge route, then saved or rejected. The response's warnings refer
// to the merged settings only. A repair of the stored settings is audited as
// Coder's action, not reported.
func (api *API) patchGroupSyncSettingsByMerge(
	rw http.ResponseWriter,
	r *http.Request,
	orgID uuid.UUID,
	aReq *audit.Request[idpsync.GroupSyncSettings],
	merge func(existing codersdk.GroupSyncSettings) codersdk.GroupSyncSettings,
) {
	ctx := r.Context()
	//nolint:gocritic // Requires system context to update runtime config
	sysCtx := dbauthz.AsSystemRestricted(ctx)

	var (
		fetched groupSyncFetch
		review  groupSyncReview
	)
	err := database.ReadModifyUpdate(api.Database, func(tx database.Store) error {
		var err error
		fetched, err = api.fetchGroupSyncSettings(sysCtx, tx, orgID)
		if err != nil {
			return err
		}
		// The request's audit entry starts from the repaired settings, so it
		// shows only the request's change; the repair is audited on its own.
		existing := fetched.review.result.Settings
		aReq.Old = idpsync.GroupSyncSettings(existing)

		review, err = reviewGroupSyncSettings(sysCtx, tx, orgID, merge(existing), idpsync.GroupSyncRouteMerge)
		if err != nil {
			return err
		}
		if review.rejected() {
			return errGroupSyncRejected
		}
		return api.IDPSync.UpdateGroupSyncSettings(sysCtx, orgID, tx, idpsync.GroupSyncSettings(review.result.Settings))
	})
	if errors.Is(err, errGroupSyncRejected) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, review.rejection())
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	api.auditGroupSyncRepair(ctx, r, orgID, fetched)

	aReq.New = idpsync.GroupSyncSettings(review.result.Settings)
	httpapi.Write(ctx, rw, http.StatusOK, review.response())
}

// errGroupSyncRejected aborts the transaction of a request whose settings the
// rewrite policy rejects. The handler then writes the rejection.
var errGroupSyncRejected = xerrors.New("group sync settings rejected")

// groupSyncReview is the result of checking group sync settings on one route:
// the settings as received, the plan the rewrite policy gave for them, and,
// unless the plan rejects them, the result of carrying out the plan.
type groupSyncReview struct {
	received codersdk.GroupSyncSettings
	plan     idpsync.GroupSyncRewritePlan
	result   idpsync.GroupSyncRewriteResult
}

// reviewGroupSyncSettings checks settings for the organization orgID as they
// arrive by route: it reads a snapshot of the deployment from tx, finds every
// violation, plans what to do under the relaxed policy, and carries out the
// plan unless it rejects the settings. On the fetch route a stored row is
// never rejected; if the plan says reject, the row is kept as stored.
func reviewGroupSyncSettings(ctx context.Context, tx database.Store, orgID uuid.UUID, settings codersdk.GroupSyncSettings, route idpsync.GroupSyncRoute) (groupSyncReview, error) {
	snapshot, err := idpsync.DatabaseGroupSnapshotReader(tx)(ctx, orgID, settings.CanonicalMappingUUIDs())
	if err != nil {
		return groupSyncReview{}, xerrors.Errorf("read group snapshot: %w", err)
	}
	findings, err := idpsync.ValidityFindings(settings, snapshot)
	if err != nil {
		return groupSyncReview{}, xerrors.Errorf("validate group sync settings: %w", err)
	}

	review := groupSyncReview{
		received: settings,
		plan:     idpsync.RewritePlan(idpsync.RelaxedGroupSyncAction, route, findings),
	}
	if review.plan.Instruction == idpsync.GroupSyncInstructionReject {
		if route != idpsync.GroupSyncRouteFetch {
			return review, nil
		}
		review.plan.Instruction = idpsync.GroupSyncInstructionKeep
	}
	review.result, err = idpsync.RewriteResult(settings, review.plan)
	if err != nil {
		return groupSyncReview{}, xerrors.Errorf("rewrite group sync settings: %w", err)
	}
	return review, nil
}

// rejected reports whether the plan rejects the settings.
func (r groupSyncReview) rejected() bool {
	return r.plan.Instruction == idpsync.GroupSyncInstructionReject
}

// response builds the response body for settings the plan did not reject.
// It carries the received settings exactly when there are warnings, since
// the warnings' paths point into them.
func (r groupSyncReview) response() codersdk.GroupSyncSettingsResponse {
	resp := codersdk.GroupSyncSettingsResponse{
		GroupSyncSettings: withNonNilMapping(r.result.Settings),
		Warnings:          r.result.Warnings,
	}
	if resp.Warnings == nil {
		resp.Warnings = []codersdk.GroupSyncWarning{}
	}
	if len(resp.Warnings) > 0 {
		received := withNonNilMapping(r.received)
		resp.ReceivedSettings = &received
	}
	return resp
}

// rejection builds the 400 response for settings the plan rejected: one
// validation for each rejected finding, naming its path and its code.
func (r groupSyncReview) rejection() codersdk.Response {
	resp := codersdk.Response{
		Message: "Group sync settings are not valid.",
		Detail:  "Each validation names the element by its JSON Pointer and the condition it violates.",
	}
	for _, planned := range r.plan.Actions {
		if planned.Action != idpsync.GroupSyncActionReject {
			continue
		}
		resp.Validations = append(resp.Validations, codersdk.ValidationError{
			Field:  planned.Finding.Path,
			Detail: "rejected: " + string(planned.Finding.Code),
		})
	}
	return resp
}

// withNonNilMapping returns settings with an empty mapping in place of a nil
// one. Both are empty forms; the endpoints have always sent `{}`, never
// `null`, and this keeps that.
func withNonNilMapping(settings codersdk.GroupSyncSettings) codersdk.GroupSyncSettings {
	if settings.Mapping == nil {
		settings.Mapping = map[string][]uuid.UUID{}
	}
	return settings
}

// groupSyncFetch is the stored group sync settings of an organization, read
// through the fetch route.
type groupSyncFetch struct {
	stored codersdk.GroupSyncSettings
	review groupSyncReview
}

// repaired reports whether the stored settings were rewritten.
func (f groupSyncFetch) repaired() bool {
	return f.review.result.Outcome == idpsync.GroupSyncRewriteOutcomeRewritten
}

// fetchGroupSyncSettings reads the organization's stored group sync settings
// through the fetch route, and replaces the stored row with its rewrite when
// the settings were repaired. tx must be the transaction of the request, so
// that the read, the snapshot, and the write see the same state.
func (api *API) fetchGroupSyncSettings(ctx context.Context, tx database.Store, orgID uuid.UUID) (groupSyncFetch, error) {
	existing, err := api.IDPSync.GroupSyncSettings(ctx, orgID, tx)
	if err != nil {
		return groupSyncFetch{}, err
	}
	fetched := groupSyncFetch{stored: codersdk.GroupSyncSettings(*existing)}
	fetched.review, err = reviewGroupSyncSettings(ctx, tx, orgID, fetched.stored, idpsync.GroupSyncRouteFetch)
	if err != nil {
		return groupSyncFetch{}, err
	}
	if fetched.repaired() {
		err = api.IDPSync.UpdateGroupSyncSettings(ctx, orgID, tx, idpsync.GroupSyncSettings(fetched.review.result.Settings))
		if err != nil {
			return groupSyncFetch{}, xerrors.Errorf("save repaired group sync settings: %w", err)
		}
	}
	return fetched, nil
}

// auditGroupSyncRepair records the repair of stored group sync settings as
// Coder's own action, naming the user whose request caused it. It records
// nothing when the settings were not repaired. It runs after the transaction
// commits, so a retried transaction records the repair once.
func (api *API) auditGroupSyncRepair(ctx context.Context, r *http.Request, orgID uuid.UUID, fetched groupSyncFetch) {
	if !fetched.repaired() {
		return
	}
	var userID uuid.UUID
	if key, ok := httpmw.APIKeyOptional(r); ok {
		userID = key.UserID
	}
	audit.BackgroundAudit(context.WithoutCancel(ctx), &audit.BackgroundAuditParams[idpsync.GroupSyncSettings]{
		Audit:            *api.AGPL.Auditor.Load(),
		Log:              api.Logger,
		UserID:           userID,
		RequestID:        httpmw.RequestID(r),
		Status:           http.StatusOK,
		Action:           database.AuditActionWrite,
		OrganizationID:   orgID,
		IP:               r.RemoteAddr,
		UserAgent:        r.UserAgent(),
		AdditionalFields: audit.BackgroundTaskFieldsBytes(ctx, api.Logger, audit.BackgroundSubsystemGroupSyncRewrite),
		Old:              idpsync.GroupSyncSettings(fetched.stored),
		New:              idpsync.GroupSyncSettings(fetched.review.result.Settings),
	})
}

// @Summary Get role IdP Sync settings by organization
// @ID get-role-idp-sync-settings-by-organization
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {object} codersdk.RoleSyncSettings
// @Router /api/v2/organizations/{organization}/settings/idpsync/roles [get]
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
// @Router /api/v2/organizations/{organization}/settings/idpsync/roles [patch]
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
// @Router /api/v2/organizations/{organization}/settings/idpsync/roles/config [patch]
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
// @Router /api/v2/organizations/{organization}/settings/idpsync/roles/mapping [patch]
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
// @Router /api/v2/settings/idpsync/organization [get]
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
// @Router /api/v2/settings/idpsync/organization [patch]
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
// @Router /api/v2/settings/idpsync/organization/config [patch]
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
// @Router /api/v2/settings/idpsync/organization/mapping [patch]
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
// @Router /api/v2/organizations/{organization}/settings/idpsync/available-fields [get]
func (api *API) organizationIDPSyncClaimFields(rw http.ResponseWriter, r *http.Request) {
	org := httpmw.OrganizationParam(r)
	api.idpSyncClaimFields(org.ID, rw, r)
}

// @Summary Get the available idp sync claim fields
// @ID get-the-available-idp-sync-claim-fields
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Success 200 {array} string
// @Router /api/v2/settings/idpsync/available-fields [get]
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
// @Router /api/v2/organizations/{organization}/settings/idpsync/field-values [get]
func (api *API) organizationIDPSyncClaimFieldValues(rw http.ResponseWriter, r *http.Request) {
	org := httpmw.OrganizationParam(r)
	api.idpSyncClaimFieldValues(org.ID, rw, r)
}

// @Summary Get the idp sync claim field values
// @ID get-the-idp-sync-claim-field-values
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param claimField query string true "Claim Field" format(string)
// @Success 200 {array} string
// @Router /api/v2/settings/idpsync/field-values [get]
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
	if fieldValues == nil {
		fieldValues = []string{}
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
