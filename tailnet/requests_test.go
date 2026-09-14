package tailnet_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/tailnet/test"
	"github.com/coder/coder/v2/testutil"
)

func TestCoordinateeAuthNilNode(t *testing.T) {
	t.Parallel()
	for name, auth := range map[string]tailnet.CoordinateeAuth{
		"Agent":      tailnet.AgentCoordinateeAuth{ID: uuid.New()},
		"Client":     tailnet.ClientCoordinateeAuth{AgentID: uuid.New()},
		"ClientUser": tailnet.ClientUserCoordinateeAuth{},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			require.NotPanics(t, func() {
				require.EqualError(t, auth.Authorize(ctx, &proto.CoordinateRequest{
					UpdateSelf: &proto.CoordinateRequest_UpdateSelf{},
				}), "update_self node is required")
			})
		})
	}
}

func TestCoordinator_InvalidRequests(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	coordinator := tailnet.NewCoordinator(slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}))
	t.Cleanup(func() { require.NoError(t, coordinator.Close()) })
	test.InvalidCoordinateRequestTest(ctx, t, coordinator)
}

func TestCoordinator_RequestPanic(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	coordinator := tailnet.NewCoordinator(slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}))
	t.Cleanup(func() { require.NoError(t, coordinator.Close()) })
	test.CoordinateRequestPanicTest(ctx, t, coordinator)
}

func TestValidateCoordinateRequest(t *testing.T) {
	t.Parallel()
	validRFH := &proto.CoordinateRequest_ReadyForHandshake{Id: tailnet.UUIDToByteSlice(uuid.New())}
	for _, tc := range []struct {
		name string
		req  *proto.CoordinateRequest
		err  string // empty means the request is accepted
	}{
		{name: "NilRequest", err: "coordinate request is required"},
		{name: "EmptyRequest", req: &proto.CoordinateRequest{}},
		{
			name: "NilNode",
			req:  &proto.CoordinateRequest{UpdateSelf: &proto.CoordinateRequest_UpdateSelf{}},
			err:  "update_self node is required",
		},
		{
			name: "EmptyNode",
			req:  &proto.CoordinateRequest{UpdateSelf: &proto.CoordinateRequest_UpdateSelf{Node: &proto.Node{}}},
		},
		{name: "NilReadyForHandshake", req: &proto.CoordinateRequest{ReadyForHandshake: nil}},
		{
			name: "EmptyReadyForHandshake",
			req:  &proto.CoordinateRequest{ReadyForHandshake: []*proto.CoordinateRequest_ReadyForHandshake{}},
		},
		{
			name: "ValidReadyForHandshake",
			req:  &proto.CoordinateRequest{ReadyForHandshake: []*proto.CoordinateRequest_ReadyForHandshake{validRFH}},
		},
		{
			name: "NilReadyForHandshakeEntry",
			req:  &proto.CoordinateRequest{ReadyForHandshake: []*proto.CoordinateRequest_ReadyForHandshake{nil}},
			err:  "ready_for_handshake entry is required",
		},
		{
			name: "NilReadyForHandshakeAfterValid",
			req:  &proto.CoordinateRequest{ReadyForHandshake: []*proto.CoordinateRequest_ReadyForHandshake{validRFH, nil}},
			err:  "ready_for_handshake entry is required",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tailnet.ValidateCoordinateRequest(tc.req)
			if tc.err == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tc.err)
		})
	}
}
