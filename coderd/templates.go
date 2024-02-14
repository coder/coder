package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/examples"
)

// Returns a single template.
//
// @Summary Get template metadata by ID
// @ID get-template-metadata-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param template path string true "Template ID" format(uuid)
// @Success 200 {object} codersdk.Template
// @Router /templates/{template} [get]
func (api *API) template(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	template := httpmw.TemplateParam(r)

	httpapi.Write(ctx, rw, http.StatusOK, api.convertTemplate(template))
}

// @Summary Delete template by ID
// @ID delete-template-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param template path string true "Template ID" format(uuid)
// @Success 200 {object} codersdk.Response
// @Router /templates/{template} [delete]
func (api *API) deleteTemplate(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		template          = httpmw.TemplateParam(r)
		auditor           = *api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.Template](rw, &audit.RequestParams{
			Audit:   auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionDelete,
		})
	)
	defer commitAudit()
	aReq.Old = template

	// This is just to get the workspace count, so we use a system context to
	// return ALL workspaces. Not just workspaces the user can view.
	// nolint:gocritic
	workspaces, err := api.Database.GetWorkspaces(dbauthz.AsSystemRestricted(ctx), database.GetWorkspacesParams{
		TemplateIDs: []uuid.UUID{template.ID},
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspaces by template id.",
			Detail:  err.Error(),
		})
		return
	}
	if len(workspaces) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "All workspaces must be deleted before a template can be removed.",
		})
		return
	}
	err = api.Database.UpdateTemplateDeletedByID(ctx, database.UpdateTemplateDeletedByIDParams{
		ID:        template.ID,
		Deleted:   true,
		UpdatedAt: dbtime.Now(),
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting template.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "Template has been deleted!",
	})
}

