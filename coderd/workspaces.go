package coderd

import (
	"database/sql"
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

// Workspace is the JSON representation of a Coder workspace.
// This type matches the database object for now, but is
// abstract for ease of change later on.
type Workspace database.Workspace

// WorkspaceHistory is the JSON representation of a workspace transitioning
// from state-to-state.
type WorkspaceHistory struct {
	ID               uuid.UUID                    `json:"id"`
	CreatedAt        time.Time                    `json:"created_at"`
	UpdatedAt        time.Time                    `json:"updated_at"`
	CompletedAt      time.Time                    `json:"completed_at"`
	WorkspaceID      uuid.UUID                    `json:"workspace_id"`
	ProjectHistoryID uuid.UUID                    `json:"project_history_id"`
	BeforeID         uuid.UUID                    `json:"before_id"`
	AfterID          uuid.UUID                    `json:"after_id"`
	Transition       database.WorkspaceTransition `json:"transition"`
	Initiator        string                       `json:"initiator"`
}

// CreateWorkspaceRequest enables callers to create a new Workspace.
type CreateWorkspaceRequest struct {
	Name string `json:"name" validate:"username,required"`
}

// CreateWorkspaceBuildRequest enables callers to create a new workspace build.
type CreateWorkspaceBuildRequest struct {
	ProjectHistoryID uuid.UUID                    `json:"project_history_id" validate:"required"`
	Transition       database.WorkspaceTransition `json:"transition" validate:"oneof=create start stop delete,required"`
}

type workspaces struct {
	Database database.Store
}

// allWorkspaces lists all workspaces for the currently authenticated user.
func (w *workspaces) allWorkspaces(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	workspaces, err := w.Database.GetWorkspacesByUserID(r.Context(), apiKey.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspaces: %s", err),
		})
		return
	}

	apiWorkspaces := make([]Workspace, 0, len(workspaces))
	for _, workspace := range workspaces {
		apiWorkspaces = append(apiWorkspaces, convertWorkspace(workspace))
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, apiWorkspaces)
}

// allWorkspacesForProject lists all projects for the parameterized project.
func (w *workspaces) allWorkspacesForProject(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	project := httpmw.ProjectParam(r)
	workspaces, err := w.Database.GetWorkspacesByProjectAndUserID(r.Context(), database.GetWorkspacesByProjectAndUserIDParams{
		OwnerID:   apiKey.UserID,
		ProjectID: project.ID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspaces: %s", err),
		})
		return
	}

	apiWorkspaces := make([]Workspace, 0, len(workspaces))
	for _, workspace := range workspaces {
		apiWorkspaces = append(apiWorkspaces, convertWorkspace(workspace))
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, apiWorkspaces)
}

// createWorkspace creates a new workspace for the currently authenticated user.
func (w *workspaces) createWorkspace(rw http.ResponseWriter, r *http.Request) {
	var createWorkspace CreateWorkspaceRequest
	if !httpapi.Read(rw, r, &createWorkspace) {
		return
	}
	apiKey := httpmw.APIKey(r)
	project := httpmw.ProjectParam(r)

	workspace, err := w.Database.GetWorkspaceByUserIDAndName(r.Context(), database.GetWorkspaceByUserIDAndNameParams{
		OwnerID: apiKey.UserID,
		Name:    createWorkspace.Name,
	})
	if err == nil {
		project, err := w.Database.GetProjectByID(r.Context(), workspace.ProjectID)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("find project for conflicting workspace name %q: %s", createWorkspace.Name, err),
			})
			return
		}
		httpapi.Write(rw, http.StatusConflict, httpapi.Response{
			Message: fmt.Sprintf("workspace %q already exists in the %q project", createWorkspace.Name, project.Name),
			Errors: []httpapi.Error{{
				Field: "name",
				Code:  "exists",
			}},
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace by name: %s", err.Error()),
		})
		return
	}

	workspace, err = w.Database.InsertWorkspace(r.Context(), database.InsertWorkspaceParams{
		ID:        uuid.New(),
		CreatedAt: database.Now(),
		UpdatedAt: database.Now(),
		OwnerID:   apiKey.UserID,
		ProjectID: project.ID,
		Name:      createWorkspace.Name,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("insert workspace: %s", err),
		})
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, convertWorkspace(workspace))
}

