package test

import (
	"context"
	"testing"

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
	p2.AssertEventuallyResponsesClosed()
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