// Create a new template in an organization.
// Returns a single template.
//
// @Summary Create template by organization
// @ID create-template-by-organization
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Templates
// @Param request body codersdk.CreateTemplateRequest true "Request body"
// @Param organization path string true "Organization ID"
// @Success 200 {object} codersdk.Template
// @Router /organizations/{organization}/templates [post]
func (api *API) postTemplateByOrganization(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx                                = r.Context()
		createTemplate                     codersdk.CreateTemplateRequest
		organization                       = httpmw.OrganizationParam(r)
		apiKey                             = httpmw.APIKey(r)
		auditor                            = *api.Auditor.Load()
		templateAudit, commitTemplateAudit = audit.InitRequest[database.Template](rw, &audit.RequestParams{
			Audit:   auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
		templateVersionAudit, commitTemplateVersionAudit = audit.InitRequest[database.TemplateVersion](rw, &audit.RequestParams{
			Audit:   auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitTemplateAudit()
	defer commitTemplateVersionAudit()

	if !httpapi.Read(ctx, rw, r, &createTemplate) {
		return
	}

	// Make a temporary struct to represent the template. This is used for
	// auditing if any of the following checks fail. It will be overwritten when
	// the template is inserted into the db.
	templateAudit.New = database.Template{
		OrganizationID: organization.ID,
		Name:           createTemplate.Name,
		Description:    createTemplate.Description,
		CreatedBy:      apiKey.UserID,
		Icon:           createTemplate.Icon,
		DisplayName:    createTemplate.DisplayName,
	}

	_, err := api.Database.GetTemplateByOrganizationAndName(ctx, database.GetTemplateByOrganizationAndNameParams{
		OrganizationID: organization.ID,
		Name:           createTemplate.Name,
	})
	if err == nil {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("Template with name %q already exists.", createTemplate.Name),
			Validations: []codersdk.ValidationError{{
				Field:  "name",
				Detail: "This value is already in use and should be unique.",
			}},
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template by name.",
			Detail:  err.Error(),
		})
		return
	}

	templateVersion, err := api.Database.GetTemplateVersionByID(ctx, createTemplate.VersionID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("Template version %q does not exist.", createTemplate.VersionID),
			Validations: []codersdk.ValidationError{
				{Field: "template_version_id", Detail: "Template version does not exist"},
			},
		})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template version.",
			Detail:  err.Error(),
		})
		return
	}
	if templateVersion.Archived {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Template version %s is archived.", createTemplate.VersionID),
			Validations: []codersdk.ValidationError{
				{Field: "template_version_id", Detail: "Template version is archived"},
			},
		})
		return
	}
	templateVersionAudit.Old = templateVersion
	if templateVersion.TemplateID.Valid {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Template version %s is already part of a template", createTemplate.VersionID),
			Validations: []codersdk.ValidationError{
				{Field: "template_version_id", Detail: "Template version is already part of a template"},
			},
		})
		return
	}

	importJob, err := api.Database.GetProvisionerJobByID(ctx, templateVersion.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}

	var (
		defaultTTL                     time.Duration
		activityBump                   = time.Hour // default
		maxTTL                         time.Duration
		autostopRequirementDaysOfWeek  []string
		autostartRequirementDaysOfWeek []string
		autostopRequirementWeeks       int64
		failureTTL                     time.Duration
		dormantTTL                     time.Duration
		dormantAutoDeletionTTL         time.Duration
	)
	if createTemplate.DefaultTTLMillis != nil {
		defaultTTL = time.Duration(*createTemplate.DefaultTTLMillis) * time.Millisecond
	}
	if createTemplate.ActivityBumpMillis != nil {
		activityBump = time.Duration(*createTemplate.ActivityBumpMillis) * time.Millisecond
	}
	if createTemplate.AutostopRequirement != nil {
		autostopRequirementDaysOfWeek = createTemplate.AutostopRequirement.DaysOfWeek
		autostopRequirementWeeks = createTemplate.AutostopRequirement.Weeks
	}
	if createTemplate.AutostartRequirement != nil {
		autostartRequirementDaysOfWeek = createTemplate.AutostartRequirement.DaysOfWeek
	} else {
		// By default, we want to allow all days of the week to be autostarted.
		autostartRequirementDaysOfWeek = codersdk.BitmapToWeekdays(0b01111111)
	}
	if createTemplate.FailureTTLMillis != nil {
		failureTTL = time.Duration(*createTemplate.FailureTTLMillis) * time.Millisecond
	}
	if createTemplate.TimeTilDormantMillis != nil {
		dormantTTL = time.Duration(*createTemplate.TimeTilDormantMillis) * time.Millisecond
	}
	if createTemplate.TimeTilDormantAutoDeleteMillis != nil {
		dormantAutoDeletionTTL = time.Duration(*createTemplate.TimeTilDormantAutoDeleteMillis) * time.Millisecond
	}

	var (
		validErrs                            []codersdk.ValidationError
		autostopRequirementDaysOfWeekParsed  uint8
		autostartRequirementDaysOfWeekParsed uint8
	)
	if defaultTTL < 0 {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "default_ttl_ms", Detail: "Must be a positive integer."})
	}
	if activityBump < 0 {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "activity_bump_ms", Detail: "Must be a positive integer."})
	}
	if maxTTL < 0 {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "max_ttl_ms", Detail: "Must be a positive integer."})
	}
	if maxTTL != 0 && defaultTTL > maxTTL {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "default_ttl_ms", Detail: "Must be less than or equal to max_ttl_ms if max_ttl_ms is set."})
	}
	if len(autostopRequirementDaysOfWeek) > 0 {
		autostopRequirementDaysOfWeekParsed, err = codersdk.WeekdaysToBitmap(autostopRequirementDaysOfWeek)
		if err != nil {
			validErrs = append(validErrs, codersdk.ValidationError{Field: "autostop_requirement.days_of_week", Detail: err.Error()})
		}
	}
	if len(autostartRequirementDaysOfWeek) > 0 {
		autostartRequirementDaysOfWeekParsed, err = codersdk.WeekdaysToBitmap(autostartRequirementDaysOfWeek)
		if err != nil {
			validErrs = append(validErrs, codersdk.ValidationError{Field: "autostart_requirement.days_of_week", Detail: err.Error()})
		}
	}
	if createTemplate.MaxTTLMillis != nil {
		maxTTL = time.Duration(*createTemplate.MaxTTLMillis) * time.Millisecond
	}
	if maxTTL != 0 && len(autostopRequirementDaysOfWeek) > 0 {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "autostop_requirement.days_of_week", Detail: "Cannot be set if max_ttl_ms is set."})
	}
	if autostopRequirementWeeks < 0 {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "autostop_requirement.weeks", Detail: "Must be a positive integer."})
	}
	if autostopRequirementWeeks > schedule.MaxTemplateAutostopRequirementWeeks {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "autostop_requirement.weeks", Detail: fmt.Sprintf("Must be less than %d.", schedule.MaxTemplateAutostopRequirementWeeks)})
	}
	if failureTTL < 0 {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "failure_ttl_ms", Detail: "Must be a positive integer."})
	}
	if dormantTTL < 0 {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "time_til_dormant_autodeletion_ms", Detail: "Must be a positive integer."})
	}
	if dormantAutoDeletionTTL < 0 {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "time_til_dormant_autodeletion_ms", Detail: "Must be a positive integer."})
	}

	if len(validErrs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid create template request.",
			Validations: validErrs,
		})
		return
	}

	var (
		dbTemplate database.Template

		allowUserCancelWorkspaceJobs = ptr.NilToDefault(createTemplate.AllowUserCancelWorkspaceJobs, false)
		allowUserAutostart           = ptr.NilToDefault(createTemplate.AllowUserAutostart, true)
		allowUserAutostop            = ptr.NilToDefault(createTemplate.AllowUserAutostop, true)
	)

	defaultsGroups := database.TemplateACL{}
	if !createTemplate.DisableEveryoneGroupAccess {
		// The organization ID is used as the group ID for the everyone group
		// in this organization.
		defaultsGroups[organization.ID.String()] = []rbac.Action{rbac.ActionRead}
	}
	err = api.Database.InTx(func(tx database.Store) error {
		now := dbtime.Now()
		id := uuid.New()
		err = tx.InsertTemplate(ctx, database.InsertTemplateParams{
			ID:                           id,
			CreatedAt:                    now,
			UpdatedAt:                    now,
			OrganizationID:               organization.ID,
			Name:                         createTemplate.Name,
			Provisioner:                  importJob.Provisioner,
			ActiveVersionID:              templateVersion.ID,
			Description:                  createTemplate.Description,
			CreatedBy:                    apiKey.UserID,
			UserACL:                      database.TemplateACL{},
			GroupACL:                     defaultsGroups,
			DisplayName:                  createTemplate.DisplayName,
			Icon:                         createTemplate.Icon,
			AllowUserCancelWorkspaceJobs: allowUserCancelWorkspaceJobs,
			MaxPortSharingLevel:          database.AppSharingLevelOwner,
		})
		if err != nil {
			return xerrors.Errorf("insert template: %s", err)
		}

		if createTemplate.RequireActiveVersion {
			err = (*api.AccessControlStore.Load()).SetTemplateAccessControl(ctx, tx, id, dbauthz.TemplateAccessControl{
				RequireActiveVersion: createTemplate.RequireActiveVersion,
			})
			if err != nil {
				return xerrors.Errorf("set template access control: %w", err)
			}
		}

		dbTemplate, err = tx.GetTemplateByID(ctx, id)
		if err != nil {
			return xerrors.Errorf("get template by id: %s", err)
		}

		dbTemplate, err = (*api.TemplateScheduleStore.Load()).Set(ctx, tx, dbTemplate, schedule.TemplateScheduleOptions{
			UserAutostartEnabled: allowUserAutostart,
			UserAutostopEnabled:  allowUserAutostop,
			UseMaxTTL:            maxTTL > 0,
			DefaultTTL:           defaultTTL,
			ActivityBump:         activityBump,
			MaxTTL:               maxTTL,
			// Some of these values are enterprise-only, but the
			// TemplateScheduleStore will handle avoiding setting them if
			// unlicensed.
			AutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: autostopRequirementDaysOfWeekParsed,
				Weeks:      autostopRequirementWeeks,
			},
			AutostartRequirement: schedule.TemplateAutostartRequirement{
				DaysOfWeek: autostartRequirementDaysOfWeekParsed,
			},
			FailureTTL:               failureTTL,
			TimeTilDormant:           dormantTTL,
			TimeTilDormantAutoDelete: dormantAutoDeletionTTL,
		})
		if err != nil {
			return xerrors.Errorf("set template schedule options: %s", err)
		}

		templateAudit.New = dbTemplate

		err = tx.UpdateTemplateVersionByID(ctx, database.UpdateTemplateVersionByIDParams{
			ID: templateVersion.ID,
			TemplateID: uuid.NullUUID{
				UUID:  dbTemplate.ID,
				Valid: true,
			},
			UpdatedAt: dbtime.Now(),
			Name:      templateVersion.Name,
			Message:   templateVersion.Message,
		})
		if err != nil {
			return xerrors.Errorf("insert template version: %s", err)
		}
		newTemplateVersion := templateVersion
		newTemplateVersion.TemplateID = uuid.NullUUID{
			UUID:  dbTemplate.ID,
			Valid: true,
		}
		templateVersionAudit.New = newTemplateVersion

		return nil
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error inserting template.",
			Detail:  err.Error(),
		})
		return
	}

	api.Telemetry.Report(&telemetry.Snapshot{
		Templates:        []telemetry.Template{telemetry.ConvertTemplate(dbTemplate)},
		TemplateVersions: []telemetry.TemplateVersion{telemetry.ConvertTemplateVersion(templateVersion)},
	})

	httpapi.Write(ctx, rw, http.StatusCreated, api.convertTemplate(dbTemplate))
}

