package tailnet_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"storj.io/drpc"
	"storj.io/drpc/drpcerr"
	"tailscale.com/net/tsaddr"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	"tailscale.com/util/dnsname"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

var errUnimplemented = drpcerr.WithCode(xerrors.New("Unimplemented"), drpcerr.Unimplemented)

func TestInMemoryCoordination(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
	clientID := uuid.UUID{1}
	agentID := uuid.UUID{2}
	mCoord := tailnettest.NewMockCoordinator(gomock.NewController(t))
	fConn := &fakeCoordinatee{}

	reqs := make(chan *proto.CoordinateRequest, 100)
	resps := make(chan *proto.CoordinateResponse, 100)
	auth := tailnet.ClientCoordinateeAuth{AgentID: agentID}
	mCoord.EXPECT().Coordinate(gomock.Any(), clientID, gomock.Any(), auth).
		Times(1).Return(reqs, resps)

	ctrl := tailnet.NewTunnelSrcCoordController(logger, fConn)
	ctrl.AddDestination(agentID)
	uut := ctrl.New(tailnet.NewInMemoryCoordinatorClient(logger, clientID, auth, mCoord))
	defer uut.Close(ctx)

	coordinationTest(ctx, t, uut, fConn, reqs, resps, agentID)

	// Recv loop should be terminated by the server hanging up after Disconnect
	err := testutil.TryReceive(ctx, t, uut.Wait())
	require.ErrorIs(t, err, io.EOF)
}

func TestTunnelSrcCoordController_Mainline(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
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

	ctrl := tailnet.NewTunnelSrcCoordController(logger.Named("coordination"), fConn)
	ctrl.AddDestination(agentID)
	uut := ctrl.New(protocol)
	defer uut.Close(ctx)

	coordinationTest(ctx, t, uut, fConn, reqs, resps, agentID)

	// Recv loop should be terminated by the server hanging up after Disconnect
	err = testutil.TryReceive(ctx, t, uut.Wait())
	require.ErrorIs(t, err, io.EOF)
}

func TestTunnelSrcCoordController_AddDestination(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)

	fConn := &fakeCoordinatee{}
	uut := tailnet.NewTunnelSrcCoordController(logger, fConn)

	// GIVEN: client already connected
	client1 := newFakeCoordinatorClient(ctx, t)
	cw1 := uut.New(client1)

	// WHEN: we add 2 destinations
	dest1 := uuid.UUID{1}
	dest2 := uuid.UUID{2}
	addDone := make(chan struct{})
	go func() {
		defer close(addDone)
		uut.AddDestination(dest1)
		uut.AddDestination(dest2)
	}()

	// THEN: Controller sends AddTunnel for the destinations
	for i := range 2 {
		b0 := byte(i + 1)
		call := testutil.TryReceive(ctx, t, client1.reqs)
		require.Equal(t, b0, call.req.GetAddTunnel().GetId()[0])
		testutil.RequireSend(ctx, t, call.err, nil)
	}
	_ = testutil.TryReceive(ctx, t, addDone)

	// THEN: Controller sets destinations on Coordinatee
	require.Contains(t, fConn.tunnelDestinations, dest1)
	require.Contains(t, fConn.tunnelDestinations, dest2)

	// WHEN: Closed from server side and reconnects
	respCall := testutil.TryReceive(ctx, t, client1.resps)
	testutil.RequireSend(ctx, t, respCall.err, io.EOF)
	closeCall := testutil.TryReceive(ctx, t, client1.close)
	testutil.RequireSend(ctx, t, closeCall, nil)
	err := testutil.TryReceive(ctx, t, cw1.Wait())
	require.ErrorIs(t, err, io.EOF)
	client2 := newFakeCoordinatorClient(ctx, t)
	cws := make(chan tailnet.CloserWaiter)
	go func() {
		cws <- uut.New(client2)
	}()

	// THEN: should immediately send both destinations
	var dests []byte
	for range 2 {
		call := testutil.TryReceive(ctx, t, client2.reqs)
		dests = append(dests, call.req.GetAddTunnel().GetId()[0])
		testutil.RequireSend(ctx, t, call.err, nil)
	}
	slices.Sort(dests)
	require.Equal(t, dests, []byte{1, 2})

	cw2 := testutil.TryReceive(ctx, t, cws)

	// close client2
	respCall = testutil.TryReceive(ctx, t, client2.resps)
	testutil.RequireSend(ctx, t, respCall.err, io.EOF)
	closeCall = testutil.TryReceive(ctx, t, client2.close)
	testutil.RequireSend(ctx, t, closeCall, nil)
	err = testutil.TryReceive(ctx, t, cw2.Wait())
	require.ErrorIs(t, err, io.EOF)
}

func TestTunnelSrcCoordController_RemoveDestination(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)

	fConn := &fakeCoordinatee{}
	uut := tailnet.NewTunnelSrcCoordController(logger, fConn)

	// GIVEN: 1 destination
	dest1 := uuid.UUID{1}
	uut.AddDestination(dest1)

	// GIVEN: client already connected
	client1 := newFakeCoordinatorClient(ctx, t)
	cws := make(chan tailnet.CloserWaiter)
	go func() {
		cws <- uut.New(client1)
	}()
	call := testutil.TryReceive(ctx, t, client1.reqs)
	testutil.RequireSend(ctx, t, call.err, nil)
	cw1 := testutil.TryReceive(ctx, t, cws)

	// WHEN: we remove one destination
	removeDone := make(chan struct{})
	go func() {
		defer close(removeDone)
		uut.RemoveDestination(dest1)
	}()

	// THEN: Controller sends RemoveTunnel for the destination
	call = testutil.TryReceive(ctx, t, client1.reqs)
	require.Equal(t, dest1[:], call.req.GetRemoveTunnel().GetId())
	testutil.RequireSend(ctx, t, call.err, nil)
	_ = testutil.TryReceive(ctx, t, removeDone)

	// WHEN: Closed from server side and reconnect
	respCall := testutil.TryReceive(ctx, t, client1.resps)
	testutil.RequireSend(ctx, t, respCall.err, io.EOF)
	closeCall := testutil.TryReceive(ctx, t, client1.close)
	testutil.RequireSend(ctx, t, closeCall, nil)
	err := testutil.TryReceive(ctx, t, cw1.Wait())
	require.ErrorIs(t, err, io.EOF)

	client2 := newFakeCoordinatorClient(ctx, t)
	go func() {
		cws <- uut.New(client2)
	}()

	// THEN: should immediately resolve without sending anything
	cw2 := testutil.TryReceive(ctx, t, cws)

	// close client2
	respCall = testutil.TryReceive(ctx, t, client2.resps)
	testutil.RequireSend(ctx, t, respCall.err, io.EOF)
	closeCall = testutil.TryReceive(ctx, t, client2.close)
	testutil.RequireSend(ctx, t, closeCall, nil)
	err = testutil.TryReceive(ctx, t, cw2.Wait())
	require.ErrorIs(t, err, io.EOF)
}

func TestTunnelSrcCoordController_RemoveDestination_Error(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)

	fConn := &fakeCoordinatee{}
	uut := tailnet.NewTunnelSrcCoordController(logger, fConn)

	// GIVEN: 3 destination
	dest1 := uuid.UUID{1}
	dest2 := uuid.UUID{2}
	dest3 := uuid.UUID{3}
	uut.AddDestination(dest1)
	uut.AddDestination(dest2)
	uut.AddDestination(dest3)

	// GIVEN: client already connected
	client1 := newFakeCoordinatorClient(ctx, t)
	cws := make(chan tailnet.CloserWaiter)
	go func() {
		cws <- uut.New(client1)
	}()
	for range 3 {
		call := testutil.TryReceive(ctx, t, client1.reqs)
		testutil.RequireSend(ctx, t, call.err, nil)
	}
	cw1 := testutil.TryReceive(ctx, t, cws)

	// WHEN: we remove all destinations
	removeDone := make(chan struct{})
	go func() {
		defer close(removeDone)
		uut.RemoveDestination(dest1)
		uut.RemoveDestination(dest2)
		uut.RemoveDestination(dest3)
	}()

	// WHEN: first RemoveTunnel call fails
	theErr := xerrors.New("a bad thing happened")
	call := testutil.TryReceive(ctx, t, client1.reqs)
	require.Equal(t, dest1[:], call.req.GetRemoveTunnel().GetId())
	testutil.RequireSend(ctx, t, call.err, theErr)

	// THEN: we disconnect and do not send remaining RemoveTunnel messages
	closeCall := testutil.TryReceive(ctx, t, client1.close)
	testutil.RequireSend(ctx, t, closeCall, nil)
	_ = testutil.TryReceive(ctx, t, removeDone)

	// shut down
	respCall := testutil.TryReceive(ctx, t, client1.resps)
	testutil.RequireSend(ctx, t, respCall.err, io.EOF)
	// triggers second close call
	closeCall = testutil.TryReceive(ctx, t, client1.close)
	testutil.RequireSend(ctx, t, closeCall, nil)
	err := testutil.TryReceive(ctx, t, cw1.Wait())
	require.ErrorIs(t, err, theErr)
}

