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

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/telemetry"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
)

var (
	maxTTLDefault               = 24 * 7 * time.Hour
	minAutostartIntervalDefault = time.Hour
)

// Auto-importable templates. These can be auto-imported after the first user
// has been created.
type AutoImportTemplate string

const (
	AutoImportTemplateKubernetes AutoImportTemplate = "kubernetes"
)

// Returns a single template.
func (api *API) template(rw http.ResponseWriter, r *http.Request) {
	template := httpmw.TemplateParam(r)

	if !api.Authorize(r, rbac.ActionRead, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

	workspaceCounts, err := api.Database.GetWorkspaceOwnerCountsByTemplateIDs(r.Context(), []uuid.UUID{template.ID})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace count.",
			Detail:  err.Error(),
		})
		return
	}

	if !api.Authorize(r, rbac.ActionRead, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

	count := uint32(0)
	if len(workspaceCounts) > 0 {
		count = uint32(workspaceCounts[0].Count)
	}

	createdByNameMap, err := getCreatedByNamesByTemplateIDs(r.Context(), api.Database, []database.Template{template})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching creator name.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertTemplate(template, count, createdByNameMap[template.ID.String()]))
}

func (api *API) deleteTemplate(rw http.ResponseWriter, r *http.Request) {
	template := httpmw.TemplateParam(r)
	if !api.Authorize(r, rbac.ActionDelete, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

	workspaces, err := api.Database.GetWorkspaces(r.Context(), database.GetWorkspacesParams{
		TemplateIds: []uuid.UUID{template.ID},
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspaces by template id.",
			Detail:  err.Error(),
		})
		return
	}
	if len(workspaces) > 0 {
		httpapi.Write(rw, http.StatusPreconditionFailed, codersdk.Response{
			Message: "All workspaces must be deleted before a template can be removed.",
		})
		return
	}
	err = api.Database.UpdateTemplateDeletedByID(r.Context(), database.UpdateTemplateDeletedByIDParams{
		ID:        template.ID,
		Deleted:   true,
		UpdatedAt: database.Now(),
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting template.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(rw, http.StatusOK, codersdk.Response{
		Message: "Template has been deleted!",
	})
}

// Create a new template in an organization.
func (api *API) postTemplateByOrganization(rw http.ResponseWriter, r *http.Request) {
	var createTemplate codersdk.CreateTemplateRequest
	organization := httpmw.OrganizationParam(r)
	apiKey := httpmw.APIKey(r)
	if !api.Authorize(r, rbac.ActionCreate, rbac.ResourceTemplate.InOrg(organization.ID)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	if !httpapi.Read(rw, r, &createTemplate) {
		return
	}
	_, err := api.Database.GetTemplateByOrganizationAndName(r.Context(), database.GetTemplateByOrganizationAndNameParams{
		OrganizationID: organization.ID,
		Name:           createTemplate.Name,
	})
	if err == nil {
		httpapi.Write(rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("Template with name %q already exists.", createTemplate.Name),
			Validations: []codersdk.ValidationError{{
				Field:  "name",
				Detail: "This value is already in use and should be unique.",
			}},
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template by name.",
			Detail:  err.Error(),
		})
		return
	}
	templateVersion, err := api.Database.GetTemplateVersionByID(r.Context(), createTemplate.VersionID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("Template version %q does not exist.", createTemplate.VersionID),
			Validations: []codersdk.ValidationError{
				{Field: "template_version_id", Detail: "Template version does not exist"},
			},
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template version.",
			Detail:  err.Error(),
		})
		return
	}
	importJob, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}

	maxTTL := maxTTLDefault
	if createTemplate.MaxTTLMillis != nil {
		maxTTL = time.Duration(*createTemplate.MaxTTLMillis) * time.Millisecond
	}
	if maxTTL < 0 {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid create template request.",
			Validations: []codersdk.ValidationError{
				{Field: "max_ttl_ms", Detail: "Must be a positive integer."},
			},
		})
		return
	}

	if maxTTL > maxTTLDefault {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid create template request.",
			Validations: []codersdk.ValidationError{
				{Field: "max_ttl_ms", Detail: "Cannot be greater than " + maxTTLDefault.String()},
			},
		})
		return
	}

	minAutostartInterval := minAutostartIntervalDefault
	if !ptr.NilOrZero(createTemplate.MinAutostartIntervalMillis) {
		minAutostartInterval = time.Duration(*createTemplate.MinAutostartIntervalMillis) * time.Millisecond
	}

	var dbTemplate database.Template
	var template codersdk.Template
	err = api.Database.InTx(func(db database.Store) error {
		now := database.Now()
		dbTemplate, err = db.InsertTemplate(r.Context(), database.InsertTemplateParams{
			ID:                   uuid.New(),
			CreatedAt:            now,
			UpdatedAt:            now,
			OrganizationID:       organization.ID,
			Name:                 createTemplate.Name,
			Provisioner:          importJob.Provisioner,
			ActiveVersionID:      templateVersion.ID,
			Description:          createTemplate.Description,
			MaxTtl:               int64(maxTTL),
			MinAutostartInterval: int64(minAutostartInterval),
			CreatedBy:            apiKey.UserID,
		})
		if err != nil {
			return xerrors.Errorf("insert template: %s", err)
		}

		err = db.UpdateTemplateVersionByID(r.Context(), database.UpdateTemplateVersionByIDParams{
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

		for _, parameterValue := range createTemplate.ParameterValues {
			_, err = db.InsertParameterValue(r.Context(), database.InsertParameterValueParams{
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

		createdByNameMap, err := getCreatedByNamesByTemplateIDs(r.Context(), db, []database.Template{dbTemplate})
		if err != nil {
			return xerrors.Errorf("get creator name: %w", err)
		}

		template = convertTemplate(dbTemplate, 0, createdByNameMap[dbTemplate.ID.String()])
		return nil
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error inserting template.",
			Detail:  err.Error(),
		})
		return
	}

	api.Telemetry.Report(&telemetry.Snapshot{
		Templates:        []telemetry.Template{telemetry.ConvertTemplate(dbTemplate)},
		TemplateVersions: []telemetry.TemplateVersion{telemetry.ConvertTemplateVersion(templateVersion)},
	})

	httpapi.Write(rw, http.StatusCreated, template)
}

func (api *API) templatesByOrganization(rw http.ResponseWriter, r *http.Request) {
	organization := httpmw.OrganizationParam(r)
	templates, err := api.Database.GetTemplatesWithFilter(r.Context(), database.GetTemplatesWithFilterParams{
		OrganizationID: organization.ID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching templates in organization.",
			Detail:  err.Error(),
		})
		return
	}

	// Filter templates based on rbac permissions
	templates, err = AuthorizeFilter(api.httpAuth, r, rbac.ActionRead, templates)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching templates.",
			Detail:  err.Error(),
		})
		return
	}

	templateIDs := make([]uuid.UUID, 0, len(templates))

	for _, template := range templates {
		templateIDs = append(templateIDs, template.ID)
	}
	workspaceCounts, err := api.Database.GetWorkspaceOwnerCountsByTemplateIDs(r.Context(), templateIDs)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace counts.",
			Detail:  err.Error(),
		})
		return
	}

	createdByNameMap, err := getCreatedByNamesByTemplateIDs(r.Context(), api.Database, templates)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching creator names.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertTemplates(templates, workspaceCounts, createdByNameMap))
}

