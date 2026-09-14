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

func TestCoordinatorInvalidRequests(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	coordinator := tailnet.NewCoordinator(slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}))
	t.Cleanup(func() { require.NoError(t, coordinator.Close()) })
	test.InvalidCoordinateRequestTest(ctx, t, coordinator)
}

func TestCoordinatorRequestPanic(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	coordinator := tailnet.NewCoordinator(slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}))
	t.Cleanup(func() { require.NoError(t, coordinator.Close()) })
	test.CoordinateRequestPanicTest(ctx, t, coordinator)
}