func TestTunnelSrcCoordController_Sync(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)

	fConn := &fakeCoordinatee{}
	uut := tailnet.NewTunnelSrcCoordController(logger, fConn)
	dest1 := uuid.UUID{1}
	dest2 := uuid.UUID{2}
	dest3 := uuid.UUID{3}

	// GIVEN: dest1 & dest2 already added
	uut.AddDestination(dest1)
	uut.AddDestination(dest2)

	// GIVEN: client already connected
	client1 := newFakeCoordinatorClient(ctx, t)
	cws := make(chan tailnet.CloserWaiter)
	go func() {
		cws <- uut.New(client1)
	}()
	for range 2 {
		call := testutil.TryReceive(ctx, t, client1.reqs)
		testutil.RequireSend(ctx, t, call.err, nil)
	}
	cw1 := testutil.TryReceive(ctx, t, cws)

	// WHEN: we sync dest2 & dest3
	syncDone := make(chan struct{})
	go func() {
		defer close(syncDone)
		uut.SyncDestinations([]uuid.UUID{dest2, dest3})
	}()

	// THEN: we get an add for dest3 and remove for dest1
	call := testutil.TryReceive(ctx, t, client1.reqs)
	require.Equal(t, dest3[:], call.req.GetAddTunnel().GetId())
	testutil.RequireSend(ctx, t, call.err, nil)
	call = testutil.TryReceive(ctx, t, client1.reqs)
	require.Equal(t, dest1[:], call.req.GetRemoveTunnel().GetId())
	testutil.RequireSend(ctx, t, call.err, nil)

	testutil.TryReceive(ctx, t, syncDone)
	// dest3 should be added to coordinatee
	require.Contains(t, fConn.tunnelDestinations, dest3)

	// shut down
	respCall := testutil.TryReceive(ctx, t, client1.resps)
	testutil.RequireSend(ctx, t, respCall.err, io.EOF)
	closeCall := testutil.TryReceive(ctx, t, client1.close)
	testutil.RequireSend(ctx, t, closeCall, nil)
	err := testutil.TryReceive(ctx, t, cw1.Wait())
	require.ErrorIs(t, err, io.EOF)
}

func TestTunnelSrcCoordController_AddDestination_Error(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)

	fConn := &fakeCoordinatee{}
	uut := tailnet.NewTunnelSrcCoordController(logger, fConn)

	// GIVEN: client already connected
	client1 := newFakeCoordinatorClient(ctx, t)
	cw1 := uut.New(client1)

	// WHEN: we add a destination, and the AddTunnel fails
	dest1 := uuid.UUID{1}
	addDone := make(chan struct{})
	go func() {
		defer close(addDone)
		uut.AddDestination(dest1)
	}()
	theErr := xerrors.New("a bad thing happened")
	call := testutil.TryReceive(ctx, t, client1.reqs)
	testutil.RequireSend(ctx, t, call.err, theErr)

	// THEN: Client is closed and exits
	closeCall := testutil.TryReceive(ctx, t, client1.close)
	testutil.RequireSend(ctx, t, closeCall, nil)

	// close the resps, since the client has closed
	resp := testutil.TryReceive(ctx, t, client1.resps)
	testutil.RequireSend(ctx, t, resp.err, net.ErrClosed)
	// this triggers a second Close() call on the client
	closeCall = testutil.TryReceive(ctx, t, client1.close)
	testutil.RequireSend(ctx, t, closeCall, nil)

	err := testutil.TryReceive(ctx, t, cw1.Wait())
	require.ErrorIs(t, err, theErr)

	_ = testutil.TryReceive(ctx, t, addDone)
}

