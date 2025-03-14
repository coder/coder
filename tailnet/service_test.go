package tailnet_test

import (
	"context"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestClientService_ServeClient_V2(t *testing.T) {
	t.Parallel()
	fCoord := tailnettest.NewFakeCoordinator()
	var coord tailnet.Coordinator = fCoord
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coord)
	logger := testutil.Logger(t)
	derpMap := &tailcfg.DERPMap{Regions: map[int]*tailcfg.DERPRegion{999: {RegionCode: "test"}}}

	telemetryEvents := make(chan []*proto.TelemetryEvent, 64)
	uut, err := tailnet.NewClientService(tailnet.ClientServiceOptions{
		Logger:                 logger,
		CoordPtr:               &coordPtr,
		DERPMapUpdateFrequency: time.Millisecond,
		DERPMapFn:              func() *tailcfg.DERPMap { return derpMap },
		NetworkTelemetryHandler: func(batch []*proto.TelemetryEvent) {
			telemetryEvents <- batch
		},
		ResumeTokenProvider: tailnet.NewInsecureTestResumeTokenProvider(),
	})
	require.NoError(t, err)

	ctx := testutil.Context(t, testutil.WaitShort)
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()
	clientID := uuid.MustParse("10000001-0000-0000-0000-000000000000")
	agentID := uuid.MustParse("20000001-0000-0000-0000-000000000000")
	errCh := make(chan error, 1)
	go func() {
		err := uut.ServeClient(ctx, "2.0", s, tailnet.StreamID{
			Name: "client",
			ID:   clientID,
			Auth: tailnet.ClientCoordinateeAuth{
				AgentID: agentID,
			},
		})
		t.Logf("ServeClient returned; err=%v", err)
		errCh <- err
	}()

	client, err := tailnet.NewDRPCClient(c, logger)
	require.NoError(t, err)

	// Coordinate
	stream, err := client.Coordinate(ctx)
	require.NoError(t, err)
	defer stream.Close()

	err = stream.Send(&proto.CoordinateRequest{
		UpdateSelf: &proto.CoordinateRequest_UpdateSelf{Node: &proto.Node{PreferredDerp: 11}},
	})
	require.NoError(t, err)

	call := testutil.RequireRecvCtx(ctx, t, fCoord.CoordinateCalls)
	require.NotNil(t, call)
	require.Equal(t, call.ID, clientID)
	require.Equal(t, call.Name, "client")
	require.NoError(t, call.Auth.Authorize(ctx, &proto.CoordinateRequest{
		AddTunnel: &proto.CoordinateRequest_Tunnel{Id: agentID[:]},
	}))
	req := testutil.RequireRecvCtx(ctx, t, call.Reqs)
	require.Equal(t, int32(11), req.GetUpdateSelf().GetNode().GetPreferredDerp())

	call.Resps <- &proto.CoordinateResponse{PeerUpdates: []*proto.CoordinateResponse_PeerUpdate{
		{
			Kind: proto.CoordinateResponse_PeerUpdate_NODE,
			Node: &proto.Node{PreferredDerp: 22},
			Id:   agentID[:],
		},
	}}
	resp, err := stream.Recv()
	require.NoError(t, err)
	u := resp.GetPeerUpdates()
	require.Len(t, u, 1)
	require.Equal(t, int32(22), u[0].GetNode().GetPreferredDerp())

	err = stream.Close()
	require.NoError(t, err)

	// DERP Map
	dms, err := client.StreamDERPMaps(ctx, &proto.StreamDERPMapsRequest{})
	require.NoError(t, err)

	gotDermMap, err := dms.Recv()
	require.NoError(t, err)
	require.Equal(t, "test", gotDermMap.GetRegions()[999].GetRegionCode())
	err = dms.Close()
	require.NoError(t, err)

	// PostTelemetry
	telemetryReq := &proto.TelemetryRequest{
		Events: []*proto.TelemetryEvent{
			{
				Id: []byte("hi"),
			},
			{
				Id: []byte("bye"),
			},
		},
	}
	res, err := client.PostTelemetry(ctx, telemetryReq)
	require.NoError(t, err)
	require.NotNil(t, res)
	gotEvents := testutil.RequireRecvCtx(ctx, t, telemetryEvents)
	require.Len(t, gotEvents, 2)
	require.Equal(t, "hi", string(gotEvents[0].Id))
	require.Equal(t, "bye", string(gotEvents[1].Id))

	// RPCs closed; we need to close the Conn to end the session.
	err = c.Close()
	require.NoError(t, err)
	err = testutil.RequireRecvCtx(ctx, t, errCh)
	require.True(t, errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe))
}

