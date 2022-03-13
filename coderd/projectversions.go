package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/render"

	"github.com/coder/coder/coderd/parameter"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
)

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

func (api *api) patchCancelProjectVersion(rw http.ResponseWriter, r *http.Request) {
	projectVersion := httpmw.ProjectVersionParam(r)
	job, err := api.Database.GetProvisionerJobByID(r.Context(), projectVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	if job.CompletedAt.Valid {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: "Job has already completed!",
		})
		return
	}
	if job.CanceledAt.Valid {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: "Job has already been marked as canceled!",
		})
		return
	}
	err = api.Database.UpdateProvisionerJobWithCancelByID(r.Context(), database.UpdateProvisionerJobWithCancelByIDParams{
		ID: job.ID,
		CanceledAt: sql.NullTime{
			Time:  database.Now(),
			Valid: true,
		},
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("update provisioner job: %s", err),
		})
		return
	}
	httpapi.Write(rw, http.StatusOK, httpapi.Response{
		Message: "Job has been marked as canceled...",
	})
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

func convertProjectVersion(version database.ProjectVersion, job codersdk.ProvisionerJob) codersdk.ProjectVersion {
	return codersdk.ProjectVersion{
		ID:        version.ID,
		ProjectID: &version.ProjectID.UUID,
		CreatedAt: version.CreatedAt,
		UpdatedAt: version.UpdatedAt,
		Name:      version.Name,
		Job:       job,
	}
}
