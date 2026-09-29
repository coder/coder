package codersdk

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// StartWorkspaceCommandRequest declares one stable command intent.
type StartWorkspaceCommandRequest struct {
	RequestID uuid.UUID         `json:"request_id" format:"uuid"`
	AgentID   uuid.UUID         `json:"agent_id" format:"uuid"`
	Command   string            `json:"command"`
	WorkDir   string            `json:"workdir,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

// WorkspaceCommand is the durable receipt. Unknown is never permission to rerun.
type WorkspaceCommand struct {
	ID                uuid.UUID               `json:"id" format:"uuid"`
	SessionID         uuid.UUID               `json:"session_id" format:"uuid"`
	RequestID         uuid.UUID               `json:"request_id" format:"uuid"`
	WorkspaceID       uuid.UUID               `json:"workspace_id" format:"uuid"`
	AgentID           uuid.UUID               `json:"agent_id" format:"uuid"`
	AgentInstanceID   uuid.UUID               `json:"agent_instance_id" format:"uuid"`
	ProcessID         uuid.UUID               `json:"process_id" format:"uuid"`
	State             string                  `json:"state"`
	ExitCode          *int32                  `json:"exit_code,omitempty"`
	Deadline          *time.Time              `json:"deadline,omitempty" format:"date-time"`
	Error             string                  `json:"error,omitempty"`
	Output            *WorkspaceCommandOutput `json:"output,omitempty"`
	OutputUnavailable bool                    `json:"output_unavailable"`
}

// WorkspaceCommandOutput is an ephemeral agent output snapshot.
type WorkspaceCommandOutput struct {
	Text      string                      `json:"text"`
	Truncated *WorkspaceCommandTruncation `json:"truncated,omitempty"`
}

// WorkspaceCommandTruncation distinguishes source bytes omitted from marker bytes.
type WorkspaceCommandTruncation struct {
	OriginalBytes int    `json:"original_bytes"`
	RetainedBytes int    `json:"retained_bytes"`
	OmittedBytes  int    `json:"omitted_bytes"`
	Strategy      string `json:"strategy"`
}

// StartWorkspaceCommand submits once or recovers the original durable receipt.
func (c *Client) StartWorkspaceCommand(ctx context.Context, org, session uuid.UUID, request StartWorkspaceCommandRequest) (WorkspaceCommand, error) {
	return c.workspaceCommandRequest(ctx, http.MethodPost, fmt.Sprintf("/api/v2/organizations/%s/workspace-executions/%s/commands", org, session), request)
}

// WorkspaceCommand reads current output, waiting independently of execution deadlines.
func (c *Client) WorkspaceCommand(ctx context.Context, org, session, id uuid.UUID, waitMillis int64) (WorkspaceCommand, error) {
	return c.workspaceCommandRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/workspace-executions/%s/commands/%s?wait_ms=%d", org, session, id, waitMillis), nil)
}

// CancelWorkspaceCommand requests cancellation and reports actual acknowledgment.
func (c *Client) CancelWorkspaceCommand(ctx context.Context, org, session, id uuid.UUID, waitMillis int64) (WorkspaceCommand, error) {
	return c.workspaceCommandRequest(ctx, http.MethodPost, fmt.Sprintf("/api/v2/organizations/%s/workspace-executions/%s/commands/%s/cancel?wait_ms=%d", org, session, id, waitMillis), nil)
}

func (c *Client) workspaceCommandRequest(ctx context.Context, method, path string, body any) (WorkspaceCommand, error) {
	res, err := c.Request(ctx, method, path, body)
	if err != nil {
		return WorkspaceCommand{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceCommand{}, ReadBodyAsError(res)
	}
	var result WorkspaceCommand
	err = ReadBodyAsJSON(res, &result)
	return result, err
}