func TestAgentCoordinationController_SendsReadyForHandshake(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
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
	testutil.RequireSend(ctx, t, resps, &proto.CoordinateResponse{
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

	rfh := testutil.TryReceive(ctx, t, reqs)
	require.NotNil(t, rfh.ReadyForHandshake)
	require.Len(t, rfh.ReadyForHandshake, 1)
	require.Equal(t, clientID[:], rfh.ReadyForHandshake[0].Id)

	go uut.Close(ctx)
	dis := testutil.TryReceive(ctx, t, reqs)
	require.NotNil(t, dis)
	require.NotNil(t, dis.Disconnect)
	close(resps)

	// Recv loop should be terminated by the server hanging up after Disconnect
	err = testutil.TryReceive(ctx, t, uut.Wait())
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
	req := testutil.TryReceive(ctx, t, reqs)
	require.Equal(t, agentID[:], req.GetAddTunnel().GetId())

	// when we call the callback, it should send a node update
	require.NotNil(t, fConn.callback)
	fConn.callback(&tailnet.Node{PreferredDERP: 1})

	req = testutil.TryReceive(ctx, t, reqs)
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
	testutil.RequireSend(ctx, t, resps, &proto.CoordinateResponse{PeerUpdates: updates})
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
	req = testutil.TryReceive(ctx, t, reqs)
	require.NotNil(t, req.Disconnect)
	close(resps)

	err = testutil.TryReceive(ctx, t, errCh)
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

func TestNewBasicDERPController_Mainline(t *testing.T) {
	t.Parallel()
	fs := make(chan *tailcfg.DERPMap)
	logger := testutil.Logger(t)
	uut := tailnet.NewBasicDERPController(logger, fakeSetter(fs))
	fc := fakeDERPClient{
		ch: make(chan *tailcfg.DERPMap),
	}
	c := uut.New(fc)
	ctx := testutil.Context(t, testutil.WaitShort)
	expectDM := &tailcfg.DERPMap{}
	testutil.RequireSend(ctx, t, fc.ch, expectDM)
	gotDM := testutil.TryReceive(ctx, t, fs)
	require.Equal(t, expectDM, gotDM)
	err := c.Close(ctx)
	require.NoError(t, err)
	err = testutil.TryReceive(ctx, t, c.Wait())
	require.ErrorIs(t, err, io.EOF)
	// ensure Close is idempotent
	err = c.Close(ctx)
	require.NoError(t, err)
}

func TestNewBasicDERPController_RecvErr(t *testing.T) {
	t.Parallel()
	fs := make(chan *tailcfg.DERPMap)
	logger := testutil.Logger(t)
	uut := tailnet.NewBasicDERPController(logger, fakeSetter(fs))
	expectedErr := xerrors.New("a bad thing happened")
	fc := fakeDERPClient{
		ch:  make(chan *tailcfg.DERPMap),
		err: expectedErr,
	}
	c := uut.New(fc)
	ctx := testutil.Context(t, testutil.WaitShort)
	err := testutil.TryReceive(ctx, t, c.Wait())
	require.ErrorIs(t, err, expectedErr)
	// ensure Close is idempotent
	err = c.Close(ctx)
	require.NoError(t, err)
}

type fakeSetter chan *tailcfg.DERPMap

func (s fakeSetter) SetDERPMap(derpMap *tailcfg.DERPMap) {
	s <- derpMap
}

type fakeDERPClient struct {
	ch  chan *tailcfg.DERPMap
	err error
}

func (f fakeDERPClient) Close() error {
	close(f.ch)
	return nil
}

func (f fakeDERPClient) Recv() (*tailcfg.DERPMap, error) {
	if f.err != nil {
		return nil, f.err
	}
	dm, ok := <-f.ch
	if ok {
		return dm, nil
	}
	return nil, io.EOF
}

func TestBasicTelemetryController_Success(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)

	uut := tailnet.NewBasicTelemetryController(logger)
	ft := newFakeTelemetryClient()
	uut.New(ft)

	sendDone := make(chan struct{})
	go func() {
		defer close(sendDone)
		uut.SendTelemetryEvent(&proto.TelemetryEvent{
			Id: []byte("test event"),
		})
	}()

	call := testutil.TryReceive(ctx, t, ft.calls)
	require.Len(t, call.req.GetEvents(), 1)
	require.Equal(t, call.req.GetEvents()[0].GetId(), []byte("test event"))

	testutil.RequireSend(ctx, t, call.errCh, nil)
	testutil.TryReceive(ctx, t, sendDone)
}

func TestBasicTelemetryController_Unimplemented(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)

	ft := newFakeTelemetryClient()

	uut := tailnet.NewBasicTelemetryController(logger)
	uut.New(ft)

	// bad code, doesn't count
	telemetryError := drpcerr.WithCode(xerrors.New("Unimplemented"), 0)

	sendDone := make(chan struct{})
	go func() {
		defer close(sendDone)
		uut.SendTelemetryEvent(&proto.TelemetryEvent{})
	}()

	call := testutil.TryReceive(ctx, t, ft.calls)
	testutil.RequireSend(ctx, t, call.errCh, telemetryError)
	testutil.TryReceive(ctx, t, sendDone)

	sendDone = make(chan struct{})
	go func() {
		defer close(sendDone)
		uut.SendTelemetryEvent(&proto.TelemetryEvent{})
	}()

	// we get another call since it wasn't really the Unimplemented error
	call = testutil.TryReceive(ctx, t, ft.calls)

	// for real this time
	telemetryError = errUnimplemented
	testutil.RequireSend(ctx, t, call.errCh, telemetryError)
	testutil.TryReceive(ctx, t, sendDone)

	// now this returns immediately without a call, because unimplemented error disables calling
	sendDone = make(chan struct{})
	go func() {
		defer close(sendDone)
		uut.SendTelemetryEvent(&proto.TelemetryEvent{})
	}()
	testutil.TryReceive(ctx, t, sendDone)

	// getting a "new" client resets
	uut.New(ft)
	sendDone = make(chan struct{})
	go func() {
		defer close(sendDone)
		uut.SendTelemetryEvent(&proto.TelemetryEvent{})
	}()
	call = testutil.TryReceive(ctx, t, ft.calls)
	testutil.RequireSend(ctx, t, call.errCh, nil)
	testutil.TryReceive(ctx, t, sendDone)
}

func TestBasicTelemetryController_NotRecognised(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
	ft := newFakeTelemetryClient()
	uut := tailnet.NewBasicTelemetryController(logger)
	uut.New(ft)

	sendDone := make(chan struct{})
	go func() {
		defer close(sendDone)
		uut.SendTelemetryEvent(&proto.TelemetryEvent{})
	}()
	// returning generic protocol error doesn't trigger unknown rpc logic
	call := testutil.TryReceive(ctx, t, ft.calls)
	testutil.RequireSend(ctx, t, call.errCh, drpc.ProtocolError.New("Protocol Error"))
	testutil.TryReceive(ctx, t, sendDone)

	sendDone = make(chan struct{})
	go func() {
		defer close(sendDone)
		uut.SendTelemetryEvent(&proto.TelemetryEvent{})
	}()
	call = testutil.TryReceive(ctx, t, ft.calls)
	// return the expected protocol error this time
	testutil.RequireSend(ctx, t, call.errCh,
		drpc.ProtocolError.New("unknown rpc: /coder.tailnet.v2.Tailnet/PostTelemetry"))
	testutil.TryReceive(ctx, t, sendDone)

	// now this returns immediately without a call, because unimplemented error disables calling
	sendDone = make(chan struct{})
	go func() {
		defer close(sendDone)
		uut.SendTelemetryEvent(&proto.TelemetryEvent{})
	}()
	testutil.TryReceive(ctx, t, sendDone)
}

type fakeTelemetryClient struct {
	calls chan *fakeTelemetryCall
}

var _ tailnet.TelemetryClient = &fakeTelemetryClient{}

func newFakeTelemetryClient() *fakeTelemetryClient {
	return &fakeTelemetryClient{
		calls: make(chan *fakeTelemetryCall),
	}
}

// PostTelemetry implements tailnet.TelemetryClient
func (f *fakeTelemetryClient) PostTelemetry(ctx context.Context, req *proto.TelemetryRequest) (*proto.TelemetryResponse, error) {
	fr := &fakeTelemetryCall{req: req, errCh: make(chan error)}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case f.calls <- fr:
		// OK
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-fr.errCh:
		return &proto.TelemetryResponse{}, err
	}
}

type fakeTelemetryCall struct {
	req   *proto.TelemetryRequest
	errCh chan error
}

func TestBasicResumeTokenController_Mainline(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
	fr := newFakeResumeTokenClient(ctx)
	mClock := quartz.NewMock(t)
	trp := mClock.Trap().TimerReset("basicResumeTokenRefresher", "refresh")
	defer trp.Close()

	uut := tailnet.NewBasicResumeTokenController(logger, mClock)
	_, ok := uut.Token()
	require.False(t, ok)

	cwCh := make(chan tailnet.CloserWaiter, 1)
	go func() {
		cwCh <- uut.New(fr)
	}()
	call := testutil.TryReceive(ctx, t, fr.calls)
	testutil.RequireSend(ctx, t, call.resp, &proto.RefreshResumeTokenResponse{
		Token:     "test token 1",
		RefreshIn: durationpb.New(100 * time.Second),
		ExpiresAt: timestamppb.New(mClock.Now().Add(200 * time.Second)),
	})
	trp.MustWait(ctx).Release() // initial refresh done
	token, ok := uut.Token()
	require.True(t, ok)
	require.Equal(t, "test token 1", token)
	cw := testutil.TryReceive(ctx, t, cwCh)

	w := mClock.Advance(100 * time.Second)
	call = testutil.TryReceive(ctx, t, fr.calls)
	testutil.RequireSend(ctx, t, call.resp, &proto.RefreshResumeTokenResponse{
		Token:     "test token 2",
		RefreshIn: durationpb.New(50 * time.Second),
		ExpiresAt: timestamppb.New(mClock.Now().Add(200 * time.Second)),
	})
	resetCall := trp.MustWait(ctx)
	require.Equal(t, resetCall.Duration, 50*time.Second)
	resetCall.Release()
	w.MustWait(ctx)
	token, ok = uut.Token()
	require.True(t, ok)
	require.Equal(t, "test token 2", token)

	err := cw.Close(ctx)
	require.NoError(t, err)
	err = testutil.TryReceive(ctx, t, cw.Wait())
	require.NoError(t, err)

	token, ok = uut.Token()
	require.True(t, ok)
	require.Equal(t, "test token 2", token)

	mClock.Advance(201 * time.Second).MustWait(ctx)
	_, ok = uut.Token()
	require.False(t, ok)
}

func TestBasicResumeTokenController_NewWhileRefreshing(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
	mClock := quartz.NewMock(t)
	trp := mClock.Trap().TimerReset("basicResumeTokenRefresher", "refresh")
	defer trp.Close()

	uut := tailnet.NewBasicResumeTokenController(logger, mClock)
	_, ok := uut.Token()
	require.False(t, ok)

	fr1 := newFakeResumeTokenClient(ctx)
	cwCh1 := make(chan tailnet.CloserWaiter, 1)
	go func() {
		cwCh1 <- uut.New(fr1)
	}()
	call1 := testutil.TryReceive(ctx, t, fr1.calls)

	fr2 := newFakeResumeTokenClient(ctx)
	cwCh2 := make(chan tailnet.CloserWaiter, 1)
	go func() {
		cwCh2 <- uut.New(fr2)
	}()
	call2 := testutil.TryReceive(ctx, t, fr2.calls)

	testutil.RequireSend(ctx, t, call2.resp, &proto.RefreshResumeTokenResponse{
		Token:     "test token 2.0",
		RefreshIn: durationpb.New(102 * time.Second),
		ExpiresAt: timestamppb.New(mClock.Now().Add(200 * time.Second)),
	})

	cw2 := testutil.TryReceive(ctx, t, cwCh2) // this ensures Close was called on 1

	testutil.RequireSend(ctx, t, call1.resp, &proto.RefreshResumeTokenResponse{
		Token:     "test token 1",
		RefreshIn: durationpb.New(101 * time.Second),
		ExpiresAt: timestamppb.New(mClock.Now().Add(200 * time.Second)),
	})

	trp.MustWait(ctx).Release()

	token, ok := uut.Token()
	require.True(t, ok)
	require.Equal(t, "test token 2.0", token)

	// refresher 1 should already be closed.
	cw1 := testutil.TryReceive(ctx, t, cwCh1)
	err := testutil.TryReceive(ctx, t, cw1.Wait())
	require.NoError(t, err)

	w := mClock.Advance(102 * time.Second)
	call := testutil.TryReceive(ctx, t, fr2.calls)
	testutil.RequireSend(ctx, t, call.resp, &proto.RefreshResumeTokenResponse{
		Token:     "test token 2.1",
		RefreshIn: durationpb.New(50 * time.Second),
		ExpiresAt: timestamppb.New(mClock.Now().Add(200 * time.Second)),
	})
	resetCall := trp.MustWait(ctx)
	require.Equal(t, resetCall.Duration, 50*time.Second)
	resetCall.Release()
	w.MustWait(ctx)
	token, ok = uut.Token()
	require.True(t, ok)
	require.Equal(t, "test token 2.1", token)

	err = cw2.Close(ctx)
	require.NoError(t, err)
	err = testutil.TryReceive(ctx, t, cw2.Wait())
	require.NoError(t, err)
}

