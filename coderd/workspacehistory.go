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

// WorkspaceHistory is an at-point representation of a workspace state.
// Iterate on before/after to determine a chronological history.
type WorkspaceHistory struct {
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
	Provision        ProvisionerJob               `json:"provision"`
}

// CreateWorkspaceHistoryRequest provides options to update the latest workspace history.
type CreateWorkspaceHistoryRequest struct {
	ProjectVersionID uuid.UUID                    `json:"project_version_id" validate:"required"`
	Transition       database.WorkspaceTransition `json:"transition" validate:"oneof=create start stop delete,required"`
}

func (api *api) postWorkspaceHistoryByUser(rw http.ResponseWriter, r *http.Request) {
	var createBuild CreateWorkspaceHistoryRequest
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
	priorHistory, err := api.Database.GetWorkspaceHistoryByWorkspaceIDWithoutAfter(r.Context(), workspace.ID)
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
			Message: fmt.Sprintf("get prior workspace history: %s", err),
		})
		return
	}

	var provisionerJob database.ProvisionerJob
	var workspaceHistory database.WorkspaceHistory
	// This must happen in a transaction to ensure history can be inserted, and
	// the prior history can update it's "after" column to point at the new.
	err = api.Database.InTx(func(db database.Store) error {
		// Generate the ID before-hand so the provisioner job is aware of it!
		workspaceHistoryID := uuid.New()
		input, err := json.Marshal(workspaceProvisionJob{
			WorkspaceHistoryID: workspaceHistoryID,
		})
		if err != nil {
			return xerrors.Errorf("marshal provision job: %w", err)
		}

		provisionerJob, err = db.InsertProvisionerJob(r.Context(), database.InsertProvisionerJobParams{
			ID:          uuid.New(),
			CreatedAt:   database.Now(),
			UpdatedAt:   database.Now(),
			InitiatorID: user.ID,
			Provisioner: project.Provisioner,
			Type:        database.ProvisionerJobTypeWorkspaceProvision,
			ProjectID:   project.ID,
			Input:       input,
		})
		if err != nil {
			return xerrors.Errorf("insert provisioner job: %w", err)
		}

		workspaceHistory, err = db.InsertWorkspaceHistory(r.Context(), database.InsertWorkspaceHistoryParams{
			ID:               workspaceHistoryID,
			CreatedAt:        database.Now(),
			UpdatedAt:        database.Now(),
			WorkspaceID:      workspace.ID,
			ProjectVersionID: projectVersion.ID,
			BeforeID:         priorHistoryID,
			Name:             namesgenerator.GetRandomName(1),
			Initiator:        user.ID,
			Transition:       createBuild.Transition,
			ProvisionJobID:   provisionerJob.ID,
		})
		if err != nil {
			return xerrors.Errorf("insert workspace history: %w", err)
		}

		if priorHistoryID.Valid {
			// Update the prior history entries "after" column.
			err = db.UpdateWorkspaceHistoryByID(r.Context(), database.UpdateWorkspaceHistoryByIDParams{
				ID:               priorHistory.ID,
				ProvisionerState: priorHistory.ProvisionerState,
				UpdatedAt:        database.Now(),
				AfterID: uuid.NullUUID{
					UUID:  workspaceHistory.ID,
					Valid: true,
				},
			})
			if err != nil {
				return xerrors.Errorf("update prior workspace history: %w", err)
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
	render.JSON(rw, r, convertWorkspaceHistory(workspaceHistory, provisionerJob))
}

// Returns all workspace history. This is not sorted. Use before/after to chronologically sort.
func (api *api) workspaceHistoryByUser(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)

	history, err := api.Database.GetWorkspaceHistoryByWorkspaceID(r.Context(), workspace.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
		history = []database.WorkspaceHistory{}
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace history: %s", err),
		})
		return
	}

	apiHistory := make([]WorkspaceHistory, 0, len(history))
	for _, history := range history {
		job, err := api.Database.GetProvisionerJobByID(r.Context(), history.ProvisionJobID)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("get provisioner job: %s", err),
			})
			return
		}
		apiHistory = append(apiHistory, convertWorkspaceHistory(history, job))
	}

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, apiHistory)
}

func (api *api) workspaceHistoryByName(rw http.ResponseWriter, r *http.Request) {
	workspaceHistory := httpmw.WorkspaceHistoryParam(r)
	job, err := api.Database.GetProvisionerJobByID(r.Context(), workspaceHistory.ProvisionJobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertWorkspaceHistory(workspaceHistory, job))
}

// Converts the internal history representation to a public external-facing model.
func convertWorkspaceHistory(workspaceHistory database.WorkspaceHistory, provisionerJob database.ProvisionerJob) WorkspaceHistory {
	//nolint:unconvert
	return WorkspaceHistory(WorkspaceHistory{
		ID:               workspaceHistory.ID,
		CreatedAt:        workspaceHistory.CreatedAt,
		UpdatedAt:        workspaceHistory.UpdatedAt,
		WorkspaceID:      workspaceHistory.WorkspaceID,
		ProjectVersionID: workspaceHistory.ProjectVersionID,
		BeforeID:         workspaceHistory.BeforeID.UUID,
		AfterID:          workspaceHistory.AfterID.UUID,
		Name:             workspaceHistory.Name,
		Transition:       workspaceHistory.Transition,
		Initiator:        workspaceHistory.Initiator,
		Provision:        convertProvisionerJob(provisionerJob),
	})
}
