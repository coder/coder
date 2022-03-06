package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/render"
	"github.com/google/uuid"

	"github.com/coder/coder/coderd/parameter"
	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
)

// ProjectVersion represents a single version of a project.
type ProjectVersion struct {
	ID        uuid.UUID      `json:"id"`
	ProjectID *uuid.UUID     `json:"project_id,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Name      string         `json:"name"`
	Job       ProvisionerJob `json:"job"`
}

// ProjectVersionParameterSchema represents a parameter parsed from project version source.
type ProjectVersionParameterSchema database.ParameterSchema

// ProjectVersionParameter represents a computed parameter value.
type ProjectVersionParameter parameter.ComputedValue

func (api *api) projectVersion(rw http.ResponseWriter, r *http.Request) {
	projectVersion := httpmw.ProjectVersionParam(r)
	job, err := api.Database.GetProvisionerJobByID(r.Context(), projectVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertProjectVersion(projectVersion, convertProvisionerJob(job)))
}

func (api *api) projectVersionSchema(rw http.ResponseWriter, r *http.Request) {
	projectVersion := httpmw.ProjectVersionParam(r)
	job, err := api.Database.GetProvisionerJobByID(r.Context(), projectVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	if !job.CompletedAt.Valid {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: "Project version job hasn't completed!",
		})
		return
	}
	schemas, err := api.Database.GetParameterSchemasByJobID(r.Context(), job.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("list parameter schemas: %s", err),
		})
		return
	}
	if schemas == nil {
		schemas = []database.ParameterSchema{}
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, schemas)
}

func (api *api) projectVersionParameters(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	projectVersion := httpmw.ProjectVersionParam(r)
	job, err := api.Database.GetProvisionerJobByID(r.Context(), projectVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	if !job.CompletedAt.Valid {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: "Job hasn't completed!",
		})
		return
	}
	values, err := parameter.Compute(r.Context(), api.Database, parameter.ComputeScope{
		ProjectImportJobID: job.ID,
		OrganizationID:     job.OrganizationID,
		UserID:             apiKey.UserID,
	}, &parameter.ComputeOptions{
		// We *never* want to send the client secret parameter values.
		HideRedisplayValues: true,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("compute values: %s", err),
		})
		return
	}
	if values == nil {
		values = []parameter.ComputedValue{}
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, values)
}

func (api *api) projectVersionResources(rw http.ResponseWriter, r *http.Request) {
	projectVersion := httpmw.ProjectVersionParam(r)
	job, err := api.Database.GetProvisionerJobByID(r.Context(), projectVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	api.provisionerJobResources(rw, r, job)
}

func (api *api) projectVersionLogs(rw http.ResponseWriter, r *http.Request) {
	projectVersion := httpmw.ProjectVersionParam(r)
	job, err := api.Database.GetProvisionerJobByID(r.Context(), projectVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	api.provisionerJobLogs(rw, r, job)
}

func convertProjectVersion(version database.ProjectVersion, job ProvisionerJob) ProjectVersion {
	return ProjectVersion{
		ID:        version.ID,
		ProjectID: &version.ProjectID.UUID,
		CreatedAt: version.CreatedAt,
		UpdatedAt: version.UpdatedAt,
		Name:      version.Name,
		Job:       job,
	}
}