func TestBasicResumeTokenController_Unimplemented(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
	mClock := quartz.NewMock(t)

	uut := tailnet.NewBasicResumeTokenController(logger, mClock)
	_, ok := uut.Token()
	require.False(t, ok)

	fr := newFakeResumeTokenClient(ctx)
	cw := uut.New(fr)

	call := testutil.TryReceive(ctx, t, fr.calls)
	testutil.RequireSend(ctx, t, call.errCh, errUnimplemented)
	err := testutil.TryReceive(ctx, t, cw.Wait())
	require.NoError(t, err)
	_, ok = uut.Token()
	require.False(t, ok)
}

func newFakeResumeTokenClient(ctx context.Context) *fakeResumeTokenClient {
	return &fakeResumeTokenClient{
		ctx:   ctx,
		calls: make(chan *fakeResumeTokenCall),
	}
}

type fakeResumeTokenClient struct {
	ctx   context.Context
	calls chan *fakeResumeTokenCall
}

func (f *fakeResumeTokenClient) RefreshResumeToken(_ context.Context, _ *proto.RefreshResumeTokenRequest) (*proto.RefreshResumeTokenResponse, error) {
	call := &fakeResumeTokenCall{
		resp:  make(chan *proto.RefreshResumeTokenResponse),
		errCh: make(chan error),
	}
	select {
	case <-f.ctx.Done():
		return nil, errTimeoutOnFake
	case f.calls <- call:
		// OK
	}
	select {
	case <-f.ctx.Done():
		return nil, errTimeoutOnFake
	case err := <-call.errCh:
		return nil, err
	case resp := <-call.resp:
		return resp, nil
	}
}

type fakeResumeTokenCall struct {
	resp  chan *proto.RefreshResumeTokenResponse
	errCh chan error
}

func TestController_Disconnects(t *testing.T) {
	t.Parallel()
	testCtx := testutil.Context(t, testutil.WaitShort)
	ctx, cancel := context.WithCancel(testCtx)
	logger := slogtest.Make(t, &slogtest.Options{
		IgnoredErrorIs: append(slogtest.DefaultIgnoredErrorIs,
			io.EOF,                   // we get EOF when we simulate a DERPMap error
			yamux.ErrSessionShutdown, // coordination can throw these when DERP error tears down session
		),
	}).Leveled(slog.LevelDebug)
	agentID := uuid.UUID{0x55}
	clientID := uuid.UUID{0x66}
	fCoord := tailnettest.NewFakeCoordinator()
	var coord tailnet.Coordinator = fCoord
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coord)
	derpMapCh := make(chan *tailcfg.DERPMap)
	defer close(derpMapCh)
	svc, err := tailnet.NewClientService(tailnet.ClientServiceOptions{
		Logger:                  logger.Named("svc"),
		CoordPtr:                &coordPtr,
		DERPMapUpdateFrequency:  time.Millisecond,
		DERPMapFn:               func() *tailcfg.DERPMap { return <-derpMapCh },
		NetworkTelemetryHandler: func([]*proto.TelemetryEvent) {},
		ResumeTokenProvider:     tailnet.NewInsecureTestResumeTokenProvider(),
	})
	require.NoError(t, err)

	dialer := &pipeDialer{
		ctx:    testCtx,
		logger: logger,
		t:      t,
		svc:    svc,
		streamID: tailnet.StreamID{
			Name: "client",
			ID:   clientID,
			Auth: tailnet.ClientCoordinateeAuth{AgentID: agentID},
		},
	}

	peersLost := make(chan struct{})
	fConn := &fakeTailnetConn{peersLostCh: peersLost}

	uut := tailnet.NewController(logger.Named("ctrl"), dialer,
		// darwin can be slow sometimes.
		tailnet.WithGracefulTimeout(5*time.Second))
	uut.CoordCtrl = tailnet.NewAgentCoordinationController(logger.Named("coord_ctrl"), fConn)
	uut.DERPCtrl = tailnet.NewBasicDERPController(logger.Named("derp_ctrl"), fConn)
	uut.Run(ctx)

	call := testutil.TryReceive(testCtx, t, fCoord.CoordinateCalls)

	// simulate a problem with DERPMaps by sending nil
	testutil.RequireSend(testCtx, t, derpMapCh, nil)

	// this should cause the coordinate call to hang up WITHOUT disconnecting
	reqNil := testutil.TryReceive(testCtx, t, call.Reqs)
	require.Nil(t, reqNil)

	// and mark all peers lost
	_ = testutil.TryReceive(testCtx, t, peersLost)

	// ...and then reconnect
	call = testutil.TryReceive(testCtx, t, fCoord.CoordinateCalls)

	// close the coordination call, which should cause a 2nd reconnection
	close(call.Resps)
	_ = testutil.TryReceive(testCtx, t, peersLost)
	call = testutil.TryReceive(testCtx, t, fCoord.CoordinateCalls)

	// canceling the context should trigger the disconnect message
	cancel()
	reqDisc := testutil.TryReceive(testCtx, t, call.Reqs)
	require.NotNil(t, reqDisc)
	require.NotNil(t, reqDisc.Disconnect)
	close(call.Resps)

	_ = testutil.TryReceive(testCtx, t, peersLost)
	_ = testutil.TryReceive(testCtx, t, uut.Closed())
}

func TestController_TelemetrySuccess(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
	agentID := uuid.UUID{0x55}
	clientID := uuid.UUID{0x66}
	fCoord := tailnettest.NewFakeCoordinator()
	var coord tailnet.Coordinator = fCoord
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coord)
	derpMapCh := make(chan *tailcfg.DERPMap)
	defer close(derpMapCh)
	eventCh := make(chan []*proto.TelemetryEvent, 1)
	svc, err := tailnet.NewClientService(tailnet.ClientServiceOptions{
		Logger:                 logger,
		CoordPtr:               &coordPtr,
		DERPMapUpdateFrequency: time.Millisecond,
		DERPMapFn:              func() *tailcfg.DERPMap { return <-derpMapCh },
		NetworkTelemetryHandler: func(batch []*proto.TelemetryEvent) {
			select {
			case <-ctx.Done():
				t.Error("timeout sending telemetry event")
			case eventCh <- batch:
				t.Log("sent telemetry batch")
			}
		},
		ResumeTokenProvider: tailnet.NewInsecureTestResumeTokenProvider(),
	})
	require.NoError(t, err)

	dialer := &pipeDialer{
		ctx:    ctx,
		logger: logger,
		t:      t,
		svc:    svc,
		streamID: tailnet.StreamID{
			Name: "client",
			ID:   clientID,
			Auth: tailnet.ClientCoordinateeAuth{AgentID: agentID},
		},
	}

	uut := tailnet.NewController(logger, dialer)
	uut.CoordCtrl = tailnet.NewAgentCoordinationController(logger, &fakeTailnetConn{})
	tel := tailnet.NewBasicTelemetryController(logger)
	uut.TelemetryCtrl = tel
	uut.Run(ctx)
	// Coordinate calls happen _after_ telemetry is connected up, so we use this
	// to ensure telemetry is connected before sending our event
	cc := testutil.TryReceive(ctx, t, fCoord.CoordinateCalls)
	defer close(cc.Resps)

	tel.SendTelemetryEvent(&proto.TelemetryEvent{
		Id: []byte("test event"),
	})

	testEvents := testutil.TryReceive(ctx, t, eventCh)

	require.Len(t, testEvents, 1)
	require.Equal(t, []byte("test event"), testEvents[0].Id)
}

