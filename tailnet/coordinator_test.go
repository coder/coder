package tailnet_test

import (
	"context"
	"io"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/tailnet/test"
	"github.com/coder/coder/v2/testutil"
)

func TestCoordinator(t *testing.T) {
	t.Parallel()
	t.Run("ClientWithoutAgent", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
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
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
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
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
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
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
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
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		coordinator := tailnet.NewCoordinator(logger)
		ctx := testutil.Context(t, testutil.WaitShort)

		test.ReadyForHandshakeTest(ctx, t, coordinator)
	})

	t.Run("AgentAck_NoPermission", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		coordinator := tailnet.NewCoordinator(logger)
		ctx := testutil.Context(t, testutil.WaitShort)

		test.ReadyForHandshakeNoPermissionTest(ctx, t, coordinator)
	})
}

func TestCoordinator_BidirectionalTunnels(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	coordinator := tailnet.NewCoordinator(logger)
	ctx := testutil.Context(t, testutil.WaitShort)
	test.BidirectionalTunnels(ctx, t, coordinator)
}

func TestCoordinator_GracefulDisconnect(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	coordinator := tailnet.NewCoordinator(logger)
	ctx := testutil.Context(t, testutil.WaitShort)
	test.GracefulDisconnectTest(ctx, t, coordinator)
}

func TestCoordinator_Lost(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	coordinator := tailnet.NewCoordinator(logger)
	ctx := testutil.Context(t, testutil.WaitShort)
	test.LostTest(ctx, t, coordinator)
}

func TestCoordinator_MultiAgent_CoordClose(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	coord1 := tailnet.NewCoordinator(logger.Named("coord1"))
	defer coord1.Close()

	ma1 := tailnettest.NewTestMultiAgent(t, coord1)
	defer ma1.Close()

	err := coord1.Close()
	require.NoError(t, err)

	ma1.RequireEventuallyClosed(ctx)
}

func TestInMemoryCoordination(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	clientID := uuid.UUID{1}
	agentID := uuid.UUID{2}
	mCoord := tailnettest.NewMockCoordinator(gomock.NewController(t))
	fConn := &fakeCoordinatee{}

	reqs := make(chan *proto.CoordinateRequest, 100)
	resps := make(chan *proto.CoordinateResponse, 100)
	mCoord.EXPECT().Coordinate(gomock.Any(), clientID, gomock.Any(), tailnet.ClientCoordinateeAuth{agentID}).
		Times(1).Return(reqs, resps)

	uut := tailnet.NewInMemoryCoordination(ctx, logger, clientID, agentID, mCoord, fConn)
	defer uut.Close(ctx)

	coordinationTest(ctx, t, uut, fConn, reqs, resps, agentID)

	select {
	case err := <-uut.Error():
		require.NoError(t, err)
	default:
		// OK!
	}
}

func TestRemoteCoordination(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	clientID := uuid.UUID{1}
	agentID := uuid.UUID{2}
	mCoord := tailnettest.NewMockCoordinator(gomock.NewController(t))
	fConn := &fakeCoordinatee{}

	reqs := make(chan *proto.CoordinateRequest, 100)
	resps := make(chan *proto.CoordinateResponse, 100)
	mCoord.EXPECT().Coordinate(gomock.Any(), clientID, gomock.Any(), tailnet.ClientCoordinateeAuth{agentID}).
		Times(1).Return(reqs, resps)

	var coord tailnet.Coordinator = mCoord
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coord)
	svc, err := tailnet.NewClientService(tailnet.ClientServiceOptions{
		Logger:                  logger.Named("svc"),
		CoordPtr:                &coordPtr,
		DERPMapUpdateFrequency:  time.Hour,
		DERPMapFn:               func() *tailcfg.DERPMap { panic("not implemented") },
		NetworkTelemetryHandler: func(batch []*proto.TelemetryEvent) { panic("not implemented") },
		ResumeTokenProvider:     tailnet.NewInsecureTestResumeTokenProvider(),
	})
	require.NoError(t, err)
	sC, cC := net.Pipe()

	serveErr := make(chan error, 1)
	go func() {
		err := svc.ServeClient(ctx, proto.CurrentVersion.String(), sC, clientID, agentID)
		serveErr <- err
	}()

	client, err := tailnet.NewDRPCClient(cC, logger)
	require.NoError(t, err)
	protocol, err := client.Coordinate(ctx)
	require.NoError(t, err)

	uut := tailnet.NewRemoteCoordination(logger.Named("coordination"), protocol, fConn, agentID)
	defer uut.Close(ctx)

	coordinationTest(ctx, t, uut, fConn, reqs, resps, agentID)

	// Recv loop should be terminated by the server hanging up after Disconnect
	err = testutil.RequireRecvCtx(ctx, t, uut.Error())
	require.ErrorIs(t, err, io.EOF)
}

