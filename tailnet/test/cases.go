package test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/tailnet"
)

func GracefulDisconnectTest(ctx context.Context, t *testing.T, coordinator tailnet.CoordinatorV2) {
	p1 := NewPeer(ctx, t, coordinator, "p1")
	defer p1.Close(ctx)
	p2 := NewPeer(ctx, t, coordinator, "p2")
	defer p2.Close(ctx)
	p1.AddTunnel(p2.ID)
	p1.UpdateDERP(1)
	p2.UpdateDERP(2)

	p1.AssertEventuallyHasDERP(p2.ID, 2)
	p2.AssertEventuallyHasDERP(p1.ID, 1)

	p2.Disconnect()
	p1.AssertEventuallyDisconnected(p2.ID)
	p2.AssertEventuallyResponsesClosed("")
}

func LostTest(ctx context.Context, t *testing.T, coordinator tailnet.CoordinatorV2) {
	p1 := NewPeer(ctx, t, coordinator, "p1")
	defer p1.Close(ctx)
	p2 := NewPeer(ctx, t, coordinator, "p2")
	defer p2.Close(ctx)
	p1.AddTunnel(p2.ID)
	p1.UpdateDERP(1)
	p2.UpdateDERP(2)

	p1.AssertEventuallyHasDERP(p2.ID, 2)
	p2.AssertEventuallyHasDERP(p1.ID, 1)

	p2.Close(ctx)
	p1.AssertEventuallyLost(p2.ID)
}

func BidirectionalTunnels(ctx context.Context, t *testing.T, coordinator tailnet.CoordinatorV2) {
	p1 := NewPeer(ctx, t, coordinator, "p1")
	defer p1.Close(ctx)
	p2 := NewPeer(ctx, t, coordinator, "p2")
	defer p2.Close(ctx)
	p1.AddTunnel(p2.ID)
	p2.AddTunnel(p1.ID)
	p1.UpdateDERP(1)
	p2.UpdateDERP(2)

	p1.AssertEventuallyHasDERP(p2.ID, 2)
	p2.AssertEventuallyHasDERP(p1.ID, 1)
}

func ReadyForHandshakeTest(ctx context.Context, t *testing.T, coordinator tailnet.CoordinatorV2) {
	p1 := NewPeer(ctx, t, coordinator, "p1")
	defer p1.Close(ctx)
	p2 := NewPeer(ctx, t, coordinator, "p2")
	defer p2.Close(ctx)
	p1.AddTunnel(p2.ID)
	p2.AddTunnel(p1.ID)
	p1.UpdateDERP(1)
	p2.UpdateDERP(2)

	p1.AssertEventuallyHasDERP(p2.ID, 2)
	p2.AssertEventuallyHasDERP(p1.ID, 1)
	p2.ReadyForHandshake(p1.ID)
	p1.AssertEventuallyReadyForHandshake(p2.ID)
}

func ReadyForHandshakeNoPermissionTest(ctx context.Context, t *testing.T, coordinator tailnet.CoordinatorV2) {
	p1 := NewPeer(ctx, t, coordinator, "p1")
	defer p1.Close(ctx)
	p2 := NewPeer(ctx, t, coordinator, "p2")
	defer p2.Close(ctx)
	p1.UpdateDERP(1)
	p2.UpdateDERP(2)

	p2.ReadyForHandshake(p1.ID)
	p2.AssertEventuallyGetsError(fmt.Sprintf("you do not share a tunnel with %q", p1.ID.String()))
}

type policyRefusingAuthorizer struct {
	refused uuid.UUID
}

func (a policyRefusingAuthorizer) AuthorizeTunnel(_ context.Context, agentID uuid.UUID) error {
	if agentID == a.refused {
		return tailnet.TunnelPolicyError{Message: "refused by policy"}
	}
	return nil
}

// TunnelPolicyRefusalTest checks that a policy refusal of one tunnel leaves
// the peer connected and able to reach other destinations.
func TunnelPolicyRefusalTest(ctx context.Context, t *testing.T, coordinator tailnet.CoordinatorV2) {
	refused := NewPeer(ctx, t, coordinator, "refused")
	defer refused.Close(ctx)
	allowed := NewPeer(ctx, t, coordinator, "allowed")
	defer allowed.Close(ctx)
	allowed.UpdateDERP(2)
	client := NewPeer(ctx, t, coordinator, "client", WithAuth(tailnet.ClientUserCoordinateeAuth{
		Auth: policyRefusingAuthorizer{refused: refused.ID},
	}))
	defer client.Close(ctx)

	client.AddTunnel(refused.ID)
	client.AssertEventuallyGetsError(tailnet.TunnelRefusedError + ": refused by policy")
	client.AddTunnel(allowed.ID)
	client.AssertEventuallyHasDERP(allowed.ID, 2)
}