func TestController_WorkspaceUpdates(t *testing.T) {
	t.Parallel()
	theError := xerrors.New("a bad thing happened")
	testCtx := testutil.Context(t, testutil.WaitShort)
	ctx, cancel := context.WithCancel(testCtx)
	logger := slogtest.Make(t, &slogtest.Options{
		IgnoredErrorIs: append(slogtest.DefaultIgnoredErrorIs, theError),
	}).Leveled(slog.LevelDebug)

	fClient := newFakeWorkspaceUpdateClient(testCtx, t)
	dialer := &fakeWorkspaceUpdatesDialer{
		client: fClient,
	}

	uut := tailnet.NewController(logger.Named("ctrl"), dialer)
	fCtrl := newFakeUpdatesController(ctx, t)
	uut.WorkspaceUpdatesCtrl = fCtrl
	uut.Run(ctx)

	// it should dial and pass the client to the controller
	call := testutil.TryReceive(testCtx, t, fCtrl.calls)
	require.Equal(t, fClient, call.client)
	fCW := newFakeCloserWaiter()
	testutil.RequireSend[tailnet.CloserWaiter](testCtx, t, call.resp, fCW)

	// if the CloserWaiter exits...
	testutil.RequireSend(testCtx, t, fCW.errCh, theError)

	// it should close, redial and reconnect
	cCall := testutil.TryReceive(testCtx, t, fClient.close)
	testutil.RequireSend(testCtx, t, cCall, nil)

	call = testutil.TryReceive(testCtx, t, fCtrl.calls)
	require.Equal(t, fClient, call.client)
	fCW = newFakeCloserWaiter()
	testutil.RequireSend[tailnet.CloserWaiter](testCtx, t, call.resp, fCW)

	// canceling the context should close the client
	cancel()
	cCall = testutil.TryReceive(testCtx, t, fClient.close)
	testutil.RequireSend(testCtx, t, cCall, nil)
}

type fakeTailnetConn struct {
	peersLostCh chan struct{}
}

func (*fakeTailnetConn) UpdatePeers([]*proto.CoordinateResponse_PeerUpdate) error {
	// TODO implement me
	panic("implement me")
}

func (f *fakeTailnetConn) SetAllPeersLost() {
	if f.peersLostCh == nil {
		return
	}
	f.peersLostCh <- struct{}{}
}

func (*fakeTailnetConn) SetNodeCallback(func(*tailnet.Node)) {}

func (*fakeTailnetConn) SetDERPMap(*tailcfg.DERPMap) {}

func (*fakeTailnetConn) SetTunnelDestination(uuid.UUID) {}

type pipeDialer struct {
	ctx      context.Context
	logger   slog.Logger
	t        testing.TB
	svc      *tailnet.ClientService
	streamID tailnet.StreamID
}

func (p *pipeDialer) Dial(_ context.Context, _ tailnet.ResumeTokenController) (tailnet.ControlProtocolClients, error) {
	s, c := net.Pipe()
	p.t.Cleanup(func() {
		_ = s.Close()
		_ = c.Close()
	})
	go func() {
		err := p.svc.ServeConnV2(p.ctx, s, p.streamID)
		p.logger.Debug(p.ctx, "piped tailnet service complete", slog.Error(err))
	}()
	client, err := tailnet.NewDRPCClient(c, p.logger)
	if err != nil {
		_ = c.Close()
		return tailnet.ControlProtocolClients{}, err
	}
	coord, err := client.Coordinate(context.Background())
	if err != nil {
		_ = c.Close()
		return tailnet.ControlProtocolClients{}, err
	}

	derps := &tailnet.DERPFromDRPCWrapper{}
	derps.Client, err = client.StreamDERPMaps(context.Background(), &proto.StreamDERPMapsRequest{})
	if err != nil {
		_ = c.Close()
		return tailnet.ControlProtocolClients{}, err
	}
	return tailnet.ControlProtocolClients{
		Closer:      client.DRPCConn(),
		Coordinator: coord,
		DERP:        derps,
		ResumeToken: client,
		Telemetry:   client,
	}, nil
}

// errTimeoutOnFake is the error we send when fakes fail to send calls or receive responses before
// their context times out. We don't want to send the context error since that often doesn't trigger
// test failures or logging.
var errTimeoutOnFake = xerrors.New("test timeout")

type fakeCoordinatorClient struct {
	ctx   context.Context
	t     testing.TB
	reqs  chan *coordReqCall
	resps chan *coordRespCall
	close chan chan<- error
}

func (f fakeCoordinatorClient) Close() error {
	f.t.Helper()
	errs := make(chan error)
	select {
	case <-f.ctx.Done():
		return errTimeoutOnFake
	case f.close <- errs:
		// OK
	}
	select {
	case <-f.ctx.Done():
		return errTimeoutOnFake
	case err := <-errs:
		return err
	}
}

func (f fakeCoordinatorClient) Send(request *proto.CoordinateRequest) error {
	f.t.Helper()
	errs := make(chan error)
	call := &coordReqCall{
		req: request,
		err: errs,
	}
	select {
	case <-f.ctx.Done():
		return errTimeoutOnFake
	case f.reqs <- call:
		// OK
	}
	select {
	case <-f.ctx.Done():
		return errTimeoutOnFake
	case err := <-errs:
		return err
	}
}

func (f fakeCoordinatorClient) Recv() (*proto.CoordinateResponse, error) {
	f.t.Helper()
	resps := make(chan *proto.CoordinateResponse)
	errs := make(chan error)
	call := &coordRespCall{
		resp: resps,
		err:  errs,
	}
	select {
	case <-f.ctx.Done():
		return nil, errTimeoutOnFake
	case f.resps <- call:
		// OK
	}
	select {
	case <-f.ctx.Done():
		return nil, errTimeoutOnFake
	case err := <-errs:
		return nil, err
	case resp := <-resps:
		return resp, nil
	}
}

func newFakeCoordinatorClient(ctx context.Context, t testing.TB) *fakeCoordinatorClient {
	return &fakeCoordinatorClient{
		ctx:   ctx,
		t:     t,
		reqs:  make(chan *coordReqCall),
		resps: make(chan *coordRespCall),
		close: make(chan chan<- error),
	}
}

type coordReqCall struct {
	req *proto.CoordinateRequest
	err chan<- error
}

type coordRespCall struct {
	resp chan<- *proto.CoordinateResponse
	err  chan<- error
}

type fakeWorkspaceUpdateClient struct {
	ctx   context.Context
	t     testing.TB
	recv  chan *updateRecvCall
	close chan chan<- error
}

func (f *fakeWorkspaceUpdateClient) Close() error {
	f.t.Helper()
	errs := make(chan error)
	select {
	case <-f.ctx.Done():
		return errTimeoutOnFake
	case f.close <- errs:
		// OK
	}
	select {
	case <-f.ctx.Done():
		return errTimeoutOnFake
	case err := <-errs:
		return err
	}
}

func (f *fakeWorkspaceUpdateClient) Recv() (*proto.WorkspaceUpdate, error) {
	f.t.Helper()
	resps := make(chan *proto.WorkspaceUpdate)
	errs := make(chan error)
	call := &updateRecvCall{
		resp: resps,
		err:  errs,
	}
	select {
	case <-f.ctx.Done():
		return nil, errTimeoutOnFake
	case f.recv <- call:
		// OK
	}
	select {
	case <-f.ctx.Done():
		return nil, errTimeoutOnFake
	case err := <-errs:
		return nil, err
	case resp := <-resps:
		return resp, nil
	}
}

func newFakeWorkspaceUpdateClient(ctx context.Context, t testing.TB) *fakeWorkspaceUpdateClient {
	return &fakeWorkspaceUpdateClient{
		ctx:   ctx,
		t:     t,
		recv:  make(chan *updateRecvCall),
		close: make(chan chan<- error),
	}
}

type updateRecvCall struct {
	resp chan<- *proto.WorkspaceUpdate
	err  chan<- error
}

// testUUID returns a UUID with bytes set as b, but shifted 6 bytes so that service prefixes don't
// overwrite them.
func testUUID(b ...byte) uuid.UUID {
	o := uuid.UUID{}
	for i := range b {
		o[i+6] = b[i]
	}
	return o
}

