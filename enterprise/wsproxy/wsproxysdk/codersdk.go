package wsproxysdk

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/codersdk"
)

func (c *Client) DialWorkspaceAgent(ctx context.Context, agentID uuid.UUID, options *codersdk.DialWorkspaceAgentOptions) (agentConn *codersdk.WorkspaceAgentConn, err error) {
	return c.CoderSDKClient.DialWorkspaceAgent(ctx, agentID, options)
}
