package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/render"
	"github.com/google/uuid"

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
)

// Workspace is the JSON representation of a Coder workspace.
// This type matches the database object for now, but is
// abstract for ease of change later on.
type Workspace database.Workspace

// CreateWorkspaceRequest enables callers to create a new Workspace.
type CreateWorkspaceRequest struct {
	Name string `json:"name" validate:"username,required"`
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

// convertWorkspace consumes the database representation and outputs an API friendly representation.
func convertWorkspace(workspace database.Workspace) Workspace {
	return Workspace(workspace)
}
