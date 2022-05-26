package coderd

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

func (api *API) workspaceBuild(rw http.ResponseWriter, r *http.Request) {
	workspaceBuild := httpmw.WorkspaceBuildParam(r)
	workspace, err := api.Database.GetWorkspaceByID(r.Context(), workspaceBuild.WorkspaceID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "no workspace exists for this job",
		})
		return
	}

	if !api.Authorize(rw, r, rbac.ActionRead, rbac.ResourceWorkspace.
		InOrg(workspace.OrganizationID).WithOwner(workspace.OwnerID.String()).WithID(workspace.ID.String())) {
		return
	}

	job, err := api.Database.GetProvisionerJobByID(r.Context(), workspaceBuild.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertWorkspaceBuild(workspaceBuild, convertProvisionerJob(job)))
}

func (api *API) workspaceBuilds(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)

	if !api.Authorize(rw, r, rbac.ActionRead, rbac.ResourceWorkspace.
		InOrg(workspace.OrganizationID).WithOwner(workspace.OwnerID.String()).WithID(workspace.ID.String())) {
		return
	}

	paginationParams, ok := parsePagination(rw, r)
	if !ok {
		return
	}
	req := database.GetWorkspaceBuildByWorkspaceIDParams{
		WorkspaceID: workspace.ID,
		AfterID:     paginationParams.AfterID,
		OffsetOpt:   int32(paginationParams.Offset),
		LimitOpt:    int32(paginationParams.Limit),
	}
	builds, err := api.Database.GetWorkspaceBuildByWorkspaceID(r.Context(), req)
	if xerrors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace builds: %s", err),
		})
		return
	}
	jobIDs := make([]uuid.UUID, 0, len(builds))
	for _, version := range builds {
		jobIDs = append(jobIDs, version.JobID)
	}
	jobs, err := api.Database.GetProvisionerJobsByIDs(r.Context(), jobIDs)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get jobs: %s", err),
		})
		return
	}
	jobByID := map[string]database.ProvisionerJob{}
	for _, job := range jobs {
		jobByID[job.ID.String()] = job
	}

	apiBuilds := make([]codersdk.WorkspaceBuild, 0)
	for _, build := range builds {
		job, exists := jobByID[build.JobID.String()]
		if !exists {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("job %q doesn't exist for build %q", build.JobID, build.ID),
			})
			return
		}
		apiBuilds = append(apiBuilds, convertWorkspaceBuild(build, convertProvisionerJob(job)))
	}

	httpapi.Write(rw, http.StatusOK, apiBuilds)
}

func (api *API) workspaceBuildByName(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(rw, r, rbac.ActionRead, rbac.ResourceWorkspace.
		InOrg(workspace.OrganizationID).WithOwner(workspace.OwnerID.String()).WithID(workspace.ID.String())) {
		return
	}

	workspaceBuildName := chi.URLParam(r, "workspacebuildname")
	workspaceBuild, err := api.Database.GetWorkspaceBuildByWorkspaceIDAndName(r.Context(), database.GetWorkspaceBuildByWorkspaceIDAndNameParams{
		WorkspaceID: workspace.ID,
		Name:        workspaceBuildName,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: fmt.Sprintf("no workspace build found by name %q", workspaceBuildName),
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace build by name: %s", err),
		})
		return
	}
	job, err := api.Database.GetProvisionerJobByID(r.Context(), workspaceBuild.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertWorkspaceBuild(workspaceBuild, convertProvisionerJob(job)))
}

