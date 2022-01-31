package coderd

import (
	"context"
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

// Workspace is a per-user deployment of a project. It tracks
// project versions, and can be updated.
type Workspace database.Workspace

// WorkspaceHistory is an at-point representation of a workspace state.
// Iterate on before/after to determine a chronological history.
type WorkspaceHistory struct {
	ID               uuid.UUID                    `json:"id"`
	Name             string                       `json:"name"`
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

type WorkspaceHistoryLog struct {
	ID        uuid.UUID
	CreatedAt time.Time          `json:"created_at"`
	Source    database.LogSource `json:"log_source"`
	Level     database.LogLevel  `json:"log_level"`
	Output    string             `json:"output"`
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
	Pubsub   database.Pubsub
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
	project, err := w.Database.GetProjectByID(r.Context(), projectHistory.ProjectID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project: %s", err),
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
		// Generate the ID before-hand so the provisioner job is aware of it!
		workspaceHistoryID := uuid.New()
		input, err := json.Marshal(workspaceProvisionJob{
			WorkspaceHistoryID: workspaceHistoryID,
		})
		if err != nil {
			return xerrors.Errorf("marshal provision job: %w", err)
		}

		provisionerJob, err := db.InsertProvisionerJob(context.Background(), database.InsertProvisionerJobParams{
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
			Name:             namesgenerator.GetRandomName(1),
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
	render.JSON(rw, r, convertWorkspaceHistory(workspaceHistory))
}

func (w *workspaces) workspaceHistoryLogs(rw http.ResponseWriter, r *http.Request) {
	workspaceHistory := httpmw.WorkspaceHistoryParam(r)
	follow := r.URL.Query().Has("follow")

	if !follow {
		// If we're not attempting to follow logs,
		// we can exit immediately!
		logs, err := w.Database.GetWorkspaceHistoryLogsByIDBefore(r.Context(), database.GetWorkspaceHistoryLogsByIDBeforeParams{
			WorkspaceHistoryID: workspaceHistory.ID,
			CreatedAt:          time.Now(),
		})
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("get workspace history logs: %s", err),
			})
			return
		}
		render.Status(r, http.StatusOK)
		render.JSON(rw, r, logs)
		return
	}

	// We only want to fetch messages before subscribe, so that
	// there aren't any duplicates.
	timeBeforeSubscribe := database.Now()
	// Start subscribing immediately, otherwise we could miss messages
	// that occur during the database read.
	newLogNotify := make(chan WorkspaceHistoryLog, 128)
	cancelNewLogNotify, err := w.Pubsub.Subscribe(workspaceHistoryLogsChannel(workspaceHistory.ID), func(ctx context.Context, message []byte) {
		var logs []database.WorkspaceHistoryLog
		err := json.Unmarshal(message, &logs)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("parse logs from publish: %s", err),
			})
			return
		}
		for _, log := range logs {
			// If many logs are sent during our database query, this channel
			// could overflow. The Go scheduler would decide the order to send
			// logs in at that point, which is an unfortunate (but not fatal)
			// flaw of this approach.
			//
			// This is an extremely unlikely outcome given reasonable database
			// query times.
			newLogNotify <- convertWorkspaceHistoryLog(log)
		}
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("listen for new logs: %s", err),
		})
		return
	}
	defer cancelNewLogNotify()

	workspaceHistoryLogs, err := w.Database.GetWorkspaceHistoryLogsByIDBefore(r.Context(), database.GetWorkspaceHistoryLogsByIDBeforeParams{
		WorkspaceHistoryID: workspaceHistory.ID,
		CreatedAt:          timeBeforeSubscribe,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace history logs: %s", err),
		})
		return
	}

	// "follow" uses the ndjson format to stream data.
	// See: https://canjs.com/doc/can-ndjson-stream.html
	rw.Header().Set("Content-Type", "application/stream+json")
	rw.WriteHeader(http.StatusOK)
	rw.(http.Flusher).Flush()

	// The Go stdlib JSON encoder appends a newline character after message write.
	encoder := json.NewEncoder(rw)
	for _, workspaceHistoryLog := range workspaceHistoryLogs {
		// JSON separated by a newline
		err = encoder.Encode(convertWorkspaceHistoryLog(workspaceHistoryLog))
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("marshal: %s", err),
			})
			return
		}
		rw.(http.Flusher).Flush()
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case log := <-newLogNotify:
			err = encoder.Encode(log)
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: fmt.Sprintf("marshal follow: %s", err),
				})
				return
			}
			rw.(http.Flusher).Flush()
		}
	}
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
		Name:             workspaceHistory.Name,
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

func convertWorkspaceHistoryLog(workspaceHistoryLog database.WorkspaceHistoryLog) WorkspaceHistoryLog {
	return WorkspaceHistoryLog{
		ID:        workspaceHistoryLog.ID,
		CreatedAt: workspaceHistoryLog.CreatedAt,
		Source:    workspaceHistoryLog.Source,
		Level:     workspaceHistoryLog.Level,
		Output:    workspaceHistoryLog.Output,
	}
}

func workspaceHistoryLogsChannel(workspaceHistoryID uuid.UUID) string {
	return fmt.Sprintf("workspace-history-logs:%s", workspaceHistoryID)
}
