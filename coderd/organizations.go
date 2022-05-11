package coderd

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

func (*api) organization(rw http.ResponseWriter, r *http.Request) {
	organization := httpmw.OrganizationParam(r)
	httpapi.Write(rw, http.StatusOK, convertOrganization(organization))
}

func (api *api) provisionerDaemonsByOrganization(rw http.ResponseWriter, r *http.Request) {
	daemons, err := api.Database.GetProvisionerDaemons(r.Context())
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner daemons: %s", err),
		})
		return
	}
	if daemons == nil {
		daemons = []database.ProvisionerDaemon{}
	}
	httpapi.Write(rw, http.StatusOK, daemons)
}

// Creates a new version of a template. An import job is queued to parse the storage method provided.
func (api *api) postTemplateVersionsByOrganization(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	organization := httpmw.OrganizationParam(r)
	var req codersdk.CreateTemplateVersionRequest
	if !httpapi.Read(rw, r, &req) {
		return
	}
	if req.TemplateID != uuid.Nil {
		_, err := api.Database.GetTemplateByID(r.Context(), req.TemplateID)
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
				Message: "template does not exist",
			})
			return
		}
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("get template: %s", err),
			})
			return
		}
	}

	file, err := api.Database.GetFileByHash(r.Context(), req.StorageSource)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: "file not found",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get file: %s", err),
		})
		return
	}

	var templateVersion database.TemplateVersion
	var provisionerJob database.ProvisionerJob
	err = api.Database.InTx(func(db database.Store) error {
		jobID := uuid.New()
		for _, parameterValue := range req.ParameterValues {
			_, err = db.InsertParameterValue(r.Context(), database.InsertParameterValueParams{
				ID:                uuid.New(),
				Name:              parameterValue.Name,
				CreatedAt:         database.Now(),
				UpdatedAt:         database.Now(),
				Scope:             database.ParameterScopeImportJob,
				ScopeID:           jobID,
				SourceScheme:      parameterValue.SourceScheme,
				SourceValue:       parameterValue.SourceValue,
				DestinationScheme: parameterValue.DestinationScheme,
			})
			if err != nil {
				return xerrors.Errorf("insert parameter value: %w", err)
			}
		}

		provisionerJob, err = api.Database.InsertProvisionerJob(r.Context(), database.InsertProvisionerJobParams{
			ID:             jobID,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			OrganizationID: organization.ID,
			InitiatorID:    apiKey.UserID,
			Provisioner:    req.Provisioner,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			StorageSource:  file.Hash,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			Input:          []byte{'{', '}'},
		})
		if err != nil {
			return xerrors.Errorf("insert provisioner job: %w", err)
		}

		var templateID uuid.NullUUID
		if req.TemplateID != uuid.Nil {
			templateID = uuid.NullUUID{
				UUID:  req.TemplateID,
				Valid: true,
			}
		}

		templateVersion, err = api.Database.InsertTemplateVersion(r.Context(), database.InsertTemplateVersionParams{
			ID:             uuid.New(),
			TemplateID:     templateID,
			OrganizationID: organization.ID,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			Name:           namesgenerator.GetRandomName(1),
			Description:    "",
			JobID:          provisionerJob.ID,
		})
		if err != nil {
			return xerrors.Errorf("insert template version: %w", err)
		}
		return nil
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusCreated, convertTemplateVersion(templateVersion, convertProvisionerJob(provisionerJob)))
}

