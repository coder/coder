package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/render"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/parameter"
	"github.com/coder/coder/codersdk"
)

func (api *api) templateVersion(rw http.ResponseWriter, r *http.Request) {
	templateVersion := httpmw.TemplateVersionParam(r)
	job, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertTemplateVersion(templateVersion, convertProvisionerJob(job)))
}

func (api *api) patchCancelTemplateVersion(rw http.ResponseWriter, r *http.Request) {
	templateVersion := httpmw.TemplateVersionParam(r)
	job, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
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

func (api *api) templateVersionSchema(rw http.ResponseWriter, r *http.Request) {
	templateVersion := httpmw.TemplateVersionParam(r)
	job, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	if !job.CompletedAt.Valid {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: "Template version job hasn't completed!",
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

func (api *api) templateVersionParameters(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	templateVersion := httpmw.TemplateVersionParam(r)
	job, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
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
		TemplateImportJobID: job.ID,
		OrganizationID:      job.OrganizationID,
		UserID:              apiKey.UserID,
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

func (api *api) templateVersionResources(rw http.ResponseWriter, r *http.Request) {
	templateVersion := httpmw.TemplateVersionParam(r)
	job, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	api.provisionerJobResources(rw, r, job)
}

func (api *api) templateVersionLogs(rw http.ResponseWriter, r *http.Request) {
	templateVersion := httpmw.TemplateVersionParam(r)
	job, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	api.provisionerJobLogs(rw, r, job)
}

func convertTemplateVersion(version database.TemplateVersion, job codersdk.ProvisionerJob) codersdk.TemplateVersion {
	return codersdk.TemplateVersion{
		ID:         version.ID,
		TemplateID: &version.TemplateID.UUID,
		CreatedAt:  version.CreatedAt,
		UpdatedAt:  version.UpdatedAt,
		Name:       version.Name,
		Job:        job,
	}
}
