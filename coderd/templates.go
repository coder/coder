package coderd

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
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

// Auto-importable templates. These can be auto-imported after the first user
// has been created.
type AutoImportTemplate string

const (
	AutoImportTemplateKubernetes AutoImportTemplate = "kubernetes"
)

// Returns a single template.
func (api *API) template(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	template := httpmw.TemplateParam(r)

	if !api.Authorize(r, rbac.ActionRead, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

	workspaceCounts, err := api.Database.GetWorkspaceOwnerCountsByTemplateIDs(ctx, []uuid.UUID{template.ID})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace count.",
			Detail:  err.Error(),
		})
		return
	}

	count := uint32(0)
	if len(workspaceCounts) > 0 {
		count = uint32(workspaceCounts[0].Count)
	}

	createdByNameMap, err := getCreatedByNamesByTemplateIDs(ctx, api.Database, []database.Template{template})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching creator name.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, api.convertTemplate(template, count, createdByNameMap[template.ID.String()]))
}

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
		httpapi.Write(ctx, rw, http.StatusPreconditionFailed, codersdk.Response{
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

		template = api.convertTemplate(dbTemplate, 0, createdByNameMap[dbTemplate.ID.String()])
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

	templateIDs := make([]uuid.UUID, 0, len(templates))

	for _, template := range templates {
		templateIDs = append(templateIDs, template.ID)
	}
	workspaceCounts, err := api.Database.GetWorkspaceOwnerCountsByTemplateIDs(ctx, templateIDs)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace counts.",
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

	httpapi.Write(ctx, rw, http.StatusOK, api.convertTemplates(templates, workspaceCounts, createdByNameMap))
}

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

	workspaceCounts, err := api.Database.GetWorkspaceOwnerCountsByTemplateIDs(ctx, []uuid.UUID{template.ID})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace counts.",
			Detail:  err.Error(),
		})
		return
	}

	count := uint32(0)
	if len(workspaceCounts) > 0 {
		count = uint32(workspaceCounts[0].Count)
	}

	createdByNameMap, err := getCreatedByNamesByTemplateIDs(ctx, api.Database, []database.Template{template})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching creator name.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, api.convertTemplate(template, count, createdByNameMap[template.ID.String()]))
}

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

	count := uint32(0)
	var updated database.Template
	err := api.Database.InTx(func(tx database.Store) error {
		// Fetch workspace counts
		workspaceCounts, err := tx.GetWorkspaceOwnerCountsByTemplateIDs(ctx, []uuid.UUID{template.ID})
		if xerrors.Is(err, sql.ErrNoRows) {
			err = nil
		}
		if err != nil {
			return err
		}

		if len(workspaceCounts) > 0 {
			count = uint32(workspaceCounts[0].Count)
		}

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

	httpapi.Write(ctx, rw, http.StatusOK, api.convertTemplate(updated, count, createdByNameMap[updated.ID.String()]))
}

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

type autoImportTemplateOpts struct {
	name    string
	archive []byte
	params  map[string]string
	userID  uuid.UUID
	orgID   uuid.UUID
}

func (api *API) autoImportTemplate(ctx context.Context, opts autoImportTemplateOpts) (database.Template, error) {
	var template database.Template
	err := api.Database.InTx(func(tx database.Store) error {
		// Insert the archive into the files table.
		var (
			hash = sha256.Sum256(opts.archive)
			now  = database.Now()
		)
		file, err := tx.InsertFile(ctx, database.InsertFileParams{
			ID:        uuid.New(),
			Hash:      hex.EncodeToString(hash[:]),
			CreatedAt: now,
			CreatedBy: opts.userID,
			Mimetype:  "application/x-tar",
			Data:      opts.archive,
		})
		if err != nil {
			return xerrors.Errorf("insert auto-imported template archive into files table: %w", err)
		}

		jobID := uuid.New()

		// Insert parameters
		for key, value := range opts.params {
			_, err = tx.InsertParameterValue(ctx, database.InsertParameterValueParams{
				ID:                uuid.New(),
				Name:              key,
				CreatedAt:         now,
				UpdatedAt:         now,
				Scope:             database.ParameterScopeImportJob,
				ScopeID:           jobID,
				SourceScheme:      database.ParameterSourceSchemeData,
				SourceValue:       value,
				DestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
			})
			if err != nil {
				return xerrors.Errorf("insert job-scoped parameter %q with value %q: %w", key, value, err)
			}
		}

		// Create provisioner job
		job, err := tx.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:             jobID,
			CreatedAt:      now,
			UpdatedAt:      now,
			OrganizationID: opts.orgID,
			InitiatorID:    opts.userID,
			Provisioner:    database.ProvisionerTypeTerraform,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         file.ID,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			Input:          []byte{'{', '}'},
		})
		if err != nil {
			return xerrors.Errorf("insert provisioner job: %w", err)
		}

		// Create template version
		templateVersion, err := tx.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
			ID: uuid.New(),
			TemplateID: uuid.NullUUID{
				UUID:  uuid.Nil,
				Valid: false,
			},
			OrganizationID: opts.orgID,
			CreatedAt:      now,
			UpdatedAt:      now,
			Name:           namesgenerator.GetRandomName(1),
			Readme:         "",
			JobID:          job.ID,
			CreatedBy:      opts.userID,
		})
		if err != nil {
			return xerrors.Errorf("insert template version: %w", err)
		}

		// Create template
		template, err = tx.InsertTemplate(ctx, database.InsertTemplateParams{
			ID:              uuid.New(),
			CreatedAt:       now,
			UpdatedAt:       now,
			OrganizationID:  opts.orgID,
			Name:            opts.name,
			Provisioner:     job.Provisioner,
			ActiveVersionID: templateVersion.ID,
			Description:     "This template was auto-imported by Coder.",
			DefaultTTL:      0,
			CreatedBy:       opts.userID,
			UserACL:         database.TemplateACL{},
			GroupACL: database.TemplateACL{
				opts.orgID.String(): []rbac.Action{rbac.ActionRead},
			},
		})
		if err != nil {
			return xerrors.Errorf("insert template: %w", err)
		}

		// Update template version with template ID
		err = tx.UpdateTemplateVersionByID(ctx, database.UpdateTemplateVersionByIDParams{
			ID: templateVersion.ID,
			TemplateID: uuid.NullUUID{
				UUID:  template.ID,
				Valid: true,
			},
		})
		if err != nil {
			return xerrors.Errorf("update template version to set template ID: %s", err)
		}

		// Insert parameters at the template scope
		for key, value := range opts.params {
			_, err = tx.InsertParameterValue(ctx, database.InsertParameterValueParams{
				ID:                uuid.New(),
				Name:              key,
				CreatedAt:         now,
				UpdatedAt:         now,
				Scope:             database.ParameterScopeTemplate,
				ScopeID:           template.ID,
				SourceScheme:      database.ParameterSourceSchemeData,
				SourceValue:       value,
				DestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
			})
			if err != nil {
				return xerrors.Errorf("insert template-scoped parameter %q with value %q: %w", key, value, err)
			}
		}

		return nil
	}, nil)

	return template, err
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

func (api *API) convertTemplates(templates []database.Template, workspaceCounts []database.GetWorkspaceOwnerCountsByTemplateIDsRow, createdByNameMap map[string]string) []codersdk.Template {
	apiTemplates := make([]codersdk.Template, 0, len(templates))

	for _, template := range templates {
		found := false
		for _, workspaceCount := range workspaceCounts {
			if workspaceCount.TemplateID.String() != template.ID.String() {
				continue
			}
			apiTemplates = append(apiTemplates, api.convertTemplate(template, uint32(workspaceCount.Count), createdByNameMap[template.ID.String()]))
			found = true
			break
		}
		if !found {
			apiTemplates = append(apiTemplates, api.convertTemplate(template, uint32(0), createdByNameMap[template.ID.String()]))
		}
	}

	// Sort templates by ActiveUserCount DESC
	sort.SliceStable(apiTemplates, func(i, j int) bool {
		return apiTemplates[i].ActiveUserCount > apiTemplates[j].ActiveUserCount
	})

	return apiTemplates
}

func (api *API) convertTemplate(
	template database.Template, workspaceOwnerCount uint32, createdByName string,
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
		WorkspaceOwnerCount:          workspaceOwnerCount,
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
