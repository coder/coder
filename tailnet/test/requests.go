package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
)

// InvalidCoordinateRequestTest checks that malformed requests are rejected
// before authorization and that a valid node bundled with a malformed entry
// is not published to tunnel peers.
func InvalidCoordinateRequestTest(ctx context.Context, t *testing.T, coordinator tailnet.CoordinatorV2) {
	const (
		baselineDERP  = 31001
		malformedDERP = 31002
	)
	for _, tc := range []struct {
		name             string
		req              *proto.CoordinateRequest
		err              string
		verifyNoMutation bool
	}{
		{name: "NilRequest", err: "coordinate request is required"},
		{
			name: "NilNode",
			req:  &proto.CoordinateRequest{UpdateSelf: &proto.CoordinateRequest_UpdateSelf{}},
			err:  "update_self node is required",
		},
		{
			name: "NilReadyForHandshake",
			req:  &proto.CoordinateRequest{ReadyForHandshake: []*proto.CoordinateRequest_ReadyForHandshake{nil}},
			err:  "ready_for_handshake entries must not be nil",
		},
		{
			name: "NodeAndNilReadyForHandshake",
			req: &proto.CoordinateRequest{
				UpdateSelf: &proto.CoordinateRequest_UpdateSelf{Node: &proto.Node{
					PreferredDerp: malformedDERP,
				}},
				ReadyForHandshake: []*proto.CoordinateRequest_ReadyForHandshake{nil},
			},
			err:              "ready_for_handshake entries must not be nil",
			verifyNoMutation: true,
		},
	} {
		// The cases share the caller's ctx and coordinator, so they run
		// sequentially. Parallel subtests would pause until the caller
		// returns and, under -parallel=1, could resume after that ctx has
		// already expired.
		t.Run(tc.name, func(t *testing.T) {
			var auth tailnet.CoordinateeAuth = CoordinateeAuthFunc(func(context.Context, *proto.CoordinateRequest) error {
				panic("authorization must not see malformed requests")
			})
			if tc.verifyNoMutation {
				auth = tailnet.SingleTailnetCoordinateeAuth{}
			}
			peer := NewPeer(ctx, t, coordinator, tc.name, WithAuth(auth))
			defer peer.Close(ctx)
			var observer *Peer
			if tc.verifyNoMutation {
				observer = NewPeer(ctx, t, coordinator, tc.name+"Observer")
				defer observer.Close(ctx)
				observer.AddTunnel(peer.ID)
				peer.UpdateDERP(baselineDERP)
				observer.AssertEventuallyHasDERP(peer.ID, baselineDERP)
			}
			require.NoError(t, tailnet.SendCtx(ctx, peer.reqs, tc.req))
			peer.AssertEventuallyResponsesClosed(tc.err)
			if tc.verifyNoMutation {
				observer.AssertEventuallyLost(peer.ID)
				for _, update := range observer.peerUpdates[peer.ID] {
					require.NotEqual(t, int32(malformedDERP), update.GetNode().GetPreferredDerp())
				}
			}
		})
	}
}

// CoordinateRequestPanicTest checks that a failed connection is withdrawn while
// other connections continue exchanging updates.
func CoordinateRequestPanicTest(ctx context.Context, t *testing.T, coordinator tailnet.CoordinatorV2) {
	broken := NewPeer(ctx, t, coordinator, "broken", WithAuth(CoordinateeAuthFunc(func(_ context.Context, req *proto.CoordinateRequest) error {
		if req.Disconnect != nil {
			panic("private panic detail")
		}
		return nil
	})))
	defer broken.Close(ctx)
	healthy := NewPeer(ctx, t, coordinator, "healthy")
	defer healthy.Close(ctx)
	other := NewPeer(ctx, t, coordinator, "other")
	defer other.Close(ctx)
	healthy.AddTunnel(broken.ID)
	healthy.AddTunnel(other.ID)
	broken.UpdateDERP(1)
	healthy.UpdateDERP(2)
	other.UpdateDERP(3)
	healthy.AssertEventuallyHasDERP(broken.ID, 1)
	healthy.AssertEventuallyHasDERP(other.ID, 3)

	broken.Disconnect()
	broken.AssertEventuallyResponsesClosed(tailnet.CloseErrInternal)
	healthy.AssertEventuallyLost(broken.ID)

	// Empty messages are valid no-ops, and an empty Node is still a Node.
	require.NoError(t, tailnet.SendCtx(ctx, other.reqs, &proto.CoordinateRequest{}))
	require.NoError(t, tailnet.SendCtx(ctx, other.reqs, &proto.CoordinateRequest{
		UpdateSelf: &proto.CoordinateRequest_UpdateSelf{Node: &proto.Node{}},
	}))
	other.UpdateDERP(4)
	healthy.AssertEventuallyHasDERP(other.ID, 4)
}
