package agentacp

import (
	"context"

	acp "github.com/coder/acp-go-sdk"
)

// sessionClient delivers updates to its session.
type sessionClient struct {
	client
	session *SessionFSM
}

func (c *sessionClient) SessionUpdate(ctx context.Context, req acp.SessionNotification) error {
	return c.session.ProcessACPUpdate(ctx, ACPUpdate{Notification: &req})
}

//nolint:misspell // ACP names this protocol variant Cancelled.
func (c *sessionClient) RequestPermission(ctx context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	s := c.session
	// ACP requires pending permission requests to return "cancelled" when a
	// prompt turn is canceled. The SDK callback context can remain active, so
	// also check s.permissionCtx before granting permission.
	// https://agentclientprotocol.com/protocol/prompt-turn#cancellation
	//
	// CODAGT-1454: A delayed permission callback can acquire mu after the next
	// turn replaces permissionCtx, allowing an interrupted turn's request. The
	// SDK dispatches callbacks asynchronously without turn identity, so capturing
	// the context before locking cannot reliably bind it to the original turn.
	// We'd need a change in the SDK to resolve this.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.permissionCtx.Err() != nil {
		return acp.RequestPermissionResponse{Outcome: acp.RequestPermissionOutcome{Cancelled: &acp.RequestPermissionOutcomeCancelled{}}}, nil
	}
	return c.client.RequestPermission(ctx, req)
}

var _ acp.Client = (*sessionClient)(nil)