type fakeDNSSetter struct {
	ctx   context.Context
	t     testing.TB
	calls chan *setDNSCall
}

type setDNSCall struct {
	hosts map[dnsname.FQDN][]netip.Addr
	err   chan<- error
}

func newFakeDNSSetter(ctx context.Context, t testing.TB) *fakeDNSSetter {
	return &fakeDNSSetter{
		ctx:   ctx,
		t:     t,
		calls: make(chan *setDNSCall),
	}
}

func (f *fakeDNSSetter) SetDNSHosts(hosts map[dnsname.FQDN][]netip.Addr) error {
	f.t.Helper()
	errs := make(chan error)
	call := &setDNSCall{
		hosts: hosts,
		err:   errs,
	}
	select {
	case <-f.ctx.Done():
		return errTimeoutOnFake
	case f.calls <- call:
		// OK
	}
	select {
	case <-f.ctx.Done():
		return errTimeoutOnFake
	case err := <-errs:
		return err
	}
}

func newFakeUpdateHandler(ctx context.Context, t testing.TB) *fakeUpdateHandler {
	return &fakeUpdateHandler{
		ctx: ctx,
		t:   t,
		ch:  make(chan tailnet.WorkspaceUpdate),
	}
}

type fakeUpdateHandler struct {
	ctx context.Context
	t   testing.TB
	ch  chan tailnet.WorkspaceUpdate
}

func (f *fakeUpdateHandler) Update(wu tailnet.WorkspaceUpdate) error {
	f.t.Helper()
	select {
	case <-f.ctx.Done():
		return errTimeoutOnFake
	case f.ch <- wu:
		// OK
	}
	return nil
}

func setupConnectedAllWorkspaceUpdatesController(
	ctx context.Context, t testing.TB, logger slog.Logger, opts ...tailnet.TunnelAllOption,
) (
	*fakeCoordinatorClient, *fakeWorkspaceUpdateClient, *tailnet.TunnelAllWorkspaceUpdatesController,
) {
	fConn := &fakeCoordinatee{}
	tsc := tailnet.NewTunnelSrcCoordController(logger, fConn)
	uut := tailnet.NewTunnelAllWorkspaceUpdatesController(logger, tsc, opts...)

	// connect up a coordinator client, to track adding and removing tunnels
	coordC := newFakeCoordinatorClient(ctx, t)
	coordCW := tsc.New(coordC)
	t.Cleanup(func() {
		// hang up coord client
		coordRecv := testutil.TryReceive(ctx, t, coordC.resps)
		testutil.RequireSend(ctx, t, coordRecv.err, io.EOF)
		// sends close on client
		cCall := testutil.TryReceive(ctx, t, coordC.close)
		testutil.RequireSend(ctx, t, cCall, nil)
		err := testutil.TryReceive(ctx, t, coordCW.Wait())
		require.ErrorIs(t, err, io.EOF)
	})

	// connect up the updates client
	updateC := newFakeWorkspaceUpdateClient(ctx, t)
	updateCW := uut.New(updateC)
	t.Cleanup(func() {
		// hang up WorkspaceUpdates client
		upRecvCall := testutil.TryReceive(ctx, t, updateC.recv)
		testutil.RequireSend(ctx, t, upRecvCall.err, io.EOF)
		err := testutil.TryReceive(ctx, t, updateCW.Wait())
		require.ErrorIs(t, err, io.EOF)
	})
	return coordC, updateC, uut
}

func TestTunnelAllWorkspaceUpdatesController_Initial(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)

	fUH := newFakeUpdateHandler(ctx, t)
	fDNS := newFakeDNSSetter(ctx, t)
	coordC, updateC, updateCtrl := setupConnectedAllWorkspaceUpdatesController(ctx, t, logger,
		tailnet.WithDNS(fDNS, "testy", tailnet.DNSNameOptions{Suffix: "mctest"}),
		tailnet.WithHandler(fUH),
	)

	// Initial update contains 2 workspaces with 1 & 2 agents, respectively
	w1ID := testUUID(1)
	w2ID := testUUID(2)
	w1a1ID := testUUID(1, 1)
	w2a1ID := testUUID(2, 1)
	w2a2ID := testUUID(2, 2)
	initUp := &proto.WorkspaceUpdate{
		UpsertedWorkspaces: []*proto.Workspace{
			{Id: w1ID[:], Name: "w1"},
			{Id: w2ID[:], Name: "w2"},
		},
		UpsertedAgents: []*proto.Agent{
			{Id: w1a1ID[:], Name: "w1a1", WorkspaceId: w1ID[:]},
			{Id: w2a1ID[:], Name: "w2a1", WorkspaceId: w2ID[:]},
			{Id: w2a2ID[:], Name: "w2a2", WorkspaceId: w2ID[:]},
		},
	}

	upRecvCall := testutil.TryReceive(ctx, t, updateC.recv)
	testutil.RequireSend(ctx, t, upRecvCall.resp, initUp)

	// This should trigger AddTunnel for each agent
	var adds []uuid.UUID
	for range 3 {
		coordCall := testutil.TryReceive(ctx, t, coordC.reqs)
		adds = append(adds, uuid.Must(uuid.FromBytes(coordCall.req.GetAddTunnel().GetId())))
		testutil.RequireSend(ctx, t, coordCall.err, nil)
	}
	require.Contains(t, adds, w1a1ID)
	require.Contains(t, adds, w2a1ID)
	require.Contains(t, adds, w2a2ID)

	ws1a1IP := netip.MustParseAddr("fd60:627a:a42b:0101::")
	w2a1IP := netip.MustParseAddr("fd60:627a:a42b:0201::")
	w2a2IP := netip.MustParseAddr("fd60:627a:a42b:0202::")

	expectedCoderConnectFQDN, err := dnsname.ToFQDN(fmt.Sprintf(tailnet.IsCoderConnectEnabledFmtString, "mctest"))
	require.NoError(t, err)

	// Also triggers setting DNS hosts
	expectedDNS := map[dnsname.FQDN][]netip.Addr{
		"w1a1.w1.me.mctest.":     {ws1a1IP},
		"w2a1.w2.me.mctest.":     {w2a1IP},
		"w2a2.w2.me.mctest.":     {w2a2IP},
		"w1a1.w1.testy.mctest.":  {ws1a1IP},
		"w2a1.w2.testy.mctest.":  {w2a1IP},
		"w2a2.w2.testy.mctest.":  {w2a2IP},
		"w1.mctest.":             {ws1a1IP},
		expectedCoderConnectFQDN: {tsaddr.CoderServiceIPv6()},
	}
	dnsCall := testutil.TryReceive(ctx, t, fDNS.calls)
	require.Equal(t, expectedDNS, dnsCall.hosts)
	testutil.RequireSend(ctx, t, dnsCall.err, nil)

	currentState := tailnet.WorkspaceUpdate{
		UpsertedWorkspaces: []*tailnet.Workspace{
			{ID: w1ID, Name: "w1"},
			{ID: w2ID, Name: "w2"},
		},
		UpsertedAgents: []*tailnet.Agent{
			{
				ID: w1a1ID, Name: "w1a1", WorkspaceID: w1ID,
				Hosts: map[dnsname.FQDN][]netip.Addr{
					"w1.mctest.":            {ws1a1IP},
					"w1a1.w1.me.mctest.":    {ws1a1IP},
					"w1a1.w1.testy.mctest.": {ws1a1IP},
				},
			},
			{
				ID: w2a1ID, Name: "w2a1", WorkspaceID: w2ID,
				Hosts: map[dnsname.FQDN][]netip.Addr{
					"w2a1.w2.me.mctest.":    {w2a1IP},
					"w2a1.w2.testy.mctest.": {w2a1IP},
				},
			},
			{
				ID: w2a2ID, Name: "w2a2", WorkspaceID: w2ID,
				Hosts: map[dnsname.FQDN][]netip.Addr{
					"w2a2.w2.me.mctest.":    {w2a2IP},
					"w2a2.w2.testy.mctest.": {w2a2IP},
				},
			},
		},
		DeletedWorkspaces: []*tailnet.Workspace{},
		DeletedAgents:     []*tailnet.Agent{},
		Kind:              tailnet.Snapshot,
	}

	// And the callback
	cbUpdate := testutil.TryReceive(ctx, t, fUH.ch)
	require.Equal(t, currentState, cbUpdate)

	// Current recvState should match
	recvState, err := updateCtrl.CurrentState()
	require.NoError(t, err)
	slices.SortFunc(recvState.UpsertedWorkspaces, func(a, b *tailnet.Workspace) int {
		return strings.Compare(a.Name, b.Name)
	})
	slices.SortFunc(recvState.UpsertedAgents, func(a, b *tailnet.Agent) int {
		return strings.Compare(a.Name, b.Name)
	})
	// tunnel is still open, so it's a diff
	currentState.Kind = tailnet.Diff

	require.Equal(t, currentState, recvState)
}

