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
	"golang.org/x/xerrors"
	"storj.io/drpc"
	"storj.io/drpc/drpcerr"
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

func TestNewBasicDERPController_Mainline(t *testing.T) {
	t.Parallel()
	fs := make(chan *tailcfg.DERPMap)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	uut := tailnet.NewBasicDERPController(logger, fakeSetter(fs))
	fc := fakeDERPClient{
		ch: make(chan *tailcfg.DERPMap),
	}
	c := uut.New(fc)
	ctx := testutil.Context(t, testutil.WaitShort)
	expectDM := &tailcfg.DERPMap{}
	testutil.RequireSendCtx(ctx, t, fc.ch, expectDM)
	gotDM := testutil.RequireRecvCtx(ctx, t, fs)
	require.Equal(t, expectDM, gotDM)
	err := c.Close(ctx)
	require.NoError(t, err)
	err = testutil.RequireRecvCtx(ctx, t, c.Wait())
	require.ErrorIs(t, err, io.EOF)
	// ensure Close is idempotent
	err = c.Close(ctx)
	require.NoError(t, err)
}

func TestNewBasicDERPController_RecvErr(t *testing.T) {
	t.Parallel()
	fs := make(chan *tailcfg.DERPMap)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	uut := tailnet.NewBasicDERPController(logger, fakeSetter(fs))
	expectedErr := xerrors.New("a bad thing happened")
	fc := fakeDERPClient{
		ch:  make(chan *tailcfg.DERPMap),
		err: expectedErr,
	}
	c := uut.New(fc)
	ctx := testutil.Context(t, testutil.WaitShort)
	err := testutil.RequireRecvCtx(ctx, t, c.Wait())
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
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

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

	call := testutil.RequireRecvCtx(ctx, t, ft.calls)
	require.Len(t, call.req.GetEvents(), 1)
	require.Equal(t, call.req.GetEvents()[0].GetId(), []byte("test event"))

	testutil.RequireSendCtx(ctx, t, call.errCh, nil)
	testutil.RequireRecvCtx(ctx, t, sendDone)
}

func TestBasicTelemetryController_Unimplemented(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

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

	call := testutil.RequireRecvCtx(ctx, t, ft.calls)
	testutil.RequireSendCtx(ctx, t, call.errCh, telemetryError)
	testutil.RequireRecvCtx(ctx, t, sendDone)

	sendDone = make(chan struct{})
	go func() {
		defer close(sendDone)
		uut.SendTelemetryEvent(&proto.TelemetryEvent{})
	}()

	// we get another call since it wasn't really the Unimplemented error
	call = testutil.RequireRecvCtx(ctx, t, ft.calls)

	// for real this time
	telemetryError = drpcerr.WithCode(xerrors.New("Unimplemented"), drpcerr.Unimplemented)
	testutil.RequireSendCtx(ctx, t, call.errCh, telemetryError)
	testutil.RequireRecvCtx(ctx, t, sendDone)

	// now this returns immediately without a call, because unimplemented error disables calling
	sendDone = make(chan struct{})
	go func() {
		defer close(sendDone)
		uut.SendTelemetryEvent(&proto.TelemetryEvent{})
	}()
	testutil.RequireRecvCtx(ctx, t, sendDone)

	// getting a "new" client resets
	uut.New(ft)
	sendDone = make(chan struct{})
	go func() {
		defer close(sendDone)
		uut.SendTelemetryEvent(&proto.TelemetryEvent{})
	}()
	call = testutil.RequireRecvCtx(ctx, t, ft.calls)
	testutil.RequireSendCtx(ctx, t, call.errCh, nil)
	testutil.RequireRecvCtx(ctx, t, sendDone)
}

func TestBasicTelemetryController_NotRecognised(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	ft := newFakeTelemetryClient()
	uut := tailnet.NewBasicTelemetryController(logger)
	uut.New(ft)

	sendDone := make(chan struct{})
	go func() {
		defer close(sendDone)
		uut.SendTelemetryEvent(&proto.TelemetryEvent{})
	}()
	// returning generic protocol error doesn't trigger unknown rpc logic
	call := testutil.RequireRecvCtx(ctx, t, ft.calls)
	testutil.RequireSendCtx(ctx, t, call.errCh, drpc.ProtocolError.New("Protocol Error"))
	testutil.RequireRecvCtx(ctx, t, sendDone)

	sendDone = make(chan struct{})
	go func() {
		defer close(sendDone)
		uut.SendTelemetryEvent(&proto.TelemetryEvent{})
	}()
	call = testutil.RequireRecvCtx(ctx, t, ft.calls)
	// return the expected protocol error this time
	testutil.RequireSendCtx(ctx, t, call.errCh,
		drpc.ProtocolError.New("unknown rpc: /coder.tailnet.v2.Tailnet/PostTelemetry"))
	testutil.RequireRecvCtx(ctx, t, sendDone)

	// now this returns immediately without a call, because unimplemented error disables calling
	sendDone = make(chan struct{})
	go func() {
		defer close(sendDone)
		uut.SendTelemetryEvent(&proto.TelemetryEvent{})
	}()
	testutil.RequireRecvCtx(ctx, t, sendDone)
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
