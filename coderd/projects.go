package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/render"
	"github.com/google/uuid"

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
)

// Project is the JSON representation of a Coder project.
// This type matches the database object for now, but is
// abstracted for ease of change later on.
type Project database.Project

// CreateProjectRequest enables callers to create a new Project.
type CreateProjectRequest struct {
	Name        string                   `json:"name" validate:"username,required"`
	Provisioner database.ProvisionerType `json:"provisioner" validate:"oneof=terraform echo,required"`
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
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, projects)
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
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, projects)
}

// Creates a new project in an organization.
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
			Message: fmt.Sprintf("get project by name: %s", err.Error()),
		})
		return
	}

	project, err := api.Database.InsertProject(r.Context(), database.InsertProjectParams{
		ID:             uuid.New(),
		CreatedAt:      database.Now(),
		UpdatedAt:      database.Now(),
		OrganizationID: organization.ID,
		Name:           createProject.Name,
		Provisioner:    createProject.Provisioner,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("insert project: %s", err),
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

// Returns all workspaces for a specific project.
func (api *api) workspacesByProject(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	project := httpmw.ProjectParam(r)
	workspaces, err := api.Database.GetWorkspacesByProjectAndUserID(r.Context(), database.GetWorkspacesByProjectAndUserIDParams{
		OwnerID:   apiKey.UserID,
		ProjectID: project.ID,
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

	apiWorkspaces := make([]Workspace, 0, len(workspaces))
	for _, workspace := range workspaces {
		apiWorkspaces = append(apiWorkspaces, convertWorkspace(workspace))
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, apiWorkspaces)
}

// Creates parameters for a project.
// This should validate the calling user has permissions!
func (api *api) postParametersByProject(rw http.ResponseWriter, r *http.Request) {
	project := httpmw.ProjectParam(r)

	postParameterValueForScope(rw, r, api.Database, database.ParameterScopeProject, project.ID.String())
}

// Lists parameters for a project.
func (api *api) parametersByProject(rw http.ResponseWriter, r *http.Request) {
	project := httpmw.ProjectParam(r)

	parametersForScope(rw, r, api.Database, database.GetParameterValuesByScopeParams{
		Scope:   database.ParameterScopeProject,
		ScopeID: project.ID.String(),
	})
}
