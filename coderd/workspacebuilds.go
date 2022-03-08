package coderd

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/render"
	"github.com/google/uuid"

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
)

// WorkspaceBuild is an at-point representation of a workspace state.
// Iterate on before/after to determine a chronological history.
type WorkspaceBuild struct {
	ID               uuid.UUID                    `json:"id"`
	CreatedAt        time.Time                    `json:"created_at"`
	UpdatedAt        time.Time                    `json:"updated_at"`
	WorkspaceID      uuid.UUID                    `json:"workspace_id"`
	ProjectVersionID uuid.UUID                    `json:"project_version_id"`
	BeforeID         uuid.UUID                    `json:"before_id"`
	AfterID          uuid.UUID                    `json:"after_id"`
	Name             string                       `json:"name"`
	Transition       database.WorkspaceTransition `json:"transition"`
	Initiator        string                       `json:"initiator"`
	Job              ProvisionerJob               `json:"job"`
}

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

func convertWorkspaceBuild(workspaceBuild database.WorkspaceBuild, job ProvisionerJob) WorkspaceBuild {
	//nolint:unconvert
	return WorkspaceBuild(WorkspaceBuild{
		ID:               workspaceBuild.ID,
		CreatedAt:        workspaceBuild.CreatedAt,
		UpdatedAt:        workspaceBuild.UpdatedAt,
		WorkspaceID:      workspaceBuild.WorkspaceID,
		ProjectVersionID: workspaceBuild.ProjectVersionID,
		BeforeID:         workspaceBuild.BeforeID.UUID,
		AfterID:          workspaceBuild.AfterID.UUID,
		Name:             workspaceBuild.Name,
		Transition:       workspaceBuild.Transition,
		Initiator:        workspaceBuild.Initiator,
		Job:              job,
	})
}

func convertWorkspaceResource(resource database.WorkspaceResource, agent *WorkspaceAgent) WorkspaceResource {
	return WorkspaceResource{
		ID:         resource.ID,
		CreatedAt:  resource.CreatedAt,
		JobID:      resource.JobID,
		Transition: resource.Transition,
		Type:       resource.Type,
		Name:       resource.Name,
		Agent:      agent,
	}
}
