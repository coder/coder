package tailnet_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	"github.com/google/uuid"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/tailnet"
)

func TestClientService_ServeClient_V2(t *testing.T) {
	t.Parallel()
	fCoord := newFakeCoordinator()
	var coord tailnet.Coordinator = fCoord
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coord)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	derpMap := &tailcfg.DERPMap{Regions: map[int]*tailcfg.DERPRegion{999: {RegionCode: "test"}}}
	uut, err := tailnet.NewClientService(
		logger, &coordPtr,
		time.Millisecond, func() *tailcfg.DERPMap { return derpMap },
	)
	require.NoError(t, err)

	ctx := testutil.Context(t, testutil.WaitShort)
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()
	clientID := uuid.MustParse("10000001-0000-0000-0000-000000000000")
	agentID := uuid.MustParse("20000001-0000-0000-0000-000000000000")
	errCh := make(chan error, 1)
	go func() {
		err := uut.ServeClient(ctx, "2.0", s, clientID, agentID)
		t.Logf("ServeClient returned; err=%v", err)
		errCh <- err
	}()

	client, err := tailnet.NewDRPCClient(c)
	require.NoError(t, err)

	// Coordinate
	stream, err := client.Coordinate(ctx)
	require.NoError(t, err)
	defer stream.Close()

	err = stream.Send(&proto.CoordinateRequest{
		UpdateSelf: &proto.CoordinateRequest_UpdateSelf{Node: &proto.Node{PreferredDerp: 11}},
	})
	require.NoError(t, err)

	call := testutil.RequireRecvCtx(ctx, t, fCoord.coordinateCalls)
	require.NotNil(t, call)
	require.Equal(t, call.id, clientID)
	require.Equal(t, call.name, "client")
	require.True(t, call.auth.Authorize(agentID))
	req := testutil.RequireRecvCtx(ctx, t, call.reqs)
	require.Equal(t, int32(11), req.GetUpdateSelf().GetNode().GetPreferredDerp())

	call.resps <- &proto.CoordinateResponse{PeerUpdates: []*proto.CoordinateResponse_PeerUpdate{
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

	// RPCs closed; we need to close the Conn to end the session.
	err = c.Close()
	require.NoError(t, err)
	err = testutil.RequireRecvCtx(ctx, t, errCh)
	require.True(t, xerrors.Is(err, io.EOF) || xerrors.Is(err, io.ErrClosedPipe))
}

func TestClientService_ServeClient_V1(t *testing.T) {
	t.Parallel()
	fCoord := newFakeCoordinator()
	var coord tailnet.Coordinator = fCoord
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coord)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	uut, err := tailnet.NewClientService(logger, &coordPtr, 0, nil)
	require.NoError(t, err)

	ctx := testutil.Context(t, testutil.WaitShort)
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()
	clientID := uuid.MustParse("10000001-0000-0000-0000-000000000000")
	agentID := uuid.MustParse("20000001-0000-0000-0000-000000000000")
	errCh := make(chan error, 1)
	go func() {
		err := uut.ServeClient(ctx, "1.0", s, clientID, agentID)
		t.Logf("ServeClient returned; err=%v", err)
		errCh <- err
	}()

	call := testutil.RequireRecvCtx(ctx, t, fCoord.serveClientCalls)
	require.NotNil(t, call)
	require.Equal(t, call.id, clientID)
	require.Equal(t, call.agent, agentID)
	require.Equal(t, s, call.conn)
	expectedError := xerrors.New("test error")
	select {
	case call.errCh <- expectedError:
	// ok!
	case <-ctx.Done():
		t.Fatalf("timeout sending error")
	}

	err = testutil.RequireRecvCtx(ctx, t, errCh)
	require.ErrorIs(t, err, expectedError)
}

type fakeCoordinator struct {
	coordinateCalls  chan *fakeCoordinate
	serveClientCalls chan *fakeServeClient
}

func (*fakeCoordinator) ServeHTTPDebug(http.ResponseWriter, *http.Request) {
	panic("unimplemented")
}

func (*fakeCoordinator) Node(uuid.UUID) *tailnet.Node {
	panic("unimplemented")
}

func (f *fakeCoordinator) ServeClient(conn net.Conn, id uuid.UUID, agent uuid.UUID) error {
	errCh := make(chan error)
	f.serveClientCalls <- &fakeServeClient{
		conn:  conn,
		id:    id,
		agent: agent,
		errCh: errCh,
	}
	return <-errCh
}

func (*fakeCoordinator) ServeAgent(net.Conn, uuid.UUID, string) error {
	panic("unimplemented")
}

func (*fakeCoordinator) Close() error {
	panic("unimplemented")
}

func (*fakeCoordinator) ServeMultiAgent(uuid.UUID) tailnet.MultiAgentConn {
	panic("unimplemented")
}

func (f *fakeCoordinator) Coordinate(ctx context.Context, id uuid.UUID, name string, a tailnet.TunnelAuth) (chan<- *proto.CoordinateRequest, <-chan *proto.CoordinateResponse) {
	reqs := make(chan *proto.CoordinateRequest, 100)
	resps := make(chan *proto.CoordinateResponse, 100)
	f.coordinateCalls <- &fakeCoordinate{
		ctx:   ctx,
		id:    id,
		name:  name,
		auth:  a,
		reqs:  reqs,
		resps: resps,
	}
	return reqs, resps
}

func newFakeCoordinator() *fakeCoordinator {
	return &fakeCoordinator{
		coordinateCalls:  make(chan *fakeCoordinate, 100),
		serveClientCalls: make(chan *fakeServeClient, 100),
	}
}

type fakeCoordinate struct {
	ctx   context.Context
	id    uuid.UUID
	name  string
	auth  tailnet.TunnelAuth
	reqs  chan *proto.CoordinateRequest
	resps chan *proto.CoordinateResponse
}

type fakeServeClient struct {
	conn  net.Conn
	id    uuid.UUID
	agent uuid.UUID
	errCh chan error
}