// Create a new template in an organization.
func (api *api) postTemplatesByOrganization(rw http.ResponseWriter, r *http.Request) {
	var createTemplate codersdk.CreateTemplateRequest
	if !httpapi.Read(rw, r, &createTemplate) {
		return
	}
	organization := httpmw.OrganizationParam(r)
	_, err := api.Database.GetTemplateByOrganizationAndName(r.Context(), database.GetTemplateByOrganizationAndNameParams{
		OrganizationID: organization.ID,
		Name:           createTemplate.Name,
	})
	if err == nil {
		httpapi.Write(rw, http.StatusConflict, httpapi.Response{
			Message: fmt.Sprintf("template %q already exists", createTemplate.Name),
			Errors: []httpapi.Error{{
				Field:  "name",
				Detail: "this value is already in use and should be unique",
			}},
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get template by name: %s", err),
		})
		return
	}
	templateVersion, err := api.Database.GetTemplateVersionByID(r.Context(), createTemplate.VersionID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: "template version does not exist",
		})
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get template version by id: %s", err),
		})
		return
	}
	importJob, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get import job by id: %s", err),
		})
		return
	}

	var template codersdk.Template
	err = api.Database.InTx(func(db database.Store) error {
		now := database.Now()
		dbTemplate, err := db.InsertTemplate(r.Context(), database.InsertTemplateParams{
			ID:              uuid.New(),
			CreatedAt:       now,
			UpdatedAt:       now,
			OrganizationID:  organization.ID,
			Name:            createTemplate.Name,
			Provisioner:     importJob.Provisioner,
			ActiveVersionID: templateVersion.ID,
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
				ScopeID:           dbTemplate.ID,
				SourceScheme:      parameterValue.SourceScheme,
				SourceValue:       parameterValue.SourceValue,
				DestinationScheme: parameterValue.DestinationScheme,
			})
			if err != nil {
				return xerrors.Errorf("insert parameter value: %w", err)
			}
		}

		template = convertTemplate(dbTemplate, 0)
		return nil
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusCreated, template)
}

func (api *api) templatesByOrganization(rw http.ResponseWriter, r *http.Request) {
	organization := httpmw.OrganizationParam(r)
	templates, err := api.Database.GetTemplatesByOrganization(r.Context(), database.GetTemplatesByOrganizationParams{
		OrganizationID: organization.ID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get templates: %s", err.Error()),
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
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace counts: %s", err.Error()),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertTemplates(templates, workspaceCounts))
}

func (api *api) templateByOrganizationAndName(rw http.ResponseWriter, r *http.Request) {
	organization := httpmw.OrganizationParam(r)
	templateName := chi.URLParam(r, "templatename")
	template, err := api.Database.GetTemplateByOrganizationAndName(r.Context(), database.GetTemplateByOrganizationAndNameParams{
		OrganizationID: organization.ID,
		Name:           templateName,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
				Message: fmt.Sprintf("no template found by name %q in the %q organization", templateName, organization.Name),
			})
			return
		}

		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get template by organization and name: %s", err),
		})
		return
	}

	workspaceCounts, err := api.Database.GetWorkspaceOwnerCountsByTemplateIDs(r.Context(), []uuid.UUID{template.ID})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace counts: %s", err.Error()),
		})
		return
	}

	count := uint32(0)
	if len(workspaceCounts) > 0 {
		count = uint32(workspaceCounts[0].Count)
	}

	httpapi.Write(rw, http.StatusOK, convertTemplate(template, count))
}

func (api *api) workspacesByOrganization(rw http.ResponseWriter, r *http.Request) {
	organization := httpmw.OrganizationParam(r)
	workspaces, err := api.Database.GetWorkspacesByOrganizationID(r.Context(), database.GetWorkspacesByOrganizationIDParams{
		OrganizationID: organization.ID,
		Deleted:        false,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspaces: %s", err),
		})
		return
	}
	apiWorkspaces, err := convertWorkspaces(r.Context(), api.Database, workspaces)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("convert workspaces: %s", err),
		})
		return
	}
	httpapi.Write(rw, http.StatusOK, apiWorkspaces)
}

func (api *api) workspacesByOwner(rw http.ResponseWriter, r *http.Request) {
	owner := httpmw.UserParam(r)
	workspaces, err := api.Database.GetWorkspacesByOwnerID(r.Context(), database.GetWorkspacesByOwnerIDParams{
		OwnerID: owner.ID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspaces: %s", err),
		})
		return
	}
	apiWorkspaces, err := convertWorkspaces(r.Context(), api.Database, workspaces)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("convert workspaces: %s", err),
		})
		return
	}
	httpapi.Write(rw, http.StatusOK, apiWorkspaces)
}

