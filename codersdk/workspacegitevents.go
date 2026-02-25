package codersdk

import (
	"time"

	"github.com/google/uuid"
)

// WorkspaceGitEventType represents the type of git event captured during an AI
// coding session.
type WorkspaceGitEventType string

const (
	WorkspaceGitEventTypeSessionStart WorkspaceGitEventType = "session_start"
	WorkspaceGitEventTypeCommit       WorkspaceGitEventType = "commit"
	WorkspaceGitEventTypePush         WorkspaceGitEventType = "push"
	WorkspaceGitEventTypeSessionEnd   WorkspaceGitEventType = "session_end"
)

// CreateWorkspaceGitEventRequest is the payload sent by workspace agents to
// report git activity (commits, pushes, session boundaries) to the control
// plane.
type CreateWorkspaceGitEventRequest struct {
	EventType     WorkspaceGitEventType `json:"event_type" validate:"required"`
	SessionID     string                `json:"session_id,omitempty"`
	CommitSHA     string                `json:"commit_sha,omitempty"`
	CommitMessage string                `json:"commit_message,omitempty"`
	Branch        string                `json:"branch,omitempty"`
	RepoName      string                `json:"repo_name,omitempty"`
	FilesChanged  []string              `json:"files_changed,omitempty"`
	AgentName     string                `json:"agent_name,omitempty"`
}

// WorkspaceGitEvent is a single git event stored for a workspace.
type WorkspaceGitEvent struct {
	ID             uuid.UUID             `json:"id" format:"uuid"`
	WorkspaceID    uuid.UUID             `json:"workspace_id" format:"uuid"`
	AgentID        uuid.UUID             `json:"agent_id" format:"uuid"`
	OwnerID        uuid.UUID             `json:"owner_id" format:"uuid"`
	OrganizationID uuid.UUID             `json:"organization_id" format:"uuid"`
	EventType      WorkspaceGitEventType `json:"event_type"`
	SessionID      string                `json:"session_id,omitempty"`
	CommitSHA      string                `json:"commit_sha,omitempty"`
	CommitMessage  string                `json:"commit_message,omitempty"`
	Branch         string                `json:"branch,omitempty"`
	RepoName       string                `json:"repo_name,omitempty"`
	FilesChanged   []string              `json:"files_changed,omitempty"`
	AgentName      string                `json:"agent_name,omitempty"`
	CreatedAt      time.Time             `json:"created_at" format:"date-time"`
}
