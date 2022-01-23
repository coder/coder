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

type CreateProjectRequest struct {
	Name        string                   `json:"name" validate:"username,required"`
	Provisioner database.ProvisionerType `json:"provisioner" validate:"oneof=terraform cdr-basic,required"`
}

type projects struct {
	Database database.Store
}

// allProjects lists all projects across organizations for a user.
func (p *projects) allProjects(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	organizations, err := p.Database.GetOrganizationsByUserID(r.Context(), apiKey.UserID)
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
	projects, err := p.Database.GetProjectsByOrganizationIDs(r.Context(), organizationIDs)
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

// allProjectsForOrganization lists all projects for a specific organization.
func (p *projects) allProjectsForOrganization(rw http.ResponseWriter, r *http.Request) {
	organization := httpmw.OrganizationParam(r)
	projects, err := p.Database.GetProjectsByOrganizationIDs(r.Context(), []string{organization.ID})
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

// createProject makes a new project in an organization.
func (p *projects) createProject(rw http.ResponseWriter, r *http.Request) {
	var createProject CreateProjectRequest
	if !httpapi.Read(rw, r, &createProject) {
		return
	}
	organization := httpmw.OrganizationParam(r)
	_, err := p.Database.GetProjectByOrganizationAndName(r.Context(), database.GetProjectByOrganizationAndNameParams{
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

	project, err := p.Database.InsertProject(r.Context(), database.InsertProjectParams{
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

// project returns a single project parsed from the URL path.
func (*projects) project(rw http.ResponseWriter, r *http.Request) {
	project := httpmw.ProjectParam(r)

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, project)
}