// @Summary Get templates by organization
// @ID get-templates-by-organization
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {array} codersdk.Template
// @Router /organizations/{organization}/templates [get]
func (api *API) templatesByOrganization(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)

	p := httpapi.NewQueryParamParser()
	values := r.URL.Query()

	deprecated := sql.NullBool{}
	if values.Has("deprecated") {
		deprecated = sql.NullBool{
			Bool:  p.Boolean(values, false, "deprecated"),
			Valid: true,
		}
	}
	if len(p.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid query params.",
			Validations: p.Errors,
		})
		return
	}

	prepared, err := api.HTTPAuth.AuthorizeSQLFilter(r, rbac.ActionRead, rbac.ResourceTemplate.Type)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error preparing sql filter.",
			Detail:  err.Error(),
		})
		return
	}

	// Filter templates based on rbac permissions
	templates, err := api.Database.GetAuthorizedTemplates(ctx, database.GetTemplatesWithFilterParams{
		OrganizationID: organization.ID,
		Deprecated:     deprecated,
	}, prepared)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}

	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching templates in organization.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, api.convertTemplates(templates))
}

// @Summary Get templates by organization and template name
// @ID get-templates-by-organization-and-template-name
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param organization path string true "Organization ID" format(uuid)
// @Param templatename path string true "Template name"
// @Success 200 {object} codersdk.Template
// @Router /organizations/{organization}/templates/{templatename} [get]
func (api *API) templateByOrganizationAndName(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)
	templateName := chi.URLParam(r, "templatename")
	template, err := api.Database.GetTemplateByOrganizationAndName(ctx, database.GetTemplateByOrganizationAndNameParams{
		OrganizationID: organization.ID,
		Name:           templateName,
	})
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}

		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, api.convertTemplate(template))
}

