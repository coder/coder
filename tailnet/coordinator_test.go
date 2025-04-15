package tailnet_test

import (
	"context"
	"net/netip"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/tailnet/test"
	"github.com/coder/coder/v2/testutil"
)

func TestCoordinator(t *testing.T) {
	t.Parallel()
	t.Run("ClientWithoutAgent", func(t *testing.T) {
		t.Parallel()
		logger := testutil.Logger(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		coordinator := tailnet.NewCoordinator(logger)
		defer func() {
			err := coordinator.Close()
			require.NoError(t, err)
		}()

		client := test.NewClient(ctx, t, coordinator, "client", uuid.New())
		defer client.Close(ctx)
		client.UpdateNode(&proto.Node{
			Addresses:     []string{tailnet.TailscaleServicePrefix.RandomPrefix().String()},
			PreferredDerp: 10,
		})
		require.Eventually(t, func() bool {
			return coordinator.Node(client.ID) != nil
		}, testutil.WaitShort, testutil.IntervalFast)
	})

	t.Run("ClientWithoutAgent_InvalidIPBits", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctx := testutil.Context(t, testutil.WaitShort)
		coordinator := tailnet.NewCoordinator(logger)
		defer func() {
			err := coordinator.Close()
			require.NoError(t, err)
		}()

		client := test.NewClient(ctx, t, coordinator, "client", uuid.New())
		defer client.Close(ctx)

		client.UpdateNode(&proto.Node{
			Addresses: []string{
				netip.PrefixFrom(tailnet.TailscaleServicePrefix.RandomAddr(), 64).String(),
			},
			PreferredDerp: 10,
		})
		client.AssertEventuallyResponsesClosed()
	})

	t.Run("AgentWithoutClients", func(t *testing.T) {
		t.Parallel()
		logger := testutil.Logger(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		coordinator := tailnet.NewCoordinator(logger)
		defer func() {
			err := coordinator.Close()
			require.NoError(t, err)
		}()

		agent := test.NewAgent(ctx, t, coordinator, "agent")
		defer agent.Close(ctx)
		agent.UpdateNode(&proto.Node{
			Addresses: []string{
				tailnet.TailscaleServicePrefix.PrefixFromUUID(agent.ID).String(),
			},
			PreferredDerp: 10,
		})
		require.Eventually(t, func() bool {
			return coordinator.Node(agent.ID) != nil
		}, testutil.WaitShort, testutil.IntervalFast)
	})

	t.Run("AgentWithoutClients_InvalidIP", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctx := testutil.Context(t, testutil.WaitShort)
		coordinator := tailnet.NewCoordinator(logger)
		defer func() {
			err := coordinator.Close()
			require.NoError(t, err)
		}()
		agent := test.NewAgent(ctx, t, coordinator, "agent")
		defer agent.Close(ctx)
		agent.UpdateNode(&proto.Node{
			Addresses: []string{
				tailnet.TailscaleServicePrefix.RandomPrefix().String(),
			},
			PreferredDerp: 10,
		})
		agent.AssertEventuallyResponsesClosed()
	})

	t.Run("AgentWithoutClients_InvalidBits", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctx := testutil.Context(t, testutil.WaitShort)
		coordinator := tailnet.NewCoordinator(logger)
		defer func() {
			err := coordinator.Close()
			require.NoError(t, err)
		}()
		agent := test.NewAgent(ctx, t, coordinator, "agent")
		defer agent.Close(ctx)
		agent.UpdateNode(&proto.Node{
			Addresses: []string{
				netip.PrefixFrom(
					tailnet.TailscaleServicePrefix.AddrFromUUID(agent.ID), 64).String(),
			},
			PreferredDerp: 10,
		})
		agent.AssertEventuallyResponsesClosed()
	})

	t.Run("AgentWithClient", func(t *testing.T) {
		t.Parallel()
		logger := testutil.Logger(t)
		coordinator := tailnet.NewCoordinator(logger)
		defer func() {
			err := coordinator.Close()
			require.NoError(t, err)
		}()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		agent := test.NewAgent(ctx, t, coordinator, "agent")
		defer agent.Close(ctx)
		agent.UpdateDERP(1)
		require.Eventually(t, func() bool {
			return coordinator.Node(agent.ID) != nil
		}, testutil.WaitShort, testutil.IntervalFast)

		client := test.NewClient(ctx, t, coordinator, "client", agent.ID)
		defer client.Close(ctx)
		client.AssertEventuallyHasDERP(agent.ID, 1)

		client.UpdateDERP(2)
		agent.AssertEventuallyHasDERP(client.ID, 2)

		// Ensure an update to the agent node reaches the client!
		agent.UpdateDERP(3)
		client.AssertEventuallyHasDERP(agent.ID, 3)

		// Close the agent so a new one can connect.
		agent.Close(ctx)

		// Create a new agent connection. This is to simulate a reconnect!
		agent = test.NewPeer(ctx, t, coordinator, "agent", test.WithID(agent.ID))
		defer agent.Close(ctx)
		// Ensure the agent gets the existing client node immediately!
		agent.AssertEventuallyHasDERP(client.ID, 2)
	})

	t.Run("AgentDoubleConnect", func(t *testing.T) {
		t.Parallel()
		logger := testutil.Logger(t)
		coordinator := tailnet.NewCoordinator(logger)
		ctx := testutil.Context(t, testutil.WaitShort)

		agentID := uuid.New()
		agent1 := test.NewPeer(ctx, t, coordinator, "agent1", test.WithID(agentID))
		defer agent1.Close(ctx)
		agent1.UpdateDERP(1)
		require.Eventually(t, func() bool {
			return coordinator.Node(agentID) != nil
		}, testutil.WaitShort, testutil.IntervalFast)

		client := test.NewPeer(ctx, t, coordinator, "client")
		defer client.Close(ctx)
		client.AddTunnel(agentID)
		client.AssertEventuallyHasDERP(agent1.ID, 1)

		client.UpdateDERP(2)
		agent1.AssertEventuallyHasDERP(client.ID, 2)

		// Ensure an update to the agent node reaches the client!
		agent1.UpdateDERP(3)
		client.AssertEventuallyHasDERP(agent1.ID, 3)

		// Create a new agent connection without disconnecting the old one.
		agent2 := test.NewPeer(ctx, t, coordinator, "agent2", test.WithID(agentID))
		defer agent2.Close(ctx)

		// Ensure the existing client node gets sent immediately!
		agent2.AssertEventuallyHasDERP(client.ID, 2)

		// This original agent channels should've been closed forcefully.
		agent1.AssertEventuallyResponsesClosed()
	})

	t.Run("AgentAck", func(t *testing.T) {
		t.Parallel()
		logger := testutil.Logger(t)
		coordinator := tailnet.NewCoordinator(logger)
		ctx := testutil.Context(t, testutil.WaitShort)

		test.ReadyForHandshakeTest(ctx, t, coordinator)
	})

	t.Run("AgentAck_NoPermission", func(t *testing.T) {
		t.Parallel()
		logger := testutil.Logger(t)
		coordinator := tailnet.NewCoordinator(logger)
		ctx := testutil.Context(t, testutil.WaitShort)

		test.ReadyForHandshakeNoPermissionTest(ctx, t, coordinator)
	})
}

