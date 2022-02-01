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
	ProjectHistoryID uuid.UUID                    `json:"project_history_id"`
	BeforeID         uuid.UUID                    `json:"before_id"`
	AfterID          uuid.UUID                    `json:"after_id"`
	Transition       database.WorkspaceTransition `json:"transition"`
	Initiator        string                       `json:"initiator"`
	Job              ProvisionerJob               `json:"job"`
}

// CreateWorkspaceHistoryRequest provides options to update the latest workspace history.
type CreateWorkspaceHistoryRequest struct {
	ProjectHistoryID uuid.UUID                    `json:"project_history_id" validate:"required"`
	Transition       database.WorkspaceTransition `json:"transition" validate:"oneof=create start stop delete,required"`
}

func (api *api) postWorkspaceHistoryByUser(rw http.ResponseWriter, r *http.Request) {
	var createBuild CreateWorkspaceHistoryRequest
	if !httpapi.Read(rw, r, &createBuild) {
		return
	}
	user := httpmw.UserParam(r)
	workspace := httpmw.WorkspaceParam(r)
	projectHistory, err := api.Database.GetProjectHistoryByID(r.Context(), createBuild.ProjectHistoryID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "project history not found",
			Errors: []httpapi.Error{{
				Field: "project_history_id",
				Code:  "exists",
			}},
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project history: %s", err),
		})
		return
	}
	project, err := api.Database.GetProjectByID(r.Context(), projectHistory.ProjectID)
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
		if err == nil {
			convertedJob := convertProvisionerJob(priorJob)
			if convertedJob.Status == ProvisionerJobStatusPending ||
				convertedJob.Status == ProvisionerJobStatusRunning {
				httpapi.Write(rw, http.StatusConflict, httpapi.Response{
					Message: "a workspace build is already active",
				})
				return
			}
		}

		priorHistoryID = uuid.NullUUID{
			UUID:  priorHistory.ID,
			Valid: true,
		}
	}
	if !errors.Is(err, sql.ErrNoRows) {
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
			ID:               uuid.New(),
			CreatedAt:        database.Now(),
			UpdatedAt:        database.Now(),
			WorkspaceID:      workspace.ID,
			ProjectHistoryID: projectHistory.ID,
			BeforeID:         priorHistoryID,
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
				ID:        priorHistory.ID,
				UpdatedAt: database.Now(),
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

	histories, err := api.Database.GetWorkspaceHistoryByWorkspaceID(r.Context(), workspace.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace history: %s", err),
		})
		return
	}

	apiHistory := make([]WorkspaceHistory, 0, len(histories))
	for _, history := range histories {
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

// Returns the latest workspace history. This works by querying for history without "after" set.
func (api *api) latestWorkspaceHistoryByUser(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)

	history, err := api.Database.GetWorkspaceHistoryByWorkspaceIDWithoutAfter(r.Context(), workspace.ID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: "workspace has no history",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace history: %s", err),
		})
		return
	}
	job, err := api.Database.GetProvisionerJobByID(r.Context(), history.ProvisionJobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertWorkspaceHistory(history, job))
}

// Converts the internal history representation to a public external-facing model.
func convertWorkspaceHistory(workspaceHistory database.WorkspaceHistory, provisionerJob database.ProvisionerJob) WorkspaceHistory {
	//nolint:unconvert
	return WorkspaceHistory(WorkspaceHistory{
		ID:               workspaceHistory.ID,
		CreatedAt:        workspaceHistory.CreatedAt,
		UpdatedAt:        workspaceHistory.UpdatedAt,
		WorkspaceID:      workspaceHistory.WorkspaceID,
		ProjectHistoryID: workspaceHistory.ProjectHistoryID,
		BeforeID:         workspaceHistory.BeforeID.UUID,
		AfterID:          workspaceHistory.AfterID.UUID,
		Transition:       workspaceHistory.Transition,
		Initiator:        workspaceHistory.Initiator,
		Job:              convertProvisionerJob(provisionerJob),
	})
}

func workspaceHistoryLogsChannel(workspaceHistoryID uuid.UUID) string {
	return fmt.Sprintf("workspace-history-logs:%s", workspaceHistoryID)
}