// @Summary Update template metadata by ID
// @ID update-template-metadata-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param template path string true "Template ID" format(uuid)
// @Success 200 {object} codersdk.Template
// @Router /templates/{template} [patch]
func (api *API) patchTemplateMeta(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		template          = httpmw.TemplateParam(r)
		auditor           = *api.Auditor.Load()
		portSharer        = *api.PortSharer.Load()
		aReq, commitAudit = audit.InitRequest[database.Template](rw, &audit.RequestParams{
			Audit:   auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()
	aReq.Old = template

	scheduleOpts, err := (*api.TemplateScheduleStore.Load()).Get(ctx, api.Database, template.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template schedule options.",
			Detail:  err.Error(),
		})
		return
	}

	var req codersdk.UpdateTemplateMeta
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	var (
		validErrs                            []codersdk.ValidationError
		autostopRequirementDaysOfWeekParsed  uint8
		autostartRequirementDaysOfWeekParsed uint8
	)
	if req.DefaultTTLMillis < 0 {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "default_ttl_ms", Detail: "Must be a positive integer."})
	}
	if req.ActivityBumpMillis < 0 {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "activity_bump_ms", Detail: "Must be a positive integer."})
	}
	if req.MaxTTLMillis < 0 {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "max_ttl_ms", Detail: "Must be a positive integer."})
	}
	if req.MaxTTLMillis != 0 && req.DefaultTTLMillis > req.MaxTTLMillis {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "default_ttl_ms", Detail: "Must be less than or equal to max_ttl_ms if max_ttl_ms is set."})
	}
	if req.MaxTTLMillis != 0 && req.AutostopRequirement != nil && len(req.AutostopRequirement.DaysOfWeek) > 0 {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "autostop_requirement.days_of_week", Detail: "Cannot be set if max_ttl_ms is set."})
	}
	useMaxTTL := req.MaxTTLMillis > 0
	if req.AutostopRequirement == nil {
		req.AutostopRequirement = &codersdk.TemplateAutostopRequirement{
			DaysOfWeek: codersdk.BitmapToWeekdays(scheduleOpts.AutostopRequirement.DaysOfWeek),
			Weeks:      scheduleOpts.AutostopRequirement.Weeks,
		}
	}
	if len(req.AutostopRequirement.DaysOfWeek) > 0 {
		autostopRequirementDaysOfWeekParsed, err = codersdk.WeekdaysToBitmap(req.AutostopRequirement.DaysOfWeek)
		if err != nil {
			validErrs = append(validErrs, codersdk.ValidationError{Field: "autostop_requirement.days_of_week", Detail: err.Error()})
		}
	}
	if req.AutostartRequirement == nil {
		req.AutostartRequirement = &codersdk.TemplateAutostartRequirement{
			DaysOfWeek: codersdk.BitmapToWeekdays(scheduleOpts.AutostartRequirement.DaysOfWeek),
		}
	}
	if len(req.AutostartRequirement.DaysOfWeek) > 0 {
		autostartRequirementDaysOfWeekParsed, err = codersdk.WeekdaysToBitmap(req.AutostartRequirement.DaysOfWeek)
		if err != nil {
			validErrs = append(validErrs, codersdk.ValidationError{Field: "autostart_requirement.days_of_week", Detail: err.Error()})
		}
	}
	if req.AutostopRequirement.Weeks < 0 {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "autostop_requirement.weeks", Detail: "Must be a positive integer."})
	}
	if req.AutostopRequirement.Weeks == 0 {
		req.AutostopRequirement.Weeks = 1
	}
	if template.AutostopRequirementWeeks <= 0 {
		template.AutostopRequirementWeeks = 1
	}
	if req.AutostopRequirement.Weeks > schedule.MaxTemplateAutostopRequirementWeeks {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "autostop_requirement.weeks", Detail: fmt.Sprintf("Must be less than %d.", schedule.MaxTemplateAutostopRequirementWeeks)})
	}
	// Defaults to the existing.
	deprecationMessage := template.Deprecated
	if req.DeprecationMessage != nil {
		deprecationMessage = *req.DeprecationMessage
	}

	// The minimum valid value for a dormant TTL is 1 minute. This is
	// to ensure an uninformed user does not send an unintentionally
	// small number resulting in potentially catastrophic consequences.
	const minTTL = 1000 * 60
	if req.FailureTTLMillis < 0 || (req.FailureTTLMillis > 0 && req.FailureTTLMillis < minTTL) {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "failure_ttl_ms", Detail: "Value must be at least one minute."})
	}
	if req.TimeTilDormantMillis < 0 || (req.TimeTilDormantMillis > 0 && req.TimeTilDormantMillis < minTTL) {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "time_til_dormant_ms", Detail: "Value must be at least one minute."})
	}
	if req.TimeTilDormantAutoDeleteMillis < 0 || (req.TimeTilDormantAutoDeleteMillis > 0 && req.TimeTilDormantAutoDeleteMillis < minTTL) {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "time_til_dormant_autodelete_ms", Detail: "Value must be at least one minute."})
	}
	maxPortShareLevel := template.MaxPortSharingLevel
	if req.MaxPortShareLevel != nil && *req.MaxPortShareLevel != codersdk.WorkspaceAgentPortShareLevel(maxPortShareLevel) {
		err := portSharer.ValidateTemplateMaxPortSharingLevel(*req.MaxPortShareLevel)
		if err != nil {
			validErrs = append(validErrs, codersdk.ValidationError{Field: "max_port_sharing_level", Detail: err.Error()})
		} else {
			maxPortShareLevel = database.AppSharingLevel(*req.MaxPortShareLevel)
		}
	}

	if len(validErrs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid request to update template metadata!",
			Validations: validErrs,
		})
		return
	}

	var updated database.Template
	err = api.Database.InTx(func(tx database.Store) error {
		if req.Name == template.Name &&
			req.Description == template.Description &&
			req.DisplayName == template.DisplayName &&
			req.Icon == template.Icon &&
			req.AllowUserAutostart == template.AllowUserAutostart &&
			req.AllowUserAutostop == template.AllowUserAutostop &&
			req.AllowUserCancelWorkspaceJobs == template.AllowUserCancelWorkspaceJobs &&
			req.DefaultTTLMillis == time.Duration(template.DefaultTTL).Milliseconds() &&
			req.ActivityBumpMillis == time.Duration(template.ActivityBump).Milliseconds() &&
			useMaxTTL == scheduleOpts.UseMaxTTL &&
			req.MaxTTLMillis == time.Duration(template.MaxTTL).Milliseconds() &&
			autostopRequirementDaysOfWeekParsed == scheduleOpts.AutostopRequirement.DaysOfWeek &&
			autostartRequirementDaysOfWeekParsed == scheduleOpts.AutostartRequirement.DaysOfWeek &&
			req.AutostopRequirement.Weeks == scheduleOpts.AutostopRequirement.Weeks &&
			req.FailureTTLMillis == time.Duration(template.FailureTTL).Milliseconds() &&
			req.TimeTilDormantMillis == time.Duration(template.TimeTilDormant).Milliseconds() &&
			req.TimeTilDormantAutoDeleteMillis == time.Duration(template.TimeTilDormantAutoDelete).Milliseconds() &&
			req.RequireActiveVersion == template.RequireActiveVersion &&
			(deprecationMessage == template.Deprecated) &&
			maxPortShareLevel == template.MaxPortSharingLevel {
			return nil
		}

		// Users should not be able to clear the template name in the UI
		name := req.Name
		if name == "" {
			name = template.Name
		}

		groupACL := template.GroupACL
		if req.DisableEveryoneGroupAccess {
			delete(groupACL, template.OrganizationID.String())
		}

		var err error
		err = tx.UpdateTemplateMetaByID(ctx, database.UpdateTemplateMetaByIDParams{
			ID:                           template.ID,
			UpdatedAt:                    dbtime.Now(),
			Name:                         name,
			DisplayName:                  req.DisplayName,
			Description:                  req.Description,
			Icon:                         req.Icon,
			AllowUserCancelWorkspaceJobs: req.AllowUserCancelWorkspaceJobs,
			GroupACL:                     groupACL,
			MaxPortSharingLevel:          maxPortShareLevel,
		})
		if err != nil {
			return xerrors.Errorf("update template metadata: %w", err)
		}

		if template.RequireActiveVersion != req.RequireActiveVersion || deprecationMessage != template.Deprecated {
			err = (*api.AccessControlStore.Load()).SetTemplateAccessControl(ctx, tx, template.ID, dbauthz.TemplateAccessControl{
				RequireActiveVersion: req.RequireActiveVersion,
				Deprecated:           deprecationMessage,
			})
			if err != nil {
				return xerrors.Errorf("set template access control: %w", err)
			}
		}

		updated, err = tx.GetTemplateByID(ctx, template.ID)
		if err != nil {
			return xerrors.Errorf("fetch updated template metadata: %w", err)
		}

		defaultTTL := time.Duration(req.DefaultTTLMillis) * time.Millisecond
		activityBump := time.Duration(req.ActivityBumpMillis) * time.Millisecond
		maxTTL := time.Duration(req.MaxTTLMillis) * time.Millisecond
		failureTTL := time.Duration(req.FailureTTLMillis) * time.Millisecond
		inactivityTTL := time.Duration(req.TimeTilDormantMillis) * time.Millisecond
		timeTilDormantAutoDelete := time.Duration(req.TimeTilDormantAutoDeleteMillis) * time.Millisecond

		if defaultTTL != time.Duration(template.DefaultTTL) ||
			activityBump != time.Duration(template.ActivityBump) ||
			useMaxTTL != scheduleOpts.UseMaxTTL ||
			maxTTL != time.Duration(template.MaxTTL) ||
			autostopRequirementDaysOfWeekParsed != scheduleOpts.AutostopRequirement.DaysOfWeek ||
			autostartRequirementDaysOfWeekParsed != scheduleOpts.AutostartRequirement.DaysOfWeek ||
			req.AutostopRequirement.Weeks != scheduleOpts.AutostopRequirement.Weeks ||
			failureTTL != time.Duration(template.FailureTTL) ||
			inactivityTTL != time.Duration(template.TimeTilDormant) ||
			timeTilDormantAutoDelete != time.Duration(template.TimeTilDormantAutoDelete) ||
			req.AllowUserAutostart != template.AllowUserAutostart ||
			req.AllowUserAutostop != template.AllowUserAutostop {
			updated, err = (*api.TemplateScheduleStore.Load()).Set(ctx, tx, updated, schedule.TemplateScheduleOptions{
				// Some of these values are enterprise-only, but the
				// TemplateScheduleStore will handle avoiding setting them if
				// unlicensed.
				UserAutostartEnabled: req.AllowUserAutostart,
				UserAutostopEnabled:  req.AllowUserAutostop,
				DefaultTTL:           defaultTTL,
				ActivityBump:         activityBump,
				UseMaxTTL:            useMaxTTL,
				MaxTTL:               maxTTL,
				AutostopRequirement: schedule.TemplateAutostopRequirement{
					DaysOfWeek: autostopRequirementDaysOfWeekParsed,
					Weeks:      req.AutostopRequirement.Weeks,
				},
				AutostartRequirement: schedule.TemplateAutostartRequirement{
					DaysOfWeek: autostartRequirementDaysOfWeekParsed,
				},
				FailureTTL:                failureTTL,
				TimeTilDormant:            inactivityTTL,
				TimeTilDormantAutoDelete:  timeTilDormantAutoDelete,
				UpdateWorkspaceLastUsedAt: req.UpdateWorkspaceLastUsedAt,
				UpdateWorkspaceDormantAt:  req.UpdateWorkspaceDormantAt,
			})
			if err != nil {
				return xerrors.Errorf("set template schedule options: %w", err)
			}
		}

		return nil
	}, nil)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	if updated.UpdatedAt.IsZero() {
		aReq.New = template
		httpapi.Write(ctx, rw, http.StatusNotModified, nil)
		return
	}
	aReq.New = updated

	httpapi.Write(ctx, rw, http.StatusOK, api.convertTemplate(updated))
}

