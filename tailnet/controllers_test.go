package tailnet_test

import (
	"context"
	"io"
	"net"
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
	"github.com/coder/coder/v2/testutil"
)

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

	ctrl := tailnet.NewSingleDestController(logger, fConn, agentID)
	uut := ctrl.New(tailnet.NewInMemoryCoordinatorClient(logger, clientID, agentID, mCoord))
	defer uut.Close(ctx)

	coordinationTest(ctx, t, uut, fConn, reqs, resps, agentID)

	// Recv loop should be terminated by the server hanging up after Disconnect
	err := testutil.RequireRecvCtx(ctx, t, uut.Wait())
	require.ErrorIs(t, err, io.EOF)
}

func TestSingleDestController(t *testing.T) {
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
		err := svc.ServeClient(ctx, proto.CurrentVersion.String(), sC, tailnet.StreamID{
			Name: "client",
			ID:   clientID,
			Auth: tailnet.ClientCoordinateeAuth{
				AgentID: agentID,
			},
		})
		serveErr <- err
	}()

	client, err := tailnet.NewDRPCClient(cC, logger)
	require.NoError(t, err)
	protocol, err := client.Coordinate(ctx)
	require.NoError(t, err)

	ctrl := tailnet.NewSingleDestController(logger.Named("coordination"), fConn, agentID)
	uut := ctrl.New(protocol)
	defer uut.Close(ctx)

	coordinationTest(ctx, t, uut, fConn, reqs, resps, agentID)

	// Recv loop should be terminated by the server hanging up after Disconnect
	err = testutil.RequireRecvCtx(ctx, t, uut.Wait())
	require.ErrorIs(t, err, io.EOF)
}

func TestAgentCoordinationController_SendsReadyForHandshake(t *testing.T) {
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
		err := svc.ServeClient(ctx, proto.CurrentVersion.String(), sC, tailnet.StreamID{
			Name: "client",
			ID:   clientID,
			Auth: tailnet.ClientCoordinateeAuth{
				AgentID: agentID,
			},
		})
		serveErr <- err
	}()

	client, err := tailnet.NewDRPCClient(cC, logger)
	require.NoError(t, err)
	protocol, err := client.Coordinate(ctx)
	require.NoError(t, err)

	ctrl := tailnet.NewAgentCoordinationController(logger.Named("coordination"), fConn)
	uut := ctrl.New(protocol)
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
	err = testutil.RequireRecvCtx(ctx, t, uut.Wait())
	require.ErrorIs(t, err, io.EOF)
}

// coordinationTest tests that a coordination behaves correctly
func coordinationTest(
	ctx context.Context, t *testing.T,
	uut tailnet.CloserWaiter, fConn *fakeCoordinatee,
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
