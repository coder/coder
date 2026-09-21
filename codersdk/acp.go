package codersdk

import (
	"time"

	"github.com/google/uuid"
)

// ACPSpawnRequest starts one of the two PoC adapters in the workspace.
type ACPSpawnRequest struct {
	Agent  string `json:"agent" enum:"claude_code,codex"`
	Prompt string `json:"prompt"`
	Title  string `json:"title,omitempty"`
}

// ACPMessageRequest submits a prompt to an existing ACP session.
type ACPMessageRequest struct {
	Message   string `json:"message"`
	Interrupt bool   `json:"interrupt,omitempty"`
}

// ACPEntry is a displayable part of an in-memory ACP transcript.
type ACPEntry struct {
	ID     string `json:"id"`
	Role   string `json:"role"`
	Kind   string `json:"kind"`
	Text   string `json:"text"`
	Title  string `json:"title,omitempty"`
	Status string `json:"status,omitempty"`
	Input  any    `json:"input,omitempty"`
	Output any    `json:"output,omitempty"`
}

// ACPSession is an ephemeral session owned by a workspace agent.
type ACPSession struct {
	SessionID        uuid.UUID  `json:"session_id" format:"uuid"`
	WorkspaceAgentID uuid.UUID  `json:"workspace_agent_id" format:"uuid"`
	ParentChatID     uuid.UUID  `json:"parent_chat_id" format:"uuid"`
	Agent            string     `json:"agent"`
	Title            string     `json:"title"`
	Status           string     `json:"status"`
	Error            string     `json:"error,omitempty"`
	CreatedAt        time.Time  `json:"created_at" format:"date-time"`
	UpdatedAt        time.Time  `json:"updated_at" format:"date-time"`
	Version          int64      `json:"version"`
	Entries          []ACPEntry `json:"entries"`
	Queued           int        `json:"queued"`
}

// ACPListResponse lists sessions belonging to a parent chat.
type ACPListResponse struct {
	Agents  []ACPSession `json:"agents"`
	Total   int          `json:"total"`
	HasMore bool         `json:"has_more"`
}
