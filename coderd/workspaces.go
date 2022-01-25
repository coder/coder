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

// Workspace is a per-user deployment of a project. It tracks
// project versions, and can be updated.
type Workspace database.Workspace

// WorkspaceHistory is an at-point representation of a workspace state.
// Iterate on before/after to determine a chronological history.
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

// CreateWorkspaceRequest provides options for creating a new workspace.
type CreateWorkspaceRequest struct {
	ProjectID uuid.UUID `json:"project_id" validate:"required"`
	Name      string    `json:"name" validate:"username,required"`
}

// CreateWorkspaceHistoryRequest provides options to update the latest workspace history.
type CreateWorkspaceHistoryRequest struct {
	ProjectHistoryID uuid.UUID                    `json:"project_history_id" validate:"required"`
	Transition       database.WorkspaceTransition `json:"transition" validate:"oneof=create start stop delete,required"`
}

type workspaces struct {
	Database database.Store
}

// Returns all workspaces across all projects and organizations.
func (w *workspaces) listAllWorkspaces(rw http.ResponseWriter, r *http.Request) {
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

// Returns all workspaces for a specific project.
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

// Create a new workspace for the currently authenticated user.
func (w *workspaces) createWorkspaceForUser(rw http.ResponseWriter, r *http.Request) {
	var createWorkspace CreateWorkspaceRequest
	if !httpapi.Read(rw, r, &createWorkspace) {
		return
	}
	apiKey := httpmw.APIKey(r)
	project, err := w.Database.GetProjectByID(r.Context(), createWorkspace.ProjectID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("project %q doesn't exist", createWorkspace.ProjectID.String()),
			Errors: []httpapi.Error{{
				Field: "project_id",
				Code:  "not_found",
			}},
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project: %s", err),
		})
		return
	}
	_, err = w.Database.GetOrganizationMemberByUserID(r.Context(), database.GetOrganizationMemberByUserIDParams{
		OrganizationID: project.OrganizationID,
		UserID:         apiKey.UserID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: "you aren't allowed to access projects in that organization",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get organization member: %s", err),
		})
		return
	}

	workspace, err := w.Database.GetWorkspaceByUserIDAndName(r.Context(), database.GetWorkspaceByUserIDAndNameParams{
		OwnerID: apiKey.UserID,
		Name:    createWorkspace.Name,
	})
	if err == nil {
		// If the workspace already exists, don't allow creation.
		project, err := w.Database.GetProjectByID(r.Context(), workspace.ProjectID)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("find project for conflicting workspace name %q: %s", createWorkspace.Name, err),
			})
			return
		}
		// The project is fetched for clarity to the user on where the conflicting name may be.
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

	// Workspaces are created without any versions.
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

// Returns a single singleWorkspace.
func (*workspaces) singleWorkspace(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertWorkspace(workspace))
}

// Returns all workspace history. This is not sorted. Use before/after to chronologically sort.
func (w *workspaces) listAllWorkspaceHistory(rw http.ResponseWriter, r *http.Request) {
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

// Returns the latest workspace history. This works by querying for history without "after" set.
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

// Begins transitioning a workspace to new state. This queues a provision job to asyncronously
// update the underlying infrastructure. Only one historical transition can occur at a time.
func (w *workspaces) createWorkspaceHistory(rw http.ResponseWriter, r *http.Request) {
	var createBuild CreateWorkspaceHistoryRequest
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
	// This must happen in a transaction to ensure history can be inserted, and
	// the prior history can update it's "after" column to point at the new.
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
	render.JSON(rw, r, convertWorkspaceHistory(workspaceHistory))
}

// Converts the internal workspace representation to a public external-facing model.
func convertWorkspace(workspace database.Workspace) Workspace {
	return Workspace(workspace)
}

// Converts the internal history representation to a public external-facing model.
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