// @Summary Get template DAUs by ID
// @ID get-template-daus-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param template path string true "Template ID" format(uuid)
// @Success 200 {object} codersdk.DAUsResponse
// @Router /templates/{template}/daus [get]
func (api *API) templateDAUs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	template := httpmw.TemplateParam(r)

	vals := r.URL.Query()
	p := httpapi.NewQueryParamParser()
	tzOffset := p.Int(vals, 0, "tz_offset")
	p.ErrorExcessParams(vals)
	if len(p.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: p.Errors,
		})
		return
	}

	_, resp, _ := api.metricsCache.TemplateDAUs(template.ID, tzOffset)
	if resp == nil || resp.Entries == nil {
		httpapi.Write(ctx, rw, http.StatusOK, &codersdk.DAUsResponse{
			Entries: []codersdk.DAUEntry{},
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// @Summary Get template examples by organization
// @ID get-template-examples-by-organization
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {array} codersdk.TemplateExample
// @Router /organizations/{organization}/templates/examples [get]
func (api *API) templateExamples(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx          = r.Context()
		organization = httpmw.OrganizationParam(r)
	)

	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceTemplate.InOrg(organization.ID)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	ex, err := examples.List()
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching examples.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, ex)
}

func (api *API) convertTemplates(templates []database.Template) []codersdk.Template {
	apiTemplates := make([]codersdk.Template, 0, len(templates))

	for _, template := range templates {
		apiTemplates = append(apiTemplates, api.convertTemplate(template))
	}

	// Sort templates by ActiveUserCount DESC
	sort.SliceStable(apiTemplates, func(i, j int) bool {
		return apiTemplates[i].ActiveUserCount > apiTemplates[j].ActiveUserCount
	})

	return apiTemplates
}

func (api *API) convertTemplate(
	template database.Template,
) codersdk.Template {
	templateAccessControl := (*(api.Options.AccessControlStore.Load())).GetTemplateAccessControl(template)

	owners := 0
	o, ok := api.metricsCache.TemplateWorkspaceOwners(template.ID)
	if ok {
		owners = o
	}

	buildTimeStats := api.metricsCache.TemplateBuildTimeStats(template.ID)

	autostopRequirementWeeks := template.AutostopRequirementWeeks
	if autostopRequirementWeeks < 1 {
		autostopRequirementWeeks = 1
	}

	return codersdk.Template{
		ID:                             template.ID,
		CreatedAt:                      template.CreatedAt,
		UpdatedAt:                      template.UpdatedAt,
		OrganizationID:                 template.OrganizationID,
		Name:                           template.Name,
		DisplayName:                    template.DisplayName,
		Provisioner:                    codersdk.ProvisionerType(template.Provisioner),
		ActiveVersionID:                template.ActiveVersionID,
		ActiveUserCount:                owners,
		BuildTimeStats:                 buildTimeStats,
		Description:                    template.Description,
		Icon:                           template.Icon,
		DefaultTTLMillis:               time.Duration(template.DefaultTTL).Milliseconds(),
		ActivityBumpMillis:             time.Duration(template.ActivityBump).Milliseconds(),
		UseMaxTTL:                      template.UseMaxTtl,
		MaxTTLMillis:                   time.Duration(template.MaxTTL).Milliseconds(),
		CreatedByID:                    template.CreatedBy,
		CreatedByName:                  template.CreatedByUsername,
		AllowUserAutostart:             template.AllowUserAutostart,
		AllowUserAutostop:              template.AllowUserAutostop,
		AllowUserCancelWorkspaceJobs:   template.AllowUserCancelWorkspaceJobs,
		FailureTTLMillis:               time.Duration(template.FailureTTL).Milliseconds(),
		TimeTilDormantMillis:           time.Duration(template.TimeTilDormant).Milliseconds(),
		TimeTilDormantAutoDeleteMillis: time.Duration(template.TimeTilDormantAutoDelete).Milliseconds(),
		AutostopRequirement: codersdk.TemplateAutostopRequirement{
			DaysOfWeek: codersdk.BitmapToWeekdays(uint8(template.AutostopRequirementDaysOfWeek)),
			Weeks:      autostopRequirementWeeks,
		},
		AutostartRequirement: codersdk.TemplateAutostartRequirement{
			DaysOfWeek: codersdk.BitmapToWeekdays(template.AutostartAllowedDays()),
		},
		// These values depend on entitlements and come from the templateAccessControl
		RequireActiveVersion: templateAccessControl.RequireActiveVersion,
		Deprecated:           templateAccessControl.IsDeprecated(),
		DeprecationMessage:   templateAccessControl.Deprecated,
		MaxPortShareLevel:    codersdk.WorkspaceAgentPortShareLevel(template.MaxPortSharingLevel),
	}
}
