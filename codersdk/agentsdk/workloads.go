package agentsdk

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk"
)

// Identity returns the identity coderd sees for this daemon.
func (c *Client) Identity(ctx context.Context) (codersdk.WorkloadIdentity, error) {
	return c.SDK.WorkloadIdentity(ctx)
}

// ReportWorkloadExit tells coderd that a workload this daemon ran is over.
// coderd records how it ended and revokes its identity.
func (c *Client) ReportWorkloadExit(ctx context.Context, id uuid.UUID, exit codersdk.AgentWorkloadExit) error {
	res, err := c.SDK.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/me/workloads/"+id.String()+"/exit", exit)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return codersdk.ReadBodyAsError(res)
	}
	return nil
}
