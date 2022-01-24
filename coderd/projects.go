package coderd

import (
	"archive/tar"
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/render"
	"github.com/google/uuid"

	"github.com/moby/moby/pkg/namesgenerator"

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
)

// Project is the JSON representation of a Coder project.
// This type matches the database object for now, but is
// abstracted for ease of change later on.
type Project database.Project

// ProjectVersion is the JSON representation of a Coder project version.
type ProjectVersion struct {
	ID            uuid.UUID                     `json:"id"`
	ProjectID     uuid.UUID                     `json:"project_id"`
	CreatedAt     time.Time                     `json:"created_at"`
	UpdatedAt     time.Time                     `json:"updated_at"`
	Name          string                        `json:"name"`
	StorageMethod database.ProjectStorageMethod `json:"storage_method"`
}

// CreateProjectRequest enables callers to create a new Project.
type CreateProjectRequest struct {
	Name        string                   `json:"name" validate:"username,required"`
	Provisioner database.ProvisionerType `json:"provisioner" validate:"oneof=terraform cdr-basic,required"`
}

// CreateProjectVersionRequest enables callers to create a new Project Version.
type CreateProjectVersionRequest struct {
	Name          string                        `json:"name,omitempty" validate:"username"`
	StorageMethod database.ProjectStorageMethod `json:"storage_method" validate:"oneof=inline-archive,required"`
	StorageSource []byte                        `json:"storage_source" validate:"max=1048576,required"`
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

// projectVersions lists versions for a single project.
func (p *projects) projectVersions(rw http.ResponseWriter, r *http.Request) {
	project := httpmw.ProjectParam(r)

	history, err := p.Database.GetProjectHistoryByProjectID(r.Context(), project.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project history: %s", err),
		})
		return
	}
	versions := make([]ProjectVersion, 0)
	for _, version := range history {
		versions = append(versions, convertProjectHistory(version))
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, versions)
}

func (p *projects) createProjectVersion(rw http.ResponseWriter, r *http.Request) {
	var createProjectVersion CreateProjectVersionRequest
	if !httpapi.Read(rw, r, &createProjectVersion) {
		return
	}

	switch createProjectVersion.StorageMethod {
	case database.ProjectStorageMethodInlineArchive:
		tarReader := tar.NewReader(bytes.NewReader(createProjectVersion.StorageSource))
		_, err := tarReader.Next()
		if err != nil {
			httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
				Message: "the archive must be a tar",
			})
			return
		}
	default:
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("unsupported storage method %s", createProjectVersion.StorageMethod),
		})
		return
	}

	project := httpmw.ProjectParam(r)
	history, err := p.Database.InsertProjectHistory(r.Context(), database.InsertProjectHistoryParams{
		ID:            uuid.New(),
		ProjectID:     project.ID,
		CreatedAt:     database.Now(),
		UpdatedAt:     database.Now(),
		Name:          namesgenerator.GetRandomName(1),
		StorageMethod: createProjectVersion.StorageMethod,
		StorageSource: createProjectVersion.StorageSource,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("insert project history: %s", err),
		})
		return
	}

	// TODO: A job to process the new version should occur here.

	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, convertProjectHistory(history))
}

func convertProjectHistory(history database.ProjectHistory) ProjectVersion {
	return ProjectVersion{
		ID:        history.ID,
		ProjectID: history.ProjectID,
		CreatedAt: history.CreatedAt,
		UpdatedAt: history.UpdatedAt,
		Name:      history.Name,
	}
}
