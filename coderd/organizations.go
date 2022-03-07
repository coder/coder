package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
)

// Organization is the JSON representation of a Coder organization.
type Organization struct {
	ID        string    `json:"id" validate:"required"`
	Name      string    `json:"name" validate:"required"`
	CreatedAt time.Time `json:"created_at" validate:"required"`
	UpdatedAt time.Time `json:"updated_at" validate:"required"`
}

// CreateProjectVersionRequest enables callers to create a new Project Version.
type CreateProjectVersionRequest struct {
	// ProjectID optionally associates a version with a project.
	ProjectID *uuid.UUID `json:"project_id"`

	StorageMethod database.ProvisionerStorageMethod `json:"storage_method" validate:"oneof=file,required"`
	StorageSource string                            `json:"storage_source" validate:"required"`
	Provisioner   database.ProvisionerType          `json:"provisioner" validate:"oneof=terraform echo,required"`
	// ParameterValues allows for additional parameters to be provided
	// during the dry-run provision stage.
	ParameterValues []CreateParameterRequest `json:"parameter_values"`
}

// CreateProjectRequest provides options when creating a project.
type CreateProjectRequest struct {
	Name string `json:"name" validate:"username,required"`

	// VersionID is an in-progress or completed job to use as
	// an initial version of the project.
	//
	// This is required on creation to enable a user-flow of validating a
	// project works. There is no reason the data-model cannot support
	// empty projects, but it doesn't make sense for users.
	VersionID uuid.UUID `json:"project_version_id" validate:"required"`
}

func (*api) organization(rw http.ResponseWriter, r *http.Request) {
	organization := httpmw.OrganizationParam(r)
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertOrganization(organization))
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
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, daemons)
}

// Creates a new version of a project. An import job is queued to parse the storage method provided.
func (api *api) postProjectVersionsByOrganization(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	organization := httpmw.OrganizationParam(r)
	var req CreateProjectVersionRequest
	if !httpapi.Read(rw, r, &req) {
		return
	}
	if req.ProjectID != nil {
		_, err := api.Database.GetProjectByID(r.Context(), *req.ProjectID)
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
				Message: "project does not exist",
			})
			return
		}
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("get project: %s", err),
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

	var projectVersion database.ProjectVersion
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
				ScopeID:           jobID.String(),
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
			Type:           database.ProvisionerJobTypeProjectVersionImport,
			Input:          []byte{'{', '}'},
		})
		if err != nil {
			return xerrors.Errorf("insert provisioner job: %w", err)
		}

		var projectID uuid.NullUUID
		if req.ProjectID != nil {
			projectID = uuid.NullUUID{
				UUID:  *req.ProjectID,
				Valid: true,
			}
		}

		projectVersion, err = api.Database.InsertProjectVersion(r.Context(), database.InsertProjectVersionParams{
			ID:             uuid.New(),
			ProjectID:      projectID,
			OrganizationID: organization.ID,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			Name:           namesgenerator.GetRandomName(1),
			Description:    "",
			JobID:          provisionerJob.ID,
		})
		if err != nil {
			return xerrors.Errorf("insert project version: %w", err)
		}
		return nil
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: err.Error(),
		})
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, convertProjectVersion(projectVersion, convertProvisionerJob(provisionerJob)))
}

// Create a new project in an organization.
func (api *api) postProjectsByOrganization(rw http.ResponseWriter, r *http.Request) {
	var createProject CreateProjectRequest
	if !httpapi.Read(rw, r, &createProject) {
		return
	}
	organization := httpmw.OrganizationParam(r)
	_, err := api.Database.GetProjectByOrganizationAndName(r.Context(), database.GetProjectByOrganizationAndNameParams{
		OrganizationID: organization.ID,
		Name:           createProject.Name,
	})
	if err == nil {
		httpapi.Write(rw, http.StatusConflict, httpapi.Response{
			Message: fmt.Sprintf("project %q already exists", createProject.Name),
			Errors: []httpapi.Error{{
				Field: "name",
				Code:  "exists",
			}},
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project by name: %s", err),
		})
		return
	}
	projectVersion, err := api.Database.GetProjectVersionByID(r.Context(), createProject.VersionID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: "project version does not exist",
		})
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project version by id: %s", err),
		})
		return
	}
	importJob, err := api.Database.GetProvisionerJobByID(r.Context(), projectVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get import job by id: %s", err),
		})
		return
	}

	var project Project
	err = api.Database.InTx(func(db database.Store) error {
		dbProject, err := db.InsertProject(r.Context(), database.InsertProjectParams{
			ID:              uuid.New(),
			CreatedAt:       database.Now(),
			UpdatedAt:       database.Now(),
			OrganizationID:  organization.ID,
			Name:            createProject.Name,
			Provisioner:     importJob.Provisioner,
			ActiveVersionID: projectVersion.ID,
		})
		if err != nil {
			return xerrors.Errorf("insert project: %s", err)
		}
		err = db.UpdateProjectVersionByID(r.Context(), database.UpdateProjectVersionByIDParams{
			ID: projectVersion.ID,
			ProjectID: uuid.NullUUID{
				UUID:  dbProject.ID,
				Valid: true,
			},
		})
		if err != nil {
			return xerrors.Errorf("insert project version: %s", err)
		}
		project = convertProject(dbProject, 0)
		return nil
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: err.Error(),
		})
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, project)
}

func (api *api) projectsByOrganization(rw http.ResponseWriter, r *http.Request) {
	organization := httpmw.OrganizationParam(r)
	projects, err := api.Database.GetProjectsByOrganization(r.Context(), organization.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get projects: %s", err.Error()),
		})
		return
	}
	projectIDs := make([]uuid.UUID, 0, len(projects))
	for _, project := range projects {
		projectIDs = append(projectIDs, project.ID)
	}
	workspaceCounts, err := api.Database.GetWorkspaceOwnerCountsByProjectIDs(r.Context(), projectIDs)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace counts: %s", err.Error()),
		})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertProjects(projects, workspaceCounts))
}

func (api *api) projectByOrganizationAndName(rw http.ResponseWriter, r *http.Request) {
	organization := httpmw.OrganizationParam(r)
	projectName := chi.URLParam(r, "projectname")
	project, err := api.Database.GetProjectByOrganizationAndName(r.Context(), database.GetProjectByOrganizationAndNameParams{
		OrganizationID: organization.ID,
		Name:           projectName,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: fmt.Sprintf("no project found by name %q in the %q organization", projectName, organization.Name),
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project by organization and name: %s", err),
		})
		return
	}
	workspaceCounts, err := api.Database.GetWorkspaceOwnerCountsByProjectIDs(r.Context(), []uuid.UUID{project.ID})
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
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertProject(project, count))
}

// convertOrganization consumes the database representation and outputs an API friendly representation.
func convertOrganization(organization database.Organization) Organization {
	return Organization{
		ID:        organization.ID,
		Name:      organization.Name,
		CreatedAt: organization.CreatedAt,
		UpdatedAt: organization.UpdatedAt,
	}
}
