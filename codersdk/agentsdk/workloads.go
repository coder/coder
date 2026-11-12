package agentsdk

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk"
)

// Identity returns the identity coderd sees for this daemon.
func (c *Client) Identity(ctx context.Context) (codersdk.WorkloadIdentity, error) {
	res, err := c.SDK.Request(ctx, http.MethodGet, "/api/v2/workspaceagents/me/identity", nil)
	if err != nil {
		return codersdk.WorkloadIdentity{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return codersdk.WorkloadIdentity{}, codersdk.ReadBodyAsError(res)
	}
	var identity codersdk.WorkloadIdentity
	return identity, codersdk.ReadBodyAsJSON(res, &identity)
}

// pendingWorkloadsWait is how long coderd is asked to hold a request for
// pending workloads when there is none.
const pendingWorkloadsWait = 25 * time.Second

// PendingWorkloads returns the workloads coderd has given this daemon and
// that the daemon has not started. When there is none, coderd holds the
// request until one arrives or a while has passed.
func (c *Client) PendingWorkloads(ctx context.Context) ([]codersdk.AgentWorkloadSpec, error) {
	// A daemon that waits to be claimed is nobody yet and has been given
	// nothing. Finding out who it is can take as long as the wait for a
	// claim, which must not count against the request that follows.
	if c.GetSessionToken() == "" {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
			return nil, nil
		}
	}
	ctx, cancel := context.WithTimeout(ctx, pendingWorkloadsWait+15*time.Second)
	defer cancel()
	res, err := c.SDK.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/workspaceagents/me/workloads/pending?wait_seconds=%d", int(pendingWorkloadsWait.Seconds())), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, codersdk.ReadBodyAsError(res)
	}
	var specs []codersdk.AgentWorkloadSpec
	return specs, codersdk.ReadBodyAsJSON(res, &specs)
}

// ReportWorkloadStart tells coderd whether a workload it gave this daemon
// runs.
func (c *Client) ReportWorkloadStart(ctx context.Context, id uuid.UUID, start codersdk.AgentWorkloadStart) error {
	res, err := c.SDK.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/me/workloads/"+id.String()+"/started", start)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return codersdk.ReadBodyAsError(res)
	}
	return nil
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
