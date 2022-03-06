package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/render"
	"github.com/google/uuid"

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
)

// ProjectVersion represents a single version of a project.
type ProjectVersion struct {
	ID        uuid.UUID  `json:"id"`
	ProjectID *uuid.UUID `json:"project_id,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Name      string     `json:"name"`
	JobID     uuid.UUID  `json:"import_job_id"`
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

func convertProjectVersion(version database.ProjectVersion) ProjectVersion {
	return ProjectVersion{
		ID:        version.ID,
		ProjectID: &version.ProjectID.UUID,
		CreatedAt: version.CreatedAt,
		UpdatedAt: version.UpdatedAt,
		Name:      version.Name,
		JobID:     version.JobID,
	}
}
