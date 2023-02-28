package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/telemetry"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/examples"
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

	if !api.Authorize(r, rbac.ActionRead, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

	createdByNameMap, err := getCreatedByNamesByTemplateIDs(ctx, api.Database, []database.Template{template})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching creator name.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, api.convertTemplate(template, createdByNameMap[template.ID.String()]))
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

	if !api.Authorize(r, rbac.ActionDelete, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

	// TODO: This just returns the workspaces a user can view. We should use
	// a system function to get all workspaces that use this template.
	// This data should never be exposed to the user aside from a non-zero count.
	// Or we move this into a postgres constraint.
	workspaces, err := api.Database.GetWorkspaces(ctx, database.GetWorkspacesParams{
		TemplateIds: []uuid.UUID{template.ID},
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
		UpdatedAt: database.Now(),
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

	if !api.Authorize(r, rbac.ActionCreate, rbac.ResourceTemplate.InOrg(organization.ID)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	if !httpapi.Read(ctx, rw, r, &createTemplate) {
		return
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
	templateVersionAudit.Old = templateVersion

	importJob, err := api.Database.GetProvisionerJobByID(ctx, templateVersion.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}

	var ttl time.Duration
	if createTemplate.DefaultTTLMillis != nil {
		ttl = time.Duration(*createTemplate.DefaultTTLMillis) * time.Millisecond
	}
	if ttl < 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid create template request.",
			Validations: []codersdk.ValidationError{
				{Field: "default_ttl_ms", Detail: "Must be a positive integer."},
			},
		})
		return
	}

	var allowUserCancelWorkspaceJobs bool
	if createTemplate.AllowUserCancelWorkspaceJobs != nil {
		allowUserCancelWorkspaceJobs = *createTemplate.AllowUserCancelWorkspaceJobs
	}

	var dbTemplate database.Template
	var template codersdk.Template
	err = api.Database.InTx(func(tx database.Store) error {
		now := database.Now()
		dbTemplate, err = tx.InsertTemplate(ctx, database.InsertTemplateParams{
			ID:              uuid.New(),
			CreatedAt:       now,
			UpdatedAt:       now,
			OrganizationID:  organization.ID,
			Name:            createTemplate.Name,
			Provisioner:     importJob.Provisioner,
			ActiveVersionID: templateVersion.ID,
			Description:     createTemplate.Description,
			DefaultTTL:      int64(ttl),
			CreatedBy:       apiKey.UserID,
			UserACL:         database.TemplateACL{},
			GroupACL: database.TemplateACL{
				organization.ID.String(): []rbac.Action{rbac.ActionRead},
			},
			DisplayName:                  createTemplate.DisplayName,
			Icon:                         createTemplate.Icon,
			AllowUserCancelWorkspaceJobs: allowUserCancelWorkspaceJobs,
		})
		if err != nil {
			return xerrors.Errorf("insert template: %s", err)
		}

		templateAudit.New = dbTemplate

		err = tx.UpdateTemplateVersionByID(ctx, database.UpdateTemplateVersionByIDParams{
			ID: templateVersion.ID,
			TemplateID: uuid.NullUUID{
				UUID:  dbTemplate.ID,
				Valid: true,
			},
			UpdatedAt: database.Now(),
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

		for _, parameterValue := range createTemplate.ParameterValues {
			_, err = tx.InsertParameterValue(ctx, database.InsertParameterValueParams{
				ID:                uuid.New(),
				Name:              parameterValue.Name,
				CreatedAt:         database.Now(),
				UpdatedAt:         database.Now(),
				Scope:             database.ParameterScopeTemplate,
				ScopeID:           template.ID,
				SourceScheme:      database.ParameterSourceScheme(parameterValue.SourceScheme),
				SourceValue:       parameterValue.SourceValue,
				DestinationScheme: database.ParameterDestinationScheme(parameterValue.DestinationScheme),
			})
			if err != nil {
				return xerrors.Errorf("insert parameter value: %w", err)
			}
		}

		createdByNameMap, err := getCreatedByNamesByTemplateIDs(ctx, tx, []database.Template{dbTemplate})
		if err != nil {
			return xerrors.Errorf("get creator name: %w", err)
		}

		template = api.convertTemplate(dbTemplate, createdByNameMap[dbTemplate.ID.String()])
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

	httpapi.Write(ctx, rw, http.StatusCreated, template)
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

	createdByNameMap, err := getCreatedByNamesByTemplateIDs(ctx, api.Database, templates)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching creator names.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, api.convertTemplates(templates, createdByNameMap))
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
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.ResourceNotFound(rw)
			return
		}

		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template.",
			Detail:  err.Error(),
		})
		return
	}

	if !api.Authorize(r, rbac.ActionRead, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

	createdByNameMap, err := getCreatedByNamesByTemplateIDs(ctx, api.Database, []database.Template{template})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching creator name.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, api.convertTemplate(template, createdByNameMap[template.ID.String()]))
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
		aReq, commitAudit = audit.InitRequest[database.Template](rw, &audit.RequestParams{
			Audit:   auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()
	aReq.Old = template

	if !api.Authorize(r, rbac.ActionUpdate, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var req codersdk.UpdateTemplateMeta
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	var validErrs []codersdk.ValidationError
	if req.DefaultTTLMillis < 0 {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "default_ttl_ms", Detail: "Must be a positive integer."})
	}

	if len(validErrs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid request to update template metadata!",
			Validations: validErrs,
		})
		return
	}

	var updated database.Template
	err := api.Database.InTx(func(tx database.Store) error {
		if req.Name == template.Name &&
			req.Description == template.Description &&
			req.DisplayName == template.DisplayName &&
			req.Icon == template.Icon &&
			req.AllowUserCancelWorkspaceJobs == template.AllowUserCancelWorkspaceJobs &&
			req.DefaultTTLMillis == time.Duration(template.DefaultTTL).Milliseconds() {
			return nil
		}

		// Update template metadata -- empty fields are not overwritten,
		// except for display_name, icon, and default_ttl.
		// The exception is required to clear content of these fields with UI.
		name := req.Name
		displayName := req.DisplayName
		desc := req.Description
		icon := req.Icon
		maxTTL := time.Duration(req.DefaultTTLMillis) * time.Millisecond
		allowUserCancelWorkspaceJobs := req.AllowUserCancelWorkspaceJobs

		if name == "" {
			name = template.Name
		}
		if desc == "" {
			desc = template.Description
		}

		var err error
		updated, err = tx.UpdateTemplateMetaByID(ctx, database.UpdateTemplateMetaByIDParams{
			ID:                           template.ID,
			UpdatedAt:                    database.Now(),
			Name:                         name,
			DisplayName:                  displayName,
			Description:                  desc,
			Icon:                         icon,
			DefaultTTL:                   int64(maxTTL),
			AllowUserCancelWorkspaceJobs: allowUserCancelWorkspaceJobs,
		})
		if err != nil {
			return err
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

	createdByNameMap, err := getCreatedByNamesByTemplateIDs(ctx, api.Database, []database.Template{updated})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching creator name.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, api.convertTemplate(updated, createdByNameMap[updated.ID.String()]))
}

// @Summary Get template DAUs by ID
// @ID get-template-daus-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param template path string true "Template ID" format(uuid)
// @Success 200 {object} codersdk.TemplateDAUsResponse
// @Router /templates/{template}/daus [get]
func (api *API) templateDAUs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	template := httpmw.TemplateParam(r)
	if !api.Authorize(r, rbac.ActionRead, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

	resp, _ := api.metricsCache.TemplateDAUs(template.ID)
	if resp == nil || resp.Entries == nil {
		httpapi.Write(ctx, rw, http.StatusOK, &codersdk.TemplateDAUsResponse{
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

func getCreatedByNamesByTemplateIDs(ctx context.Context, db database.Store, templates []database.Template) (map[string]string, error) {
	creators := make(map[string]string, len(templates))
	for _, template := range templates {
		creator, err := db.GetUserByID(ctx, template.CreatedBy)
		if err != nil {
			return map[string]string{}, err
		}
		creators[template.ID.String()] = creator.Username
	}
	return creators, nil
}

func (api *API) convertTemplates(templates []database.Template, createdByNameMap map[string]string) []codersdk.Template {
	apiTemplates := make([]codersdk.Template, 0, len(templates))

	for _, template := range templates {
		apiTemplates = append(apiTemplates, api.convertTemplate(template, createdByNameMap[template.ID.String()]))
	}

	// Sort templates by ActiveUserCount DESC
	sort.SliceStable(apiTemplates, func(i, j int) bool {
		return apiTemplates[i].ActiveUserCount > apiTemplates[j].ActiveUserCount
	})

	return apiTemplates
}

func (api *API) convertTemplate(
	template database.Template, createdByName string,
) codersdk.Template {
	activeCount, _ := api.metricsCache.TemplateUniqueUsers(template.ID)

	buildTimeStats := api.metricsCache.TemplateBuildTimeStats(template.ID)

	return codersdk.Template{
		ID:                           template.ID,
		CreatedAt:                    template.CreatedAt,
		UpdatedAt:                    template.UpdatedAt,
		OrganizationID:               template.OrganizationID,
		Name:                         template.Name,
		DisplayName:                  template.DisplayName,
		Provisioner:                  codersdk.ProvisionerType(template.Provisioner),
		ActiveVersionID:              template.ActiveVersionID,
		ActiveUserCount:              activeCount,
		BuildTimeStats:               buildTimeStats,
		Description:                  template.Description,
		Icon:                         template.Icon,
		DefaultTTLMillis:             time.Duration(template.DefaultTTL).Milliseconds(),
		CreatedByID:                  template.CreatedBy,
		CreatedByName:                createdByName,
		AllowUserCancelWorkspaceJobs: template.AllowUserCancelWorkspaceJobs,
	}
}
