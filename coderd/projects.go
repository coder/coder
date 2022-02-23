package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/render"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
)

// ParameterValue represents a set value for the scope.
type ParameterValue database.ParameterValue

// CreateParameterValueRequest is used to create a new parameter value for a scope.
type CreateParameterValueRequest struct {
	Name              string                              `json:"name" validate:"required"`
	SourceValue       string                              `json:"source_value" validate:"required"`
	SourceScheme      database.ParameterSourceScheme      `json:"source_scheme" validate:"oneof=data,required"`
	DestinationScheme database.ParameterDestinationScheme `json:"destination_scheme" validate:"oneof=environment_variable provisioner_variable,required"`
}

// Project is the JSON representation of a Coder project.
// This type matches the database object for now, but is
// abstracted for ease of change later on.
type Project struct {
	ID                  uuid.UUID                `json:"id"`
	CreatedAt           time.Time                `json:"created_at"`
	UpdatedAt           time.Time                `json:"updated_at"`
	OrganizationID      string                   `json:"organization_id"`
	Name                string                   `json:"name"`
	Provisioner         database.ProvisionerType `json:"provisioner"`
	ActiveVersionID     uuid.UUID                `json:"active_version_id"`
	WorkspaceOwnerCount uint32                   `json:"workspace_owner_count"`
}

// CreateProjectRequest enables callers to create a new Project.
type CreateProjectRequest struct {
	Name string `json:"name" validate:"username,required"`

	// VersionImportJobID is an in-progress or completed job to use as
	// an initial version of the project.
	//
	// This is required on creation to enable a user-flow of validating
	// the project works. There is no reason the data-model cannot support
	// empty projects, but it doesn't make sense for users.
	VersionImportJobID uuid.UUID `json:"import_job_id" validate:"required"`
}

// Lists all projects the authenticated user has access to.
func (api *api) projects(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	organizations, err := api.Database.GetOrganizationsByUserID(r.Context(), apiKey.UserID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get organizations: %s", err.Error()),
		})
		return
	}
	organizationIDs := make([]string, 0, len(organizations))
	for _, organization := range organizations {
		organizationIDs = append(organizationIDs, organization.ID)
	}
	projects, err := api.Database.GetProjectsByOrganizationIDs(r.Context(), organizationIDs)
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

// Lists all projects in an organization.
func (api *api) projectsByOrganization(rw http.ResponseWriter, r *http.Request) {
	organization := httpmw.OrganizationParam(r)
	projects, err := api.Database.GetProjectsByOrganizationIDs(r.Context(), []string{organization.ID})
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
	importJob, err := api.Database.GetProvisionerJobByID(r.Context(), createProject.VersionImportJobID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: "import job does not exist",
		})
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get import job by id: %s", err),
		})
		return
	}

	var project Project
	err = api.Database.InTx(func(db database.Store) error {
		projectVersionID := uuid.New()
		dbProject, err := db.InsertProject(r.Context(), database.InsertProjectParams{
			ID:              uuid.New(),
			CreatedAt:       database.Now(),
			UpdatedAt:       database.Now(),
			OrganizationID:  organization.ID,
			Name:            createProject.Name,
			Provisioner:     importJob.Provisioner,
			ActiveVersionID: projectVersionID,
		})
		if err != nil {
			return xerrors.Errorf("insert project: %s", err)
		}
		_, err = db.InsertProjectVersion(r.Context(), database.InsertProjectVersionParams{
			ID:          projectVersionID,
			ProjectID:   dbProject.ID,
			CreatedAt:   database.Now(),
			UpdatedAt:   database.Now(),
			Name:        namesgenerator.GetRandomName(1),
			ImportJobID: importJob.ID,
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

// Returns a single project.
func (*api) projectByOrganization(rw http.ResponseWriter, r *http.Request) {
	project := httpmw.ProjectParam(r)

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, project)
}

// Creates parameters for a project.
// This should validate the calling user has permissions!
func (api *api) postParametersByProject(rw http.ResponseWriter, r *http.Request) {
	project := httpmw.ProjectParam(r)
	var createRequest CreateParameterValueRequest
	if !httpapi.Read(rw, r, &createRequest) {
		return
	}
	parameterValue, err := api.Database.InsertParameterValue(r.Context(), database.InsertParameterValueParams{
		ID:                uuid.New(),
		Name:              createRequest.Name,
		CreatedAt:         database.Now(),
		UpdatedAt:         database.Now(),
		Scope:             database.ParameterScopeProject,
		ScopeID:           project.ID.String(),
		SourceScheme:      createRequest.SourceScheme,
		SourceValue:       createRequest.SourceValue,
		DestinationScheme: createRequest.DestinationScheme,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("insert parameter value: %s", err),
		})
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, parameterValue)
}

// Lists parameters for a project.
func (api *api) parametersByProject(rw http.ResponseWriter, r *http.Request) {
	project := httpmw.ProjectParam(r)
	parameterValues, err := api.Database.GetParameterValuesByScope(r.Context(), database.GetParameterValuesByScopeParams{
		Scope:   database.ParameterScopeProject,
		ScopeID: project.ID.String(),
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
		parameterValues = []database.ParameterValue{}
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get parameter values: %s", err),
		})
		return
	}

	apiParameterValues := make([]ParameterValue, 0, len(parameterValues))
	for _, parameterValue := range parameterValues {
		apiParameterValues = append(apiParameterValues, convertParameterValue(parameterValue))
	}

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, apiParameterValues)
}

func convertProjects(projects []database.Project, workspaceCounts []database.GetWorkspaceOwnerCountsByProjectIDsRow) []Project {
	apiProjects := make([]Project, 0, len(projects))
	for _, project := range projects {
		found := false
		for _, workspaceCount := range workspaceCounts {
			if workspaceCount.ProjectID.String() != project.ID.String() {
				continue
			}
			apiProjects = append(apiProjects, convertProject(project, uint32(workspaceCount.Count)))
			found = true
			break
		}
		if !found {
			apiProjects = append(apiProjects, convertProject(project, uint32(0)))
		}
	}
	return apiProjects
}

func convertProject(project database.Project, workspaceOwnerCount uint32) Project {
	return Project{
		ID:                  project.ID,
		CreatedAt:           project.CreatedAt,
		UpdatedAt:           project.UpdatedAt,
		OrganizationID:      project.OrganizationID,
		Name:                project.Name,
		Provisioner:         project.Provisioner,
		ActiveVersionID:     project.ActiveVersionID,
		WorkspaceOwnerCount: workspaceOwnerCount,
	}
}

func convertParameterValue(parameterValue database.ParameterValue) ParameterValue {
	parameterValue.SourceValue = ""
	return ParameterValue(parameterValue)
}
