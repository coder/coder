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

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
)

// ProjectVersion represents a single version of a project.
type ProjectVersion struct {
	ID          uuid.UUID `json:"id"`
	ProjectID   uuid.UUID `json:"project_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Name        string    `json:"name"`
	ImportJobID uuid.UUID `json:"import_job_id"`
}

// CreateProjectVersionRequest enables callers to create a new Project Version.
type CreateProjectVersionRequest struct {
	ImportJobID uuid.UUID `json:"import_job_id" validate:"required"`
}

// Lists versions for a single project.
func (api *api) projectVersionsByOrganization(rw http.ResponseWriter, r *http.Request) {
	project := httpmw.ProjectParam(r)

	version, err := api.Database.GetProjectVersionsByProjectID(r.Context(), project.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project version: %s", err),
		})
		return
	}
	apiVersion := make([]ProjectVersion, 0)
	for _, version := range version {
		apiVersion = append(apiVersion, convertProjectVersion(version))
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, apiVersion)
}

// Return a single project version by organization and name.
func (*api) projectVersionByOrganizationAndName(rw http.ResponseWriter, r *http.Request) {
	projectVersion := httpmw.ProjectVersionParam(r)
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertProjectVersion(projectVersion))
}

// Creates a new version of the project. An import job is queued to parse
// the storage method provided. Once completed, the import job will specify
// the version as latest.
func (api *api) postProjectVersionByOrganization(rw http.ResponseWriter, r *http.Request) {
	var createProjectVersion CreateProjectVersionRequest
	if !httpapi.Read(rw, r, &createProjectVersion) {
		return
	}
	job, err := api.Database.GetProvisionerJobByID(r.Context(), createProjectVersion.ImportJobID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: "job not found",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	project := httpmw.ProjectParam(r)
	projectVersion, err := api.Database.InsertProjectVersion(r.Context(), database.InsertProjectVersionParams{
		ID:          uuid.New(),
		ProjectID:   project.ID,
		CreatedAt:   database.Now(),
		UpdatedAt:   database.Now(),
		Name:        namesgenerator.GetRandomName(1),
		ImportJobID: job.ID,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("insert project version: %s", err),
		})
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, convertProjectVersion(projectVersion))
}

func convertProjectVersion(version database.ProjectVersion) ProjectVersion {
	return ProjectVersion{
		ID:          version.ID,
		ProjectID:   version.ProjectID,
		CreatedAt:   version.CreatedAt,
		UpdatedAt:   version.UpdatedAt,
		Name:        version.Name,
		ImportJobID: version.ImportJobID,
	}
}