func TestTunnelAllWorkspaceUpdatesController_DeleteAgent(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)

	fUH := newFakeUpdateHandler(ctx, t)
	fDNS := newFakeDNSSetter(ctx, t)
	coordC, updateC, updateCtrl := setupConnectedAllWorkspaceUpdatesController(ctx, t, logger,
		tailnet.WithDNS(fDNS, "testy", tailnet.DNSNameOptions{Suffix: tailnet.CoderDNSSuffix}),
		tailnet.WithHandler(fUH),
	)

	w1ID := testUUID(1)
	w1a1ID := testUUID(1, 1)
	w1a2ID := testUUID(1, 2)
	ws1a1IP := netip.MustParseAddr("fd60:627a:a42b:0101::")
	ws1a2IP := netip.MustParseAddr("fd60:627a:a42b:0102::")

	initUp := &proto.WorkspaceUpdate{
		UpsertedWorkspaces: []*proto.Workspace{
			{Id: w1ID[:], Name: "w1"},
		},
		UpsertedAgents: []*proto.Agent{
			{Id: w1a1ID[:], Name: "w1a1", WorkspaceId: w1ID[:]},
		},
	}

	upRecvCall := testutil.TryReceive(ctx, t, updateC.recv)
	testutil.RequireSend(ctx, t, upRecvCall.resp, initUp)

	// Add for w1a1
	coordCall := testutil.TryReceive(ctx, t, coordC.reqs)
	require.Equal(t, w1a1ID[:], coordCall.req.GetAddTunnel().GetId())
	testutil.RequireSend(ctx, t, coordCall.err, nil)

	expectedCoderConnectFQDN, err := dnsname.ToFQDN(
		fmt.Sprintf(tailnet.IsCoderConnectEnabledFmtString, tailnet.CoderDNSSuffix))
	require.NoError(t, err)

	// DNS for w1a1
	expectedDNS := map[dnsname.FQDN][]netip.Addr{
		"w1a1.w1.testy.coder.":   {ws1a1IP},
		"w1a1.w1.me.coder.":      {ws1a1IP},
		"w1.coder.":              {ws1a1IP},
		expectedCoderConnectFQDN: {tsaddr.CoderServiceIPv6()},
	}
	dnsCall := testutil.TryReceive(ctx, t, fDNS.calls)
	require.Equal(t, expectedDNS, dnsCall.hosts)
	testutil.RequireSend(ctx, t, dnsCall.err, nil)

	initRecvUp := tailnet.WorkspaceUpdate{
		UpsertedWorkspaces: []*tailnet.Workspace{
			{ID: w1ID, Name: "w1"},
		},
		UpsertedAgents: []*tailnet.Agent{
			{ID: w1a1ID, Name: "w1a1", WorkspaceID: w1ID, Hosts: map[dnsname.FQDN][]netip.Addr{
				"w1a1.w1.testy.coder.": {ws1a1IP},
				"w1a1.w1.me.coder.":    {ws1a1IP},
				"w1.coder.":            {ws1a1IP},
			}},
		},
		DeletedWorkspaces: []*tailnet.Workspace{},
		DeletedAgents:     []*tailnet.Agent{},
		Kind:              tailnet.Snapshot,
	}

	cbUpdate := testutil.TryReceive(ctx, t, fUH.ch)
	require.Equal(t, initRecvUp, cbUpdate)

	state, err := updateCtrl.CurrentState()
	require.NoError(t, err)
	// tunnel is still open, so it's a diff
	initRecvUp.Kind = tailnet.Diff

	require.Equal(t, initRecvUp, state)

	// Send update that removes w1a1 and adds w1a2
	agentUpdate := &proto.WorkspaceUpdate{
		UpsertedAgents: []*proto.Agent{
			{Id: w1a2ID[:], Name: "w1a2", WorkspaceId: w1ID[:]},
		},
		DeletedAgents: []*proto.Agent{
			{Id: w1a1ID[:], WorkspaceId: w1ID[:]},
		},
	}
	upRecvCall = testutil.TryReceive(ctx, t, updateC.recv)
	testutil.RequireSend(ctx, t, upRecvCall.resp, agentUpdate)

	// Add for w1a2
	coordCall = testutil.TryReceive(ctx, t, coordC.reqs)
	require.Equal(t, w1a2ID[:], coordCall.req.GetAddTunnel().GetId())
	testutil.RequireSend(ctx, t, coordCall.err, nil)

	// Remove for w1a1
	coordCall = testutil.TryReceive(ctx, t, coordC.reqs)
	require.Equal(t, w1a1ID[:], coordCall.req.GetRemoveTunnel().GetId())
	testutil.RequireSend(ctx, t, coordCall.err, nil)

	// DNS contains only w1a2
	expectedDNS = map[dnsname.FQDN][]netip.Addr{
		"w1a2.w1.testy.coder.":   {ws1a2IP},
		"w1a2.w1.me.coder.":      {ws1a2IP},
		"w1.coder.":              {ws1a2IP},
		expectedCoderConnectFQDN: {tsaddr.CoderServiceIPv6()},
	}
	dnsCall = testutil.TryReceive(ctx, t, fDNS.calls)
	require.Equal(t, expectedDNS, dnsCall.hosts)
	testutil.RequireSend(ctx, t, dnsCall.err, nil)

	cbUpdate = testutil.TryReceive(ctx, t, fUH.ch)
	sndRecvUpdate := tailnet.WorkspaceUpdate{
		UpsertedWorkspaces: []*tailnet.Workspace{},
		UpsertedAgents: []*tailnet.Agent{
			{ID: w1a2ID, Name: "w1a2", WorkspaceID: w1ID, Hosts: map[dnsname.FQDN][]netip.Addr{
				"w1a2.w1.testy.coder.": {ws1a2IP},
				"w1a2.w1.me.coder.":    {ws1a2IP},
				"w1.coder.":            {ws1a2IP},
			}},
		},
		DeletedWorkspaces: []*tailnet.Workspace{},
		DeletedAgents: []*tailnet.Agent{
			{ID: w1a1ID, Name: "w1a1", WorkspaceID: w1ID, Hosts: map[dnsname.FQDN][]netip.Addr{
				"w1a1.w1.testy.coder.": {ws1a1IP},
				"w1a1.w1.me.coder.":    {ws1a1IP},
				"w1.coder.":            {ws1a1IP},
			}},
		},
	}
	require.Equal(t, sndRecvUpdate, cbUpdate)

	state, err = updateCtrl.CurrentState()
	require.NoError(t, err)
	require.Equal(t, tailnet.WorkspaceUpdate{
		UpsertedWorkspaces: []*tailnet.Workspace{
			{ID: w1ID, Name: "w1"},
		},
		UpsertedAgents: []*tailnet.Agent{
			{ID: w1a2ID, Name: "w1a2", WorkspaceID: w1ID, Hosts: map[dnsname.FQDN][]netip.Addr{
				"w1a2.w1.testy.coder.": {ws1a2IP},
				"w1a2.w1.me.coder.":    {ws1a2IP},
				"w1.coder.":            {ws1a2IP},
			}},
		},
		DeletedWorkspaces: []*tailnet.Workspace{},
		DeletedAgents:     []*tailnet.Agent{},
	}, state)
}

