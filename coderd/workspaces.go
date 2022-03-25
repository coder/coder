package coderd

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

func (api *api) workspace(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)
	render.Status(r, http.StatusOK)

	build, err := api.Database.GetWorkspaceBuildByWorkspaceIDWithoutAfter(r.Context(), workspace.ID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace build: %s", err),
		})
		return
	}
	job, err := api.Database.GetProvisionerJobByID(r.Context(), build.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace build job: %s", err),
		})
		return
	}
	project, err := api.Database.GetProjectByID(r.Context(), workspace.ProjectID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project: %s", err),
		})
		return
	}
	render.JSON(rw, r, convertWorkspace(workspace, convertWorkspaceBuild(build, convertProvisionerJob(job)), project))
}

func (api *api) workspaceBuilds(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)

	builds, err := api.Database.GetWorkspaceBuildByWorkspaceID(r.Context(), workspace.ID)
	if errors.Is(err, sql.ErrNoRows) {
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

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, apiBuilds)
}

func (api *api) postWorkspaceBuilds(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	workspace := httpmw.WorkspaceParam(r)
	var createBuild codersdk.CreateWorkspaceBuildRequest
	if !httpapi.Read(rw, r, &createBuild) {
		return
	}
	if createBuild.ProjectVersionID == uuid.Nil {
		latestBuild, err := api.Database.GetWorkspaceBuildByWorkspaceIDWithoutAfter(r.Context(), workspace.ID)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("get latest workspace build: %s", err),
			})
			return
		}
		createBuild.ProjectVersionID = latestBuild.ProjectVersionID
	}
	projectVersion, err := api.Database.GetProjectVersionByID(r.Context(), createBuild.ProjectVersionID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "project version not found",
			Errors: []httpapi.Error{{
				Field: "project_version_id",
				Code:  "exists",
			}},
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project version: %s", err),
		})
		return
	}
	projectVersionJob, err := api.Database.GetProvisionerJobByID(r.Context(), projectVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	projectVersionJobStatus := convertProvisionerJob(projectVersionJob).Status
	switch projectVersionJobStatus {
	case codersdk.ProvisionerJobPending, codersdk.ProvisionerJobRunning:
		httpapi.Write(rw, http.StatusNotAcceptable, httpapi.Response{
			Message: fmt.Sprintf("The provided project version is %s. Wait for it to complete importing!", projectVersionJobStatus),
		})
		return
	case codersdk.ProvisionerJobFailed:
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: fmt.Sprintf("The provided project version %q has failed to import: %q. You cannot build workspaces with it!", projectVersion.Name, projectVersionJob.Error.String),
		})
		return
	case codersdk.ProvisionerJobCanceled:
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: "The provided project version was canceled during import. You cannot builds workspaces with it!",
		})
		return
	}

	project, err := api.Database.GetProjectByID(r.Context(), projectVersion.ProjectID.UUID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project: %s", err),
		})
		return
	}

	// Store prior history ID if it exists to update it after we create new!
	priorHistoryID := uuid.NullUUID{}
	priorHistory, err := api.Database.GetWorkspaceBuildByWorkspaceIDWithoutAfter(r.Context(), workspace.ID)
	if err == nil {
		priorJob, err := api.Database.GetProvisionerJobByID(r.Context(), priorHistory.JobID)
		if err == nil && !priorJob.CompletedAt.Valid {
			httpapi.Write(rw, http.StatusConflict, httpapi.Response{
				Message: "a workspace build is already active",
			})
			return
		}

		priorHistoryID = uuid.NullUUID{
			UUID:  priorHistory.ID,
			Valid: true,
		}
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
			OrganizationID: project.OrganizationID,
			Provisioner:    project.Provisioner,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			StorageMethod:  projectVersionJob.StorageMethod,
			StorageSource:  projectVersionJob.StorageSource,
			Input:          input,
		})
		if err != nil {
			return xerrors.Errorf("insert provisioner job: %w", err)
		}
		workspaceBuild, err = db.InsertWorkspaceBuild(r.Context(), database.InsertWorkspaceBuildParams{
			ID:               workspaceBuildID,
			CreatedAt:        database.Now(),
			UpdatedAt:        database.Now(),
			WorkspaceID:      workspace.ID,
			ProjectVersionID: projectVersion.ID,
			BeforeID:         priorHistoryID,
			Name:             namesgenerator.GetRandomName(1),
			ProvisionerState: priorHistory.ProvisionerState,
			Initiator:        apiKey.UserID,
			Transition:       createBuild.Transition,
			JobID:            provisionerJob.ID,
		})
		if err != nil {
			return xerrors.Errorf("insert workspace build: %w", err)
		}

		if priorHistoryID.Valid {
			// Update the prior history entries "after" column.
			err = db.UpdateWorkspaceBuildByID(r.Context(), database.UpdateWorkspaceBuildByIDParams{
				ID:               priorHistory.ID,
				ProvisionerState: priorHistory.ProvisionerState,
				UpdatedAt:        database.Now(),
				AfterID: uuid.NullUUID{
					UUID:  workspaceBuild.ID,
					Valid: true,
				},
			})
			if err != nil {
				return xerrors.Errorf("update prior workspace build: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: err.Error(),
		})
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, convertWorkspaceBuild(workspaceBuild, convertProvisionerJob(provisionerJob)))
}

func (api *api) workspaceBuildByName(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)
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

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertWorkspaceBuild(workspaceBuild, convertProvisionerJob(job)))
}

func convertWorkspace(workspace database.Workspace, workspaceBuild codersdk.WorkspaceBuild, project database.Project) codersdk.Workspace {
	return codersdk.Workspace{
		ID:          workspace.ID,
		CreatedAt:   workspace.CreatedAt,
		UpdatedAt:   workspace.UpdatedAt,
		OwnerID:     workspace.OwnerID,
		ProjectID:   workspace.ProjectID,
		LatestBuild: workspaceBuild,
		ProjectName: project.Name,
		Outdated:    workspaceBuild.ProjectVersionID.String() != project.ActiveVersionID.String(),
		Name:        workspace.Name,
	}
}