func (api *API) postWorkspaceBuilds(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	workspace := httpmw.WorkspaceParam(r)
	var createBuild codersdk.CreateWorkspaceBuildRequest
	if !httpapi.Read(rw, r, &createBuild) {
		return
	}

	// Rbac action depends on the transition
	var action rbac.Action
	switch createBuild.Transition {
	case codersdk.WorkspaceTransitionDelete:
		action = rbac.ActionDelete
	case codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop:
		action = rbac.ActionUpdate
	default:
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("transition not supported: %q", createBuild.Transition),
		})
		return
	}
	if !api.Authorize(rw, r, action, rbac.ResourceWorkspace.
		InOrg(workspace.OrganizationID).WithOwner(workspace.OwnerID.String()).WithID(workspace.ID.String())) {
		return
	}

	if createBuild.TemplateVersionID == uuid.Nil {
		latestBuild, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(r.Context(), workspace.ID)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("get latest workspace build: %s", err),
			})
			return
		}
		createBuild.TemplateVersionID = latestBuild.TemplateVersionID
	}
	templateVersion, err := api.Database.GetTemplateVersionByID(r.Context(), createBuild.TemplateVersionID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "template version not found",
			Errors: []httpapi.Error{{
				Field:  "template_version_id",
				Detail: "template version not found",
			}},
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get template version: %s", err),
		})
		return
	}
	templateVersionJob, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	templateVersionJobStatus := convertProvisionerJob(templateVersionJob).Status
	switch templateVersionJobStatus {
	case codersdk.ProvisionerJobPending, codersdk.ProvisionerJobRunning:
		httpapi.Write(rw, http.StatusNotAcceptable, httpapi.Response{
			Message: fmt.Sprintf("The provided template version is %s. Wait for it to complete importing!", templateVersionJobStatus),
		})
		return
	case codersdk.ProvisionerJobFailed:
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: fmt.Sprintf("The provided template version %q has failed to import: %q. You cannot build workspaces with it!", templateVersion.Name, templateVersionJob.Error.String),
		})
		return
	case codersdk.ProvisionerJobCanceled:
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: "The provided template version was canceled during import. You cannot builds workspaces with it!",
		})
		return
	}

	template, err := api.Database.GetTemplateByID(r.Context(), templateVersion.TemplateID.UUID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get template: %s", err),
		})
		return
	}

	// Store prior build number to compute new build number
	var priorBuildNum int32
	priorHistory, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(r.Context(), workspace.ID)
	if err == nil {
		priorJob, err := api.Database.GetProvisionerJobByID(r.Context(), priorHistory.JobID)
		if err == nil && convertProvisionerJob(priorJob).Status.Active() {
			httpapi.Write(rw, http.StatusConflict, httpapi.Response{
				Message: "a workspace build is already active",
			})
			return
		}

		priorBuildNum = priorHistory.BuildNumber
	} else if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get prior workspace build: %s", err),
		})
		return
	}

	var workspaceBuild database.WorkspaceBuild
	var provisionerJob database.ProvisionerJob
	// This must happen in a transaction to ensure history can be inserted, and
	// the prior history can update it's "after" column to point at the new.
	err = api.Database.InTx(func(db database.Store) error {
		workspaceBuildID := uuid.New()
		input, err := json.Marshal(workspaceProvisionJob{
			WorkspaceBuildID: workspaceBuildID,
		})
		if err != nil {
			return xerrors.Errorf("marshal provision job: %w", err)
		}
		provisionerJob, err = db.InsertProvisionerJob(r.Context(), database.InsertProvisionerJobParams{
			ID:             uuid.New(),
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			InitiatorID:    apiKey.UserID,
			OrganizationID: template.OrganizationID,
			Provisioner:    template.Provisioner,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			StorageMethod:  templateVersionJob.StorageMethod,
			StorageSource:  templateVersionJob.StorageSource,
			Input:          input,
		})
		if err != nil {
			return xerrors.Errorf("insert provisioner job: %w", err)
		}
		state := createBuild.ProvisionerState
		if len(state) == 0 {
			state = priorHistory.ProvisionerState
		}

		workspaceBuild, err = db.InsertWorkspaceBuild(r.Context(), database.InsertWorkspaceBuildParams{
			ID:                workspaceBuildID,
			CreatedAt:         database.Now(),
			UpdatedAt:         database.Now(),
			WorkspaceID:       workspace.ID,
			TemplateVersionID: templateVersion.ID,
			BuildNumber:       priorBuildNum + 1,
			Name:              namesgenerator.GetRandomName(1),
			ProvisionerState:  state,
			InitiatorID:       apiKey.UserID,
			Transition:        database.WorkspaceTransition(createBuild.Transition),
			JobID:             provisionerJob.ID,
		})
		if err != nil {
			return xerrors.Errorf("insert workspace build: %w", err)
		}

		return nil
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusCreated, convertWorkspaceBuild(workspaceBuild, convertProvisionerJob(provisionerJob)))
}