func (*workspaces) workspace(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertWorkspace(workspace))
}

func (w *workspaces) allWorkspaceHistory(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)

	histories, err := w.Database.GetWorkspaceHistoryByWorkspaceID(r.Context(), workspace.ID)
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
		apiHistory = append(apiHistory, convertWorkspaceHistory(history))
	}

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, apiHistory)
}

func (w *workspaces) latestWorkspaceHistory(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)

	history, err := w.Database.GetWorkspaceHistoryByWorkspaceIDWithoutAfter(r.Context(), workspace.ID)
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

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertWorkspaceHistory(history))
}

func (w *workspaces) createWorkspaceBuild(rw http.ResponseWriter, r *http.Request) {
	var createBuild CreateWorkspaceBuildRequest
	if !httpapi.Read(rw, r, &createBuild) {
		return
	}
	user := httpmw.UserParam(r)
	workspace := httpmw.WorkspaceParam(r)
	projectHistory, err := w.Database.GetProjectHistoryByID(r.Context(), createBuild.ProjectHistoryID)
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

	// Store prior history ID if it exists to update it after we create new!
	priorHistoryID := uuid.NullUUID{}
	priorHistory, err := w.Database.GetWorkspaceHistoryByWorkspaceIDWithoutAfter(r.Context(), workspace.ID)
	if err == nil {
		if !priorHistory.CompletedAt.Valid {
			httpapi.Write(rw, http.StatusConflict, httpapi.Response{
				Message: "a workspace build is already active",
			})
			return
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

	var workspaceHistory database.WorkspaceHistory
	err = w.Database.InTx(func(db database.Store) error {
		workspaceHistory, err = db.InsertWorkspaceHistory(r.Context(), database.InsertWorkspaceHistoryParams{
			ID:               uuid.New(),
			CreatedAt:        database.Now(),
			UpdatedAt:        database.Now(),
			WorkspaceID:      workspace.ID,
			ProjectHistoryID: projectHistory.ID,
			BeforeID:         priorHistoryID,
			Initiator:        user.ID,
			Transition:       createBuild.Transition,
			// This should create a provision job once that gets implemented!
			ProvisionJobID: uuid.New(),
		})
		if err != nil {
			return xerrors.Errorf("insert workspace history: %w", err)
		}

		if priorHistoryID.Valid {
			err = db.UpdateWorkspaceHistoryByID(r.Context(), database.UpdateWorkspaceHistoryByIDParams{
				ID:               priorHistory.ID,
				UpdatedAt:        database.Now(),
				ProvisionerState: priorHistory.ProvisionerState,
				CompletedAt:      priorHistory.CompletedAt,
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
	render.JSON(rw, r, convertWorkspaceHistory(workspaceHistory))
}

// convertWorkspace consumes the database representation and outputs an API friendly representation.
func convertWorkspace(workspace database.Workspace) Workspace {
	return Workspace(workspace)
}

func convertWorkspaceHistory(workspaceHistory database.WorkspaceHistory) WorkspaceHistory {
	//nolint:unconvert
	return WorkspaceHistory(WorkspaceHistory{
		ID:               workspaceHistory.ID,
		CreatedAt:        workspaceHistory.CreatedAt,
		UpdatedAt:        workspaceHistory.UpdatedAt,
		CompletedAt:      workspaceHistory.CompletedAt.Time,
		WorkspaceID:      workspaceHistory.WorkspaceID,
		ProjectHistoryID: workspaceHistory.ProjectHistoryID,
		BeforeID:         workspaceHistory.BeforeID.UUID,
		AfterID:          workspaceHistory.AfterID.UUID,
		Transition:       workspaceHistory.Transition,
		Initiator:        workspaceHistory.Initiator,
	})
}