func TestRemoteCoordination_SendsReadyForHandshake(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	clientID := uuid.UUID{1}
	agentID := uuid.UUID{2}
	mCoord := tailnettest.NewMockCoordinator(gomock.NewController(t))
	fConn := &fakeCoordinatee{}

	reqs := make(chan *proto.CoordinateRequest, 100)
	resps := make(chan *proto.CoordinateResponse, 100)
	mCoord.EXPECT().Coordinate(gomock.Any(), clientID, gomock.Any(), tailnet.ClientCoordinateeAuth{agentID}).
		Times(1).Return(reqs, resps)

	var coord tailnet.Coordinator = mCoord
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coord)
	svc, err := tailnet.NewClientService(tailnet.ClientServiceOptions{
		Logger:                  logger.Named("svc"),
		CoordPtr:                &coordPtr,
		DERPMapUpdateFrequency:  time.Hour,
		DERPMapFn:               func() *tailcfg.DERPMap { panic("not implemented") },
		NetworkTelemetryHandler: func(batch []*proto.TelemetryEvent) { panic("not implemented") },
		ResumeTokenProvider:     tailnet.NewInsecureTestResumeTokenProvider(),
	})
	require.NoError(t, err)
	sC, cC := net.Pipe()

	serveErr := make(chan error, 1)
	go func() {
		err := svc.ServeClient(ctx, proto.CurrentVersion.String(), sC, clientID, agentID)
		serveErr <- err
	}()

	client, err := tailnet.NewDRPCClient(cC, logger)
	require.NoError(t, err)
	protocol, err := client.Coordinate(ctx)
	require.NoError(t, err)

	uut := tailnet.NewRemoteCoordination(logger.Named("coordination"), protocol, fConn, uuid.UUID{})
	defer uut.Close(ctx)

	nk, err := key.NewNode().Public().MarshalBinary()
	require.NoError(t, err)
	dk, err := key.NewDisco().Public().MarshalText()
	require.NoError(t, err)
	testutil.RequireSendCtx(ctx, t, resps, &proto.CoordinateResponse{
		PeerUpdates: []*proto.CoordinateResponse_PeerUpdate{{
			Id:   clientID[:],
			Kind: proto.CoordinateResponse_PeerUpdate_NODE,
			Node: &proto.Node{
				Id:    3,
				Key:   nk,
				Disco: string(dk),
			},
		}},
	})

	rfh := testutil.RequireRecvCtx(ctx, t, reqs)
	require.NotNil(t, rfh.ReadyForHandshake)
	require.Len(t, rfh.ReadyForHandshake, 1)
	require.Equal(t, clientID[:], rfh.ReadyForHandshake[0].Id)

	go uut.Close(ctx)
	dis := testutil.RequireRecvCtx(ctx, t, reqs)
	require.NotNil(t, dis)
	require.NotNil(t, dis.Disconnect)
	close(resps)

	// Recv loop should be terminated by the server hanging up after Disconnect
	err = testutil.RequireRecvCtx(ctx, t, uut.Error())
	require.ErrorIs(t, err, io.EOF)
}

// coordinationTest tests that a coordination behaves correctly
func coordinationTest(
	ctx context.Context, t *testing.T,
	uut tailnet.Coordination, fConn *fakeCoordinatee,
	reqs chan *proto.CoordinateRequest, resps chan *proto.CoordinateResponse,
	agentID uuid.UUID,
) {
	// It should add the tunnel, since we configured as a client
	req := testutil.RequireRecvCtx(ctx, t, reqs)
	require.Equal(t, agentID[:], req.GetAddTunnel().GetId())

	// when we call the callback, it should send a node update
	require.NotNil(t, fConn.callback)
	fConn.callback(&tailnet.Node{PreferredDERP: 1})

	req = testutil.RequireRecvCtx(ctx, t, reqs)
	require.Equal(t, int32(1), req.GetUpdateSelf().GetNode().GetPreferredDerp())

	// When we send a peer update, it should update the coordinatee
	nk, err := key.NewNode().Public().MarshalBinary()
	require.NoError(t, err)
	dk, err := key.NewDisco().Public().MarshalText()
	require.NoError(t, err)
	updates := []*proto.CoordinateResponse_PeerUpdate{
		{
			Id:   agentID[:],
			Kind: proto.CoordinateResponse_PeerUpdate_NODE,
			Node: &proto.Node{
				Id:    2,
				Key:   nk,
				Disco: string(dk),
			},
		},
	}
	testutil.RequireSendCtx(ctx, t, resps, &proto.CoordinateResponse{PeerUpdates: updates})
	require.Eventually(t, func() bool {
		fConn.Lock()
		defer fConn.Unlock()
		return len(fConn.updates) > 0
	}, testutil.WaitShort, testutil.IntervalFast)
	require.Len(t, fConn.updates[0], 1)
	require.Equal(t, agentID[:], fConn.updates[0][0].Id)

	errCh := make(chan error, 1)
	go func() {
		errCh <- uut.Close(ctx)
	}()

	// When we close, it should gracefully disconnect
	req = testutil.RequireRecvCtx(ctx, t, reqs)
	require.NotNil(t, req.Disconnect)
	close(resps)

	err = testutil.RequireRecvCtx(ctx, t, errCh)
	require.NoError(t, err)

	// It should set all peers lost on the coordinatee
	require.Equal(t, 1, fConn.setAllPeersLostCalls)
}

type fakeCoordinatee struct {
	sync.Mutex
	callback             func(*tailnet.Node)
	updates              [][]*proto.CoordinateResponse_PeerUpdate
	setAllPeersLostCalls int
	tunnelDestinations   map[uuid.UUID]struct{}
}

func (f *fakeCoordinatee) UpdatePeers(updates []*proto.CoordinateResponse_PeerUpdate) error {
	f.Lock()
	defer f.Unlock()
	f.updates = append(f.updates, updates)
	return nil
}

func (f *fakeCoordinatee) SetAllPeersLost() {
	f.Lock()
	defer f.Unlock()
	f.setAllPeersLostCalls++
}

func (f *fakeCoordinatee) SetTunnelDestination(id uuid.UUID) {
	f.Lock()
	defer f.Unlock()

	if f.tunnelDestinations == nil {
		f.tunnelDestinations = map[uuid.UUID]struct{}{}
	}
	f.tunnelDestinations[id] = struct{}{}
}

func (f *fakeCoordinatee) SetNodeCallback(callback func(*tailnet.Node)) {
	f.Lock()
	defer f.Unlock()
	f.callback = callback
}