func TestTunnelAllWorkspaceUpdatesController_DNSError(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	dnsError := xerrors.New("a bad thing happened")
	logger := slogtest.Make(t,
		&slogtest.Options{IgnoredErrorIs: []error{dnsError}}).
		Leveled(slog.LevelDebug)

	fDNS := newFakeDNSSetter(ctx, t)
	fConn := &fakeCoordinatee{}
	tsc := tailnet.NewTunnelSrcCoordController(logger, fConn)
	uut := tailnet.NewTunnelAllWorkspaceUpdatesController(logger, tsc,
		tailnet.WithDNS(fDNS, "testy", tailnet.DNSNameOptions{Suffix: tailnet.CoderDNSSuffix}),
	)

	updateC := newFakeWorkspaceUpdateClient(ctx, t)
	updateCW := uut.New(updateC)

	w1ID := testUUID(1)
	w1a1ID := testUUID(1, 1)
	ws1a1IP := netip.MustParseAddr("fd60:627a:a42b:0101::")

	initUp := &proto.WorkspaceUpdate{
		UpsertedWorkspaces: []*proto.Workspace{
			{Id: w1ID[:], Name: "w1"},
		},
		UpsertedAgents: []*proto.Agent{
			{Id: w1a1ID[:], Name: "w1a1", WorkspaceId: w1ID[:]},
		},
	}
	upRecvCall := testutil.TryReceive(ctx, t, updateC.recv)
	testutil.RequireSend(ctx, t, upRecvCall.resp, initUp)

	expectedCoderConnectFQDN, err := dnsname.ToFQDN(
		fmt.Sprintf(tailnet.IsCoderConnectEnabledFmtString, tailnet.CoderDNSSuffix))
	require.NoError(t, err)

	// DNS for w1a1
	expectedDNS := map[dnsname.FQDN][]netip.Addr{
		"w1a1.w1.me.coder.":      {ws1a1IP},
		"w1a1.w1.testy.coder.":   {ws1a1IP},
		"w1.coder.":              {ws1a1IP},
		expectedCoderConnectFQDN: {tsaddr.CoderServiceIPv6()},
	}
	dnsCall := testutil.TryReceive(ctx, t, fDNS.calls)
	require.Equal(t, expectedDNS, dnsCall.hosts)
	testutil.RequireSend(ctx, t, dnsCall.err, dnsError)

	// should trigger a close on the client
	closeCall := testutil.TryReceive(ctx, t, updateC.close)
	testutil.RequireSend(ctx, t, closeCall, io.EOF)

	// error should be our initial DNS error
	err = testutil.TryReceive(ctx, t, updateCW.Wait())
	require.ErrorIs(t, err, dnsError)
}

func TestTunnelAllWorkspaceUpdatesController_HandleErrors(t *testing.T) {
	t.Parallel()
	validWorkspaceID := testUUID(1)
	validAgentID := testUUID(1, 1)

	testCases := []struct {
		name          string
		update        *proto.WorkspaceUpdate
		errorContains string
	}{
		{
			name: "unparsableUpsertWorkspaceID",
			update: &proto.WorkspaceUpdate{
				UpsertedWorkspaces: []*proto.Workspace{
					{Id: []byte{2, 2}, Name: "bander"},
				},
			},
			errorContains: "failed to parse workspace ID",
		},
		{
			name: "unparsableDeleteWorkspaceID",
			update: &proto.WorkspaceUpdate{
				DeletedWorkspaces: []*proto.Workspace{
					{Id: []byte{2, 2}, Name: "bander"},
				},
			},
			errorContains: "failed to parse workspace ID",
		},
		{
			name: "unparsableDeleteAgentWorkspaceID",
			update: &proto.WorkspaceUpdate{
				DeletedAgents: []*proto.Agent{
					{Id: validAgentID[:], Name: "devo", WorkspaceId: []byte{2, 2}},
				},
			},
			errorContains: "failed to parse workspace ID",
		},
		{
			name: "unparsableUpsertAgentWorkspaceID",
			update: &proto.WorkspaceUpdate{
				UpsertedAgents: []*proto.Agent{
					{Id: validAgentID[:], Name: "devo", WorkspaceId: []byte{2, 2}},
				},
			},
			errorContains: "failed to parse workspace ID",
		},
		{
			name: "unparsableDeleteAgentID",
			update: &proto.WorkspaceUpdate{
				DeletedAgents: []*proto.Agent{
					{Id: []byte{2, 2}, Name: "devo", WorkspaceId: validWorkspaceID[:]},
				},
			},
			errorContains: "failed to parse agent ID",
		},
		{
			name: "unparsableUpsertAgentID",
			update: &proto.WorkspaceUpdate{
				UpsertedAgents: []*proto.Agent{
					{Id: []byte{2, 2}, Name: "devo", WorkspaceId: validWorkspaceID[:]},
				},
			},
			errorContains: "failed to parse agent ID",
		},
		{
			name: "upsertAgentMissingWorkspace",
			update: &proto.WorkspaceUpdate{
				UpsertedAgents: []*proto.Agent{
					{Id: validAgentID[:], Name: "devo", WorkspaceId: validWorkspaceID[:]},
				},
			},
			errorContains: fmt.Sprintf("workspace %s not found", validWorkspaceID.String()),
		},
		{
			name: "deleteAgentMissingWorkspace",
			update: &proto.WorkspaceUpdate{
				DeletedAgents: []*proto.Agent{
					{Id: validAgentID[:], Name: "devo", WorkspaceId: validWorkspaceID[:]},
				},
			},
			errorContains: fmt.Sprintf("workspace %s not found", validWorkspaceID.String()),
		},
	}
	// nolint: paralleltest // no longer need to reinitialize loop vars in go 1.22
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

			fConn := &fakeCoordinatee{}
			tsc := tailnet.NewTunnelSrcCoordController(logger, fConn)
			uut := tailnet.NewTunnelAllWorkspaceUpdatesController(logger, tsc)
			updateC := newFakeWorkspaceUpdateClient(ctx, t)
			updateCW := uut.New(updateC)

			recvCall := testutil.TryReceive(ctx, t, updateC.recv)
			testutil.RequireSend(ctx, t, recvCall.resp, tc.update)
			closeCall := testutil.TryReceive(ctx, t, updateC.close)
			testutil.RequireSend(ctx, t, closeCall, nil)

			err := testutil.TryReceive(ctx, t, updateCW.Wait())
			require.ErrorContains(t, err, tc.errorContains)
		})
	}
}

type fakeWorkspaceUpdatesController struct {
	ctx   context.Context
	t     testing.TB
	calls chan *newWorkspaceUpdatesCall
}

func (*fakeWorkspaceUpdatesController) CurrentState() *proto.WorkspaceUpdate {
	panic("unimplemented")
}

type newWorkspaceUpdatesCall struct {
	client tailnet.WorkspaceUpdatesClient
	resp   chan<- tailnet.CloserWaiter
}

func (f fakeWorkspaceUpdatesController) New(client tailnet.WorkspaceUpdatesClient) tailnet.CloserWaiter {
	resps := make(chan tailnet.CloserWaiter)
	call := &newWorkspaceUpdatesCall{
		client: client,
		resp:   resps,
	}
	select {
	case <-f.ctx.Done():
		cw := newFakeCloserWaiter()
		cw.errCh <- errTimeoutOnFake
		return cw
	case f.calls <- call:
		// OK
	}
	select {
	case <-f.ctx.Done():
		cw := newFakeCloserWaiter()
		cw.errCh <- errTimeoutOnFake
		return cw
	case resp := <-resps:
		return resp
	}
}

func newFakeUpdatesController(ctx context.Context, t *testing.T) *fakeWorkspaceUpdatesController {
	return &fakeWorkspaceUpdatesController{
		ctx:   ctx,
		t:     t,
		calls: make(chan *newWorkspaceUpdatesCall),
	}
}

type fakeCloserWaiter struct {
	closeCalls chan chan error
	errCh      chan error
}

func (f *fakeCloserWaiter) Close(ctx context.Context) error {
	errRes := make(chan error)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case f.closeCalls <- errRes:
		// OK
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errRes:
		return err
	}
}

func (f *fakeCloserWaiter) Wait() <-chan error {
	return f.errCh
}

func newFakeCloserWaiter() *fakeCloserWaiter {
	return &fakeCloserWaiter{
		closeCalls: make(chan chan error),
		errCh:      make(chan error, 1),
	}
}

type fakeWorkspaceUpdatesDialer struct {
	client tailnet.WorkspaceUpdatesClient
}

func (f *fakeWorkspaceUpdatesDialer) Dial(_ context.Context, _ tailnet.ResumeTokenController) (tailnet.ControlProtocolClients, error) {
	return tailnet.ControlProtocolClients{
		WorkspaceUpdates: f.client,
		Closer:           fakeCloser{},
	}, nil
}

type fakeCloser struct{}

func (fakeCloser) Close() error {
	return nil
}
