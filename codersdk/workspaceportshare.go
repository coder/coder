package codersdk

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

const (
	WorkspaceAgentPortShareLevelOwner         WorkspaceAgentPortShareLevel = 0
	WorkspaceAgentPortShareLevelAuthenticated WorkspaceAgentPortShareLevel = 1
	WorkspaceAgentPortShareLevelPublic        WorkspaceAgentPortShareLevel = 2
)

type (
	WorkspaceAgentPortShareLevel         int
	UpdateWorkspaceAgentPortShareRequest struct {
		AgentName  string                       `json:"agent_name"`
		Port       int32                        `json:"port"`
		ShareLevel WorkspaceAgentPortShareLevel `json:"share_level"`
	}
)

func (c *Client) UpdateWorkspaceAgentPortShare(ctx context.Context, workspaceID uuid.UUID, req UpdateWorkspaceAgentPortShareRequest) error {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/workspaces/%s/port-share", workspaceID), req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return nil
}
