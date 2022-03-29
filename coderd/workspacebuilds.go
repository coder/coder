package coderd

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/go-chi/render"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

func (api *api) workspaceBuild(rw http.ResponseWriter, r *http.Request) {
	workspaceBuild := httpmw.WorkspaceBuildParam(r)
	job, err := api.Database.GetProvisionerJobByID(r.Context(), workspaceBuild.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertWorkspaceBuild(workspaceBuild, convertProvisionerJob(job)))
}

func (api *api) patchCancelWorkspaceBuild(rw http.ResponseWriter, r *http.Request) {
	workspaceBuild := httpmw.WorkspaceBuildParam(r)
	job, err := api.Database.GetProvisionerJobByID(r.Context(), workspaceBuild.JobID)
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

func (api *api) workspaceBuildResources(rw http.ResponseWriter, r *http.Request) {
	workspaceBuild := httpmw.WorkspaceBuildParam(r)
	job, err := api.Database.GetProvisionerJobByID(r.Context(), workspaceBuild.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	api.provisionerJobResources(rw, r, job)
}

func (api *api) workspaceBuildLogs(rw http.ResponseWriter, r *http.Request) {
	workspaceBuild := httpmw.WorkspaceBuildParam(r)
	job, err := api.Database.GetProvisionerJobByID(r.Context(), workspaceBuild.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	api.provisionerJobLogs(rw, r, job)
}

func convertWorkspaceBuild(workspaceBuild database.WorkspaceBuild, job codersdk.ProvisionerJob) codersdk.WorkspaceBuild {
	//nolint:unconvert
	return codersdk.WorkspaceBuild{
		ID:               workspaceBuild.ID,
		CreatedAt:        workspaceBuild.CreatedAt,
		UpdatedAt:        workspaceBuild.UpdatedAt,
		WorkspaceID:      workspaceBuild.WorkspaceID,
		ProjectVersionID: workspaceBuild.ProjectVersionID,
		BeforeID:         workspaceBuild.BeforeID.UUID,
		AfterID:          workspaceBuild.AfterID.UUID,
		Name:             workspaceBuild.Name,
		Transition:       workspaceBuild.Transition,
		InitiatorID:      workspaceBuild.InitiatorID,
		Job:              job,
	}
}

func convertWorkspaceResource(resource database.WorkspaceResource, agent *codersdk.WorkspaceAgent) codersdk.WorkspaceResource {
	return codersdk.WorkspaceResource{
		ID:         resource.ID,
		CreatedAt:  resource.CreatedAt,
		JobID:      resource.JobID,
		Transition: resource.Transition,
		Address:    resource.Address,
		Type:       resource.Type,
		Name:       resource.Name,
		Agent:      agent,
	}
}