func (api *API) templateByOrganizationAndName(rw http.ResponseWriter, r *http.Request) {
	organization := httpmw.OrganizationParam(r)
	templateName := chi.URLParam(r, "templatename")
	template, err := api.Database.GetTemplateByOrganizationAndName(r.Context(), database.GetTemplateByOrganizationAndNameParams{
		OrganizationID: organization.ID,
		Name:           templateName,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.ResourceNotFound(rw)
			return
		}

		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template.",
			Detail:  err.Error(),
		})
		return
	}

	if !api.Authorize(r, rbac.ActionRead, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

	workspaceCounts, err := api.Database.GetWorkspaceOwnerCountsByTemplateIDs(r.Context(), []uuid.UUID{template.ID})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace counts.",
			Detail:  err.Error(),
		})
		return
	}

	count := uint32(0)
	if len(workspaceCounts) > 0 {
		count = uint32(workspaceCounts[0].Count)
	}

	createdByNameMap, err := getCreatedByNamesByTemplateIDs(r.Context(), api.Database, []database.Template{template})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching creator name.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertTemplate(template, count, createdByNameMap[template.ID.String()]))
}

func (api *API) patchTemplateMeta(rw http.ResponseWriter, r *http.Request) {
	template := httpmw.TemplateParam(r)
	if !api.Authorize(r, rbac.ActionUpdate, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var req codersdk.UpdateTemplateMeta
	if !httpapi.Read(rw, r, &req) {
		return
	}

	var validErrs []codersdk.ValidationError
	if req.MaxTTLMillis < 0 {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "max_ttl_ms", Detail: "Must be a positive integer."})
	}
	if req.MinAutostartIntervalMillis < 0 {
		validErrs = append(validErrs, codersdk.ValidationError{Field: "min_autostart_interval_ms", Detail: "Must be a positive integer."})
	}
	if req.MaxTTLMillis > maxTTLDefault.Milliseconds() {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid create template request.",
			Validations: []codersdk.ValidationError{
				{Field: "max_ttl_ms", Detail: "Cannot be greater than " + maxTTLDefault.String()},
			},
		})
		return
	}

	if len(validErrs) > 0 {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid request to update template metadata!",
			Validations: validErrs,
		})
		return
	}

	count := uint32(0)
	var updated database.Template
	err := api.Database.InTx(func(s database.Store) error {
		// Fetch workspace counts
		workspaceCounts, err := s.GetWorkspaceOwnerCountsByTemplateIDs(r.Context(), []uuid.UUID{template.ID})
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
			req.Icon == template.Icon &&
			req.MaxTTLMillis == time.Duration(template.MaxTtl).Milliseconds() &&
			req.MinAutostartIntervalMillis == time.Duration(template.MinAutostartInterval).Milliseconds() {
			return nil
		}

		// Update template metadata -- empty fields are not overwritten.
		name := req.Name
		desc := req.Description
		icon := req.Icon
		maxTTL := time.Duration(req.MaxTTLMillis) * time.Millisecond
		minAutostartInterval := time.Duration(req.MinAutostartIntervalMillis) * time.Millisecond

		if name == "" {
			name = template.Name
		}
		if desc == "" {
			desc = template.Description
		}
		if minAutostartInterval == 0 {
			minAutostartInterval = time.Duration(template.MinAutostartInterval)
		}

		if err := s.UpdateTemplateMetaByID(r.Context(), database.UpdateTemplateMetaByIDParams{
			ID:                   template.ID,
			UpdatedAt:            database.Now(),
			Name:                 name,
			Description:          desc,
			Icon:                 icon,
			MaxTtl:               int64(maxTTL),
			MinAutostartInterval: int64(minAutostartInterval),
		}); err != nil {
			return err
		}

		updated, err = s.GetTemplateByID(r.Context(), template.ID)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating template metadata.",
			Detail:  err.Error(),
		})
		return
	}

	if updated.UpdatedAt.IsZero() {
		httpapi.Write(rw, http.StatusNotModified, nil)
		return
	}

	createdByNameMap, err := getCreatedByNamesByTemplateIDs(r.Context(), api.Database, []database.Template{updated})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching creator name.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertTemplate(updated, count, createdByNameMap[updated.ID.String()]))
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
	err := api.Database.InTx(func(s database.Store) error {
		// Insert the archive into the files table.
		var (
			hash = sha256.Sum256(opts.archive)
			now  = database.Now()
		)
		file, err := s.InsertFile(ctx, database.InsertFileParams{
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
			_, err = s.InsertParameterValue(ctx, database.InsertParameterValueParams{
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
		job, err := s.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:             jobID,
			CreatedAt:      now,
			UpdatedAt:      now,
			OrganizationID: opts.orgID,
			InitiatorID:    opts.userID,
			Provisioner:    database.ProvisionerTypeTerraform,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			StorageSource:  file.Hash,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			Input:          []byte{'{', '}'},
		})
		if err != nil {
			return xerrors.Errorf("insert provisioner job: %w", err)
		}

		// Create template version
		templateVersion, err := s.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
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
			CreatedBy: uuid.NullUUID{
				UUID:  opts.userID,
				Valid: true,
			},
		})
		if err != nil {
			return xerrors.Errorf("insert template version: %w", err)
		}

		// Create template
		template, err = s.InsertTemplate(ctx, database.InsertTemplateParams{
			ID:                   uuid.New(),
			CreatedAt:            now,
			UpdatedAt:            now,
			OrganizationID:       opts.orgID,
			Name:                 opts.name,
			Provisioner:          job.Provisioner,
			ActiveVersionID:      templateVersion.ID,
			Description:          "This template was auto-imported by Coder.",
			MaxTtl:               int64(maxTTLDefault),
			MinAutostartInterval: int64(minAutostartIntervalDefault),
			CreatedBy:            opts.userID,
		})
		if err != nil {
			return xerrors.Errorf("insert template: %w", err)
		}

		// Update template version with template ID
		err = s.UpdateTemplateVersionByID(ctx, database.UpdateTemplateVersionByIDParams{
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
			_, err = s.InsertParameterValue(ctx, database.InsertParameterValueParams{
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
	})

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

func convertTemplates(templates []database.Template, workspaceCounts []database.GetWorkspaceOwnerCountsByTemplateIDsRow, createdByNameMap map[string]string) []codersdk.Template {
	apiTemplates := make([]codersdk.Template, 0, len(templates))

	for _, template := range templates {
		found := false
		for _, workspaceCount := range workspaceCounts {
			if workspaceCount.TemplateID.String() != template.ID.String() {
				continue
			}
			apiTemplates = append(apiTemplates, convertTemplate(template, uint32(workspaceCount.Count), createdByNameMap[template.ID.String()]))
			found = true
			break
		}
		if !found {
			apiTemplates = append(apiTemplates, convertTemplate(template, uint32(0), createdByNameMap[template.ID.String()]))
		}
	}

	// Sort templates by WorkspaceOwnerCount DESC
	sort.SliceStable(apiTemplates, func(i, j int) bool {
		return apiTemplates[i].WorkspaceOwnerCount > apiTemplates[j].WorkspaceOwnerCount
	})

	return apiTemplates
}

func convertTemplate(template database.Template, workspaceOwnerCount uint32, createdByName string) codersdk.Template {
	return codersdk.Template{
		ID:                         template.ID,
		CreatedAt:                  template.CreatedAt,
		UpdatedAt:                  template.UpdatedAt,
		OrganizationID:             template.OrganizationID,
		Name:                       template.Name,
		Provisioner:                codersdk.ProvisionerType(template.Provisioner),
		ActiveVersionID:            template.ActiveVersionID,
		WorkspaceOwnerCount:        workspaceOwnerCount,
		Description:                template.Description,
		Icon:                       template.Icon,
		MaxTTLMillis:               time.Duration(template.MaxTtl).Milliseconds(),
		MinAutostartIntervalMillis: time.Duration(template.MinAutostartInterval).Milliseconds(),
		CreatedByID:                template.CreatedBy,
		CreatedByName:              createdByName,
	}
}