func TestCoordinator_BidirectionalTunnels(t *testing.T) {
	t.Parallel()
	logger := testutil.Logger(t)
	coordinator := tailnet.NewCoordinator(logger)
	ctx := testutil.Context(t, testutil.WaitShort)
	test.BidirectionalTunnels(ctx, t, coordinator)
}

func TestCoordinator_GracefulDisconnect(t *testing.T) {
	t.Parallel()
	logger := testutil.Logger(t)
	coordinator := tailnet.NewCoordinator(logger)
	ctx := testutil.Context(t, testutil.WaitShort)
	test.GracefulDisconnectTest(ctx, t, coordinator)
}

func TestCoordinator_Lost(t *testing.T) {
	t.Parallel()
	logger := testutil.Logger(t)
	coordinator := tailnet.NewCoordinator(logger)
	ctx := testutil.Context(t, testutil.WaitShort)
	test.LostTest(ctx, t, coordinator)
}

// TestCoordinatorPropogatedPeerContext tests that the context for a specific peer
// is propogated through to the `Authorize“ method of the coordinatee auth
func TestCoordinatorPropogatedPeerContext(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)

	peerCtx := context.WithValue(ctx, test.FakeSubjectKey{}, struct{}{})
	peerCtx, peerCtxCancel := context.WithCancel(peerCtx)
	peerID := uuid.UUID{0x01}
	agentID := uuid.UUID{0x02}

	c1 := tailnet.NewCoordinator(logger)
	t.Cleanup(func() {
		err := c1.Close()
		require.NoError(t, err)
	})

	ch := make(chan struct{})
	auth := test.FakeCoordinateeAuth{
		Chan: ch,
	}

	reqs, _ := c1.Coordinate(peerCtx, peerID, "peer1", auth)

	testutil.RequireSend(ctx, t, reqs, &proto.CoordinateRequest{AddTunnel: &proto.CoordinateRequest_Tunnel{Id: tailnet.UUIDToByteSlice(agentID)}})
	_ = testutil.TryReceive(ctx, t, ch)
	// If we don't cancel the context, the coordinator close will wait until the
	// peer request loop finishes, which will be after the timeout
	peerCtxCancel()
}