func (api *API) patchCancelWorkspaceBuild(rw http.ResponseWriter, r *http.Request) {
	workspaceBuild := httpmw.WorkspaceBuildParam(r)
	workspace, err := api.Database.GetWorkspaceByID(r.Context(), workspaceBuild.WorkspaceID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "no workspace exists for this job",
		})
		return
	}

	if !api.Authorize(rw, r, rbac.ActionUpdate, rbac.ResourceWorkspace.
		InOrg(workspace.OrganizationID).WithOwner(workspace.OwnerID.String()).WithID(workspace.ID.String())) {
		return
	}

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

func (api *API) workspaceBuildResources(rw http.ResponseWriter, r *http.Request) {
	workspaceBuild := httpmw.WorkspaceBuildParam(r)
	workspace, err := api.Database.GetWorkspaceByID(r.Context(), workspaceBuild.WorkspaceID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "no workspace exists for this job",
		})
		return
	}

	if !api.Authorize(rw, r, rbac.ActionRead, rbac.ResourceWorkspace.
		InOrg(workspace.OrganizationID).WithOwner(workspace.OwnerID.String()).WithID(workspace.ID.String())) {
		return
	}

	job, err := api.Database.GetProvisionerJobByID(r.Context(), workspaceBuild.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	api.provisionerJobResources(rw, r, job)
}

func (api *API) workspaceBuildLogs(rw http.ResponseWriter, r *http.Request) {
	workspaceBuild := httpmw.WorkspaceBuildParam(r)
	workspace, err := api.Database.GetWorkspaceByID(r.Context(), workspaceBuild.WorkspaceID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "no workspace exists for this job",
		})
		return
	}

	if !api.Authorize(rw, r, rbac.ActionRead, rbac.ResourceWorkspace.
		InOrg(workspace.OrganizationID).WithOwner(workspace.OwnerID.String()).WithID(workspace.ID.String())) {
		return
	}

	job, err := api.Database.GetProvisionerJobByID(r.Context(), workspaceBuild.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	api.provisionerJobLogs(rw, r, job)
}

func (api *API) workspaceBuildState(rw http.ResponseWriter, r *http.Request) {
	workspaceBuild := httpmw.WorkspaceBuildParam(r)
	workspace, err := api.Database.GetWorkspaceByID(r.Context(), workspaceBuild.WorkspaceID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "no workspace exists for this job",
		})
		return
	}

	if !api.Authorize(rw, r, rbac.ActionRead, rbac.ResourceWorkspace.
		InOrg(workspace.OrganizationID).WithOwner(workspace.OwnerID.String()).WithID(workspace.ID.String())) {
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write(workspaceBuild.ProvisionerState)
}

func convertWorkspaceBuild(workspaceBuild database.WorkspaceBuild, job codersdk.ProvisionerJob) codersdk.WorkspaceBuild {
	//nolint:unconvert
	return codersdk.WorkspaceBuild{
		ID:                workspaceBuild.ID,
		CreatedAt:         workspaceBuild.CreatedAt,
		UpdatedAt:         workspaceBuild.UpdatedAt,
		WorkspaceID:       workspaceBuild.WorkspaceID,
		TemplateVersionID: workspaceBuild.TemplateVersionID,
		BuildNumber:       workspaceBuild.BuildNumber,
		Name:              workspaceBuild.Name,
		Transition:        codersdk.WorkspaceTransition(workspaceBuild.Transition),
		InitiatorID:       workspaceBuild.InitiatorID,
		Job:               job,
	}
}

func convertWorkspaceResource(resource database.WorkspaceResource, agents []codersdk.WorkspaceAgent) codersdk.WorkspaceResource {
	return codersdk.WorkspaceResource{
		ID:         resource.ID,
		CreatedAt:  resource.CreatedAt,
		JobID:      resource.JobID,
		Transition: codersdk.WorkspaceTransition(resource.Transition),
		Type:       resource.Type,
		Name:       resource.Name,
		Agents:     agents,
	}
}