func TestClientService_ServeClient_V1(t *testing.T) {
	t.Parallel()
	fCoord := tailnettest.NewFakeCoordinator()
	var coord tailnet.Coordinator = fCoord
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coord)
	logger := testutil.Logger(t)
	uut, err := tailnet.NewClientService(tailnet.ClientServiceOptions{
		Logger:                  logger,
		CoordPtr:                &coordPtr,
		DERPMapUpdateFrequency:  0,
		DERPMapFn:               nil,
		NetworkTelemetryHandler: nil,
		ResumeTokenProvider:     tailnet.NewInsecureTestResumeTokenProvider(),
	})
	require.NoError(t, err)

	ctx := testutil.Context(t, testutil.WaitShort)
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()
	clientID := uuid.MustParse("10000001-0000-0000-0000-000000000000")
	agentID := uuid.MustParse("20000001-0000-0000-0000-000000000000")
	errCh := make(chan error, 1)
	go func() {
		err := uut.ServeClient(ctx, "1.0", s, tailnet.StreamID{
			Name: "client",
			ID:   clientID,
			Auth: tailnet.ClientCoordinateeAuth{
				AgentID: agentID,
			},
		})
		t.Logf("ServeClient returned; err=%v", err)
		errCh <- err
	}()

	err = testutil.RequireRecvCtx(ctx, t, errCh)
	require.ErrorIs(t, err, tailnet.ErrUnsupportedVersion)
}

func TestNetworkTelemetryBatcher(t *testing.T) {
	t.Parallel()

	var (
		events = make(chan []*proto.TelemetryEvent, 64)
		mClock = quartz.NewMock(t)
		b      = tailnet.NewNetworkTelemetryBatcher(mClock, time.Millisecond, 3, func(batch []*proto.TelemetryEvent) {
			assert.LessOrEqual(t, len(batch), 3)
			events <- batch
		})
	)

	b.Handler([]*proto.TelemetryEvent{
		{Id: []byte("1")},
		{Id: []byte("2")},
	})
	b.Handler([]*proto.TelemetryEvent{
		{Id: []byte("3")},
		{Id: []byte("4")},
	})

	// Should overflow and send a batch.
	ctx := testutil.Context(t, testutil.WaitShort)
	batch := testutil.RequireRecvCtx(ctx, t, events)
	require.Len(t, batch, 3)
	require.Equal(t, "1", string(batch[0].Id))
	require.Equal(t, "2", string(batch[1].Id))
	require.Equal(t, "3", string(batch[2].Id))

	// Should send any pending events when the ticker fires.
	mClock.Advance(time.Millisecond)
	batch = testutil.RequireRecvCtx(ctx, t, events)
	require.Len(t, batch, 1)
	require.Equal(t, "4", string(batch[0].Id))

	// Should send any pending events when closed.
	b.Handler([]*proto.TelemetryEvent{
		{Id: []byte("5")},
		{Id: []byte("6")},
	})
	err := b.Close()
	require.NoError(t, err)
	batch = testutil.RequireRecvCtx(ctx, t, events)
	require.Len(t, batch, 2)
	require.Equal(t, "5", string(batch[0].Id))
	require.Equal(t, "6", string(batch[1].Id))
}

func TestClientUserCoordinateeAuth(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)

	agentID := uuid.UUID{0x01}
	agentID2 := uuid.UUID{0x02}
	clientID := uuid.UUID{0x03}

	ctrl := gomock.NewController(t)
	updatesProvider := tailnettest.NewMockWorkspaceUpdatesProvider(ctrl)

	fCoord, client := createUpdateService(t, ctx, clientID, updatesProvider)

	// Coordinate
	stream, err := client.Coordinate(ctx)
	require.NoError(t, err)
	defer stream.Close()

	err = stream.Send(&proto.CoordinateRequest{
		UpdateSelf: &proto.CoordinateRequest_UpdateSelf{Node: &proto.Node{PreferredDerp: 11}},
	})
	require.NoError(t, err)

	call := testutil.RequireRecvCtx(ctx, t, fCoord.CoordinateCalls)
	require.NotNil(t, call)
	require.Equal(t, call.ID, clientID)
	require.Equal(t, call.Name, "client")
	req := testutil.RequireRecvCtx(ctx, t, call.Reqs)
	require.Equal(t, int32(11), req.GetUpdateSelf().GetNode().GetPreferredDerp())

	// Authorize uses `ClientUserCoordinateeAuth`
	require.NoError(t, call.Auth.Authorize(ctx, &proto.CoordinateRequest{
		AddTunnel: &proto.CoordinateRequest_Tunnel{Id: tailnet.UUIDToByteSlice(agentID)},
	}))
	require.Error(t, call.Auth.Authorize(ctx, &proto.CoordinateRequest{
		AddTunnel: &proto.CoordinateRequest_Tunnel{Id: tailnet.UUIDToByteSlice(agentID2)},
	}))
}

