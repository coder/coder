package coderd

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/render"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"

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
	ProvisionJobID   uuid.UUID                    `json:"provision_job_id"`
}

// CreateWorkspaceBuildRequest provides options to update the latest workspace build.
type CreateWorkspaceBuildRequest struct {
	ProjectVersionID uuid.UUID                    `json:"project_version_id" validate:"required"`
	Transition       database.WorkspaceTransition `json:"transition" validate:"oneof=create start stop delete,required"`
}

func (api *api) postWorkspaceBuildByUser(rw http.ResponseWriter, r *http.Request) {
	var createBuild CreateWorkspaceBuildRequest
	if !httpapi.Read(rw, r, &createBuild) {
		return
	}
	user := httpmw.UserParam(r)
	workspace := httpmw.WorkspaceParam(r)
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
	projectVersionJob, err := api.Database.GetProvisionerJobByID(r.Context(), projectVersion.ImportJobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	projectVersionJobStatus := convertProvisionerJob(projectVersionJob).Status
	switch projectVersionJobStatus {
	case ProvisionerJobStatusPending, ProvisionerJobStatusRunning:
		httpapi.Write(rw, http.StatusNotAcceptable, httpapi.Response{
			Message: fmt.Sprintf("The provided project version is %s. Wait for it to complete importing!", projectVersionJobStatus),
		})
		return
	case ProvisionerJobStatusFailed:
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: fmt.Sprintf("The provided project version %q has failed to import. You cannot create workspaces using it!", projectVersion.Name),
		})
		return
	case ProvisionerJobStatusCancelled:
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: "The provided project version was canceled during import. You cannot create workspaces using it!",
		})
		return
	}

	project, err := api.Database.GetProjectByID(r.Context(), projectVersion.ProjectID)
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
		priorJob, err := api.Database.GetProvisionerJobByID(r.Context(), priorHistory.ProvisionJobID)
		if err == nil && !convertProvisionerJob(priorJob).Status.Completed() {
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
	// This must happen in a transaction to ensure history can be inserted, and
	// the prior history can update it's "after" column to point at the new.
	err = api.Database.InTx(func(db database.Store) error {
		provisionerJobID := uuid.New()
		workspaceBuild, err = db.InsertWorkspaceBuild(r.Context(), database.InsertWorkspaceBuildParams{
			ID:               uuid.New(),
			CreatedAt:        database.Now(),
			UpdatedAt:        database.Now(),
			WorkspaceID:      workspace.ID,
			ProjectVersionID: projectVersion.ID,
			BeforeID:         priorHistoryID,
			Name:             namesgenerator.GetRandomName(1),
			Initiator:        user.ID,
			Transition:       createBuild.Transition,
			ProvisionJobID:   provisionerJobID,
		})
		if err != nil {
			return xerrors.Errorf("insert workspace build: %w", err)
		}

		input, err := json.Marshal(workspaceProvisionJob{
			WorkspaceBuildID: workspaceBuild.ID,
		})
		if err != nil {
			return xerrors.Errorf("marshal provision job: %w", err)
		}

		_, err = db.InsertProvisionerJob(r.Context(), database.InsertProvisionerJobParams{
			ID:             provisionerJobID,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			InitiatorID:    user.ID,
			OrganizationID: project.OrganizationID,
			Provisioner:    project.Provisioner,
			Type:           database.ProvisionerJobTypeWorkspaceProvision,
			StorageMethod:  projectVersionJob.StorageMethod,
			StorageSource:  projectVersionJob.StorageSource,
			Input:          input,
		})
		if err != nil {
			return xerrors.Errorf("insert provisioner job: %w", err)
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
	render.JSON(rw, r, convertWorkspaceBuild(workspaceBuild))
}

// Returns all workspace build. This is not sorted. Use before/after to chronologically sort.
func (api *api) workspaceBuildByUser(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)

	history, err := api.Database.GetWorkspaceBuildByWorkspaceID(r.Context(), workspace.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
		history = []database.WorkspaceBuild{}
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace build: %s", err),
		})
		return
	}

	apiHistory := make([]WorkspaceBuild, 0, len(history))
	for _, history := range history {
		apiHistory = append(apiHistory, convertWorkspaceBuild(history))
	}

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, apiHistory)
}

func (*api) workspaceBuildByName(rw http.ResponseWriter, r *http.Request) {
	workspaceBuild := httpmw.WorkspaceBuildParam(r)
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertWorkspaceBuild(workspaceBuild))
}

// Converts the internal history representation to a public external-facing model.
func convertWorkspaceBuild(workspaceBuild database.WorkspaceBuild) WorkspaceBuild {
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
		ProvisionJobID:   workspaceBuild.ProvisionJobID,
	})
}
