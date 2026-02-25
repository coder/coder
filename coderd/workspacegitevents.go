package coderd

import (
	"database/sql"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Report workspace git event
// @ID report-workspace-git-event
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Agents
// @Param request body codersdk.CreateWorkspaceGitEventRequest true "Git event"
// @Success 201 {object} codersdk.WorkspaceGitEvent
// @Router /workspaceagents/me/git-events [post]
func (api *API) postWorkspaceAgentGitEvent(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceAgent := httpmw.WorkspaceAgent(r)

	var req codersdk.CreateWorkspaceGitEventRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// Validate event_type is one of the allowed values.
	switch req.EventType {
	case codersdk.WorkspaceGitEventTypeSessionStart,
		codersdk.WorkspaceGitEventTypeCommit,
		codersdk.WorkspaceGitEventTypePush,
		codersdk.WorkspaceGitEventTypeSessionEnd:
	default:
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid event type.",
			Detail:  "event_type must be one of: session_start, commit, push, session_end.",
		})
		return
	}

	// Fetch the workspace to obtain owner_id and organization_id.
	workspace, err := api.Database.GetWorkspaceByAgentID(ctx, workspaceAgent.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get workspace.",
			Detail:  err.Error(),
		})
		return
	}

	event, err := api.Database.InsertWorkspaceGitEvent(ctx, database.InsertWorkspaceGitEventParams{
		WorkspaceID:    workspace.ID,
		AgentID:        workspaceAgent.ID,
		OwnerID:        workspace.OwnerID,
		OrganizationID: workspace.OrganizationID,
		EventType:      string(req.EventType),
		SessionID: sql.NullString{
			String: req.SessionID,
			Valid:  req.SessionID != "",
		},
		CommitSha: sql.NullString{
			String: req.CommitSHA,
			Valid:  req.CommitSHA != "",
		},
		CommitMessage: sql.NullString{
			String: req.CommitMessage,
			Valid:  req.CommitMessage != "",
		},
		Branch: sql.NullString{
			String: req.Branch,
			Valid:  req.Branch != "",
		},
		RepoName: sql.NullString{
			String: req.RepoName,
			Valid:  req.RepoName != "",
		},
		FilesChanged: req.FilesChanged,
		AgentName: sql.NullString{
			String: req.AgentName,
			Valid:  req.AgentName != "",
		},
		AiBridgeInterceptionID: uuid.NullUUID{},
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to insert git event.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.WorkspaceGitEvent{
		ID:             event.ID,
		WorkspaceID:    event.WorkspaceID,
		AgentID:        event.AgentID,
		OwnerID:        event.OwnerID,
		OrganizationID: event.OrganizationID,
		EventType:      codersdk.WorkspaceGitEventType(event.EventType),
		SessionID:      event.SessionID.String,
		CommitSHA:      event.CommitSha.String,
		CommitMessage:  event.CommitMessage.String,
		Branch:         event.Branch.String,
		RepoName:       event.RepoName.String,
		FilesChanged:   event.FilesChanged,
		AgentName:      event.AgentName.String,
		CreatedAt:      event.CreatedAt,
	})
}