func (api *api) workspaceByOwnerAndName(rw http.ResponseWriter, r *http.Request) {
	owner := httpmw.UserParam(r)
	organization := httpmw.OrganizationParam(r)
	workspaceName := chi.URLParam(r, "workspace")

	workspace, err := api.Database.GetWorkspaceByOwnerIDAndName(r.Context(), database.GetWorkspaceByOwnerIDAndNameParams{
		OwnerID: owner.ID,
		Name:    workspaceName,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: fmt.Sprintf("no workspace found by name %q", workspaceName),
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace by name: %s", err),
		})
		return
	}

	if workspace.OrganizationID != organization.ID {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: fmt.Sprintf("workspace is not owned by organization %q", organization.Name),
		})
		return
	}

	build, err := api.Database.GetWorkspaceBuildByWorkspaceIDWithoutAfter(r.Context(), workspace.ID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace build: %s", err),
		})
		return
	}
	job, err := api.Database.GetProvisionerJobByID(r.Context(), build.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	template, err := api.Database.GetTemplateByID(r.Context(), workspace.TemplateID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get template: %s", err),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertWorkspace(workspace,
		convertWorkspaceBuild(build, convertProvisionerJob(job)), template))
}

// Create a new workspace for the currently authenticated user.
func (api *api) postWorkspacesByOrganization(rw http.ResponseWriter, r *http.Request) {
	var createWorkspace codersdk.CreateWorkspaceRequest
	if !httpapi.Read(rw, r, &createWorkspace) {
		return
	}
	apiKey := httpmw.APIKey(r)
	template, err := api.Database.GetTemplateByID(r.Context(), createWorkspace.TemplateID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("template %q doesn't exist", createWorkspace.TemplateID.String()),
			Errors: []httpapi.Error{{
				Field:  "template_id",
				Detail: "template not found",
			}},
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get template: %s", err),
		})
		return
	}
	organization := httpmw.OrganizationParam(r)
	if organization.ID != template.OrganizationID {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: fmt.Sprintf("template is not in organization %q", organization.Name),
		})
		return
	}
	_, err = api.Database.GetOrganizationMemberByUserID(r.Context(), database.GetOrganizationMemberByUserIDParams{
		OrganizationID: template.OrganizationID,
		UserID:         apiKey.UserID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: "you aren't allowed to access templates in that organization",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get organization member: %s", err),
		})
		return
	}

	workspace, err := api.Database.GetWorkspaceByOwnerIDAndName(r.Context(), database.GetWorkspaceByOwnerIDAndNameParams{
		OwnerID: apiKey.UserID,
		Name:    createWorkspace.Name,
	})
	if err == nil {
		// If the workspace already exists, don't allow creation.
		template, err := api.Database.GetTemplateByID(r.Context(), workspace.TemplateID)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("find template for conflicting workspace name %q: %s", createWorkspace.Name, err),
			})
			return
		}
		// The template is fetched for clarity to the user on where the conflicting name may be.
		httpapi.Write(rw, http.StatusConflict, httpapi.Response{
			Message: fmt.Sprintf("workspace %q already exists in the %q template", createWorkspace.Name, template.Name),
			Errors: []httpapi.Error{{
				Field:  "name",
				Detail: "this value is already in use and should be unique",
			}},
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace by name: %s", err.Error()),
		})
		return
	}

	templateVersion, err := api.Database.GetTemplateVersionByID(r.Context(), template.ActiveVersionID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get template version: %s", err),
		})
		return
	}
	templateVersionJob, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get template version job: %s", err),
		})
		return
	}
	templateVersionJobStatus := convertProvisionerJob(templateVersionJob).Status
	switch templateVersionJobStatus {
	case codersdk.ProvisionerJobPending, codersdk.ProvisionerJobRunning:
		httpapi.Write(rw, http.StatusNotAcceptable, httpapi.Response{
			Message: fmt.Sprintf("The provided template version is %s. Wait for it to complete importing!", templateVersionJobStatus),
		})
		return
	case codersdk.ProvisionerJobFailed:
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: fmt.Sprintf("The provided template version %q has failed to import. You cannot create workspaces using it!", templateVersion.Name),
		})
		return
	case codersdk.ProvisionerJobCanceled:
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: "The provided template version was canceled during import. You cannot create workspaces using it!",
		})
		return
	}

	var provisionerJob database.ProvisionerJob
	var workspaceBuild database.WorkspaceBuild
	err = api.Database.InTx(func(db database.Store) error {
		workspaceBuildID := uuid.New()
		// Workspaces are created without any versions.
		workspace, err = db.InsertWorkspace(r.Context(), database.InsertWorkspaceParams{
			ID:             uuid.New(),
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			OwnerID:        apiKey.UserID,
			OrganizationID: template.OrganizationID,
			TemplateID:     template.ID,
			Name:           createWorkspace.Name,
		})
		if err != nil {
			return xerrors.Errorf("insert workspace: %w", err)
		}
		for _, parameterValue := range createWorkspace.ParameterValues {
			_, err = db.InsertParameterValue(r.Context(), database.InsertParameterValueParams{
				ID:                uuid.New(),
				Name:              parameterValue.Name,
				CreatedAt:         database.Now(),
				UpdatedAt:         database.Now(),
				Scope:             database.ParameterScopeWorkspace,
				ScopeID:           workspace.ID,
				SourceScheme:      parameterValue.SourceScheme,
				SourceValue:       parameterValue.SourceValue,
				DestinationScheme: parameterValue.DestinationScheme,
			})
			if err != nil {
				return xerrors.Errorf("insert parameter value: %w", err)
			}
		}

		input, err := json.Marshal(workspaceProvisionJob{
			WorkspaceBuildID: workspaceBuildID,
		})
		if err != nil {
			return xerrors.Errorf("marshal provision job: %w", err)
		}
		provisionerJob, err = db.InsertProvisionerJob(r.Context(), database.InsertProvisionerJobParams{
			ID:             uuid.New(),
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			InitiatorID:    apiKey.UserID,
			OrganizationID: template.OrganizationID,
			Provisioner:    template.Provisioner,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			StorageMethod:  templateVersionJob.StorageMethod,
			StorageSource:  templateVersionJob.StorageSource,
			Input:          input,
		})
		if err != nil {
			return xerrors.Errorf("insert provisioner job: %w", err)
		}
		workspaceBuild, err = db.InsertWorkspaceBuild(r.Context(), database.InsertWorkspaceBuildParams{
			ID:                workspaceBuildID,
			CreatedAt:         database.Now(),
			UpdatedAt:         database.Now(),
			WorkspaceID:       workspace.ID,
			TemplateVersionID: templateVersion.ID,
			Name:              namesgenerator.GetRandomName(1),
			InitiatorID:       apiKey.UserID,
			Transition:        database.WorkspaceTransitionStart,
			JobID:             provisionerJob.ID,
		})
		if err != nil {
			return xerrors.Errorf("insert workspace build: %w", err)
		}
		return nil
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("create workspace: %s", err),
		})
		return
	}

	httpapi.Write(rw, http.StatusCreated, convertWorkspace(workspace,
		convertWorkspaceBuild(workspaceBuild, convertProvisionerJob(templateVersionJob)), template))
}

// convertOrganization consumes the database representation and outputs an API friendly representation.
func convertOrganization(organization database.Organization) codersdk.Organization {
	return codersdk.Organization{
		ID:        organization.ID,
		Name:      organization.Name,
		CreatedAt: organization.CreatedAt,
		UpdatedAt: organization.UpdatedAt,
	}
}
