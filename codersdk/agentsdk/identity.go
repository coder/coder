package agentsdk

import (
	"context"

	"github.com/coder/coder/v2/codersdk"
)

// Identity returns the identity coderd sees for this daemon.
func (c *Client) Identity(ctx context.Context) (codersdk.WorkloadIdentity, error) {
	return c.SDK.WorkloadIdentity(ctx)
}