func TestWorkspaceUpdates(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	updatesProvider := tailnettest.NewMockWorkspaceUpdatesProvider(ctrl)
	mSub := tailnettest.NewMockSubscription(ctrl)
	updatesCh := make(chan *proto.WorkspaceUpdate, 1)

	clientID := uuid.UUID{0x03}
	wsID := uuid.UUID{0x04}

	_, client := createUpdateService(t, ctx, clientID, updatesProvider)

	// Workspace updates
	expected := &proto.WorkspaceUpdate{
		UpsertedWorkspaces: []*proto.Workspace{
			{
				Id:     tailnet.UUIDToByteSlice(wsID),
				Name:   "ws1",
				Status: proto.Workspace_RUNNING,
			},
		},
		UpsertedAgents:    []*proto.Agent{},
		DeletedWorkspaces: []*proto.Workspace{},
		DeletedAgents:     []*proto.Agent{},
	}
	updatesCh <- expected
	updatesProvider.EXPECT().Subscribe(gomock.Any(), clientID).
		Times(1).
		Return(mSub, nil)
	mSub.EXPECT().Updates().MinTimes(1).Return(updatesCh)
	mSub.EXPECT().Close().Times(1).Return(nil)

	updatesStream, err := client.WorkspaceUpdates(ctx, &proto.WorkspaceUpdatesRequest{
		WorkspaceOwnerId: tailnet.UUIDToByteSlice(clientID),
	})
	require.NoError(t, err)
	defer updatesStream.Close()

	updates, err := updatesStream.Recv()
	require.NoError(t, err)
	require.Len(t, updates.GetUpsertedWorkspaces(), 1)
	require.Equal(t, expected.GetUpsertedWorkspaces()[0].GetName(), updates.GetUpsertedWorkspaces()[0].GetName())
	require.Equal(t, expected.GetUpsertedWorkspaces()[0].GetStatus(), updates.GetUpsertedWorkspaces()[0].GetStatus())
	require.Equal(t, expected.GetUpsertedWorkspaces()[0].GetId(), updates.GetUpsertedWorkspaces()[0].GetId())
}

//nolint:revive // t takes precedence
func createUpdateService(t *testing.T, ctx context.Context, clientID uuid.UUID, updates tailnet.WorkspaceUpdatesProvider) (*tailnettest.FakeCoordinator, proto.DRPCTailnetClient) {
	fCoord := tailnettest.NewFakeCoordinator()
	var coord tailnet.Coordinator = fCoord
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coord)
	logger := testutil.Logger(t)

	uut, err := tailnet.NewClientService(tailnet.ClientServiceOptions{
		Logger:                   logger,
		CoordPtr:                 &coordPtr,
		WorkspaceUpdatesProvider: updates,
	})
	require.NoError(t, err)

	c, s := net.Pipe()
	t.Cleanup(func() {
		_ = c.Close()
		_ = s.Close()
	})

	errCh := make(chan error, 1)
	go func() {
		err := uut.ServeClient(ctx, "2.0", s, tailnet.StreamID{
			Name: "client",
			ID:   clientID,
			Auth: tailnet.ClientUserCoordinateeAuth{
				Auth: &fakeTunnelAuth{},
			},
		})
		t.Logf("ServeClient returned; err=%v", err)
		errCh <- err
	}()

	client, err := tailnet.NewDRPCClient(c, logger)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = c.Close()
		require.NoError(t, err)
		err = testutil.RequireRecvCtx(ctx, t, errCh)
		require.True(t, errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe))
	})
	return fCoord, client
}

type fakeTunnelAuth struct{}

// AuthorizeTunnel implements tailnet.TunnelAuthorizer.
func (*fakeTunnelAuth) AuthorizeTunnel(_ context.Context, agentID uuid.UUID) error {
	if agentID[0] != 1 {
		return xerrors.New("policy disallows request")
	}
	return nil
}

var _ tailnet.TunnelAuthorizer = (*fakeTunnelAuth)(nil)
