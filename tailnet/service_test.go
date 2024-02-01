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
	fCoord := NewFakeCoordinator()
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
	require.True(t, call.Auth.Authorize(agentID))
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

	// RPCs closed; we need to close the Conn to end the session.
	err = c.Close()
	require.NoError(t, err)
	err = testutil.RequireRecvCtx(ctx, t, errCh)
	require.True(t, xerrors.Is(err, io.EOF) || xerrors.Is(err, io.ErrClosedPipe))
}

func TestClientService_ServeClient_V1(t *testing.T) {
	t.Parallel()
	fCoord := NewFakeCoordinator()
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

	call := testutil.RequireRecvCtx(ctx, t, fCoord.ServeClientCalls)
	require.NotNil(t, call)
	require.Equal(t, call.ID, clientID)
	require.Equal(t, call.Agent, agentID)
	require.Equal(t, s, call.Conn)
	expectedError := xerrors.New("test error")
	select {
	case call.ErrCh <- expectedError:
	// ok!
	case <-ctx.Done():
		t.Fatalf("timeout sending error")
	}

	err = testutil.RequireRecvCtx(ctx, t, errCh)
	require.ErrorIs(t, err, expectedError)
}

type FakeCoordinator struct {
	CoordinateCalls  chan *FakeCoordinate
	ServeClientCalls chan *FakeServeClient
}

func (*FakeCoordinator) ServeHTTPDebug(http.ResponseWriter, *http.Request) {
	panic("unimplemented")
}

func (*FakeCoordinator) Node(uuid.UUID) *tailnet.Node {
	panic("unimplemented")
}

func (f *FakeCoordinator) ServeClient(conn net.Conn, id uuid.UUID, agent uuid.UUID) error {
	errCh := make(chan error)
	f.ServeClientCalls <- &FakeServeClient{
		Conn:  conn,
		ID:    id,
		Agent: agent,
		ErrCh: errCh,
	}
	return <-errCh
}

func (*FakeCoordinator) ServeAgent(net.Conn, uuid.UUID, string) error {
	panic("unimplemented")
}

func (*FakeCoordinator) Close() error {
	panic("unimplemented")
}

func (*FakeCoordinator) ServeMultiAgent(uuid.UUID) tailnet.MultiAgentConn {
	panic("unimplemented")
}

func (f *FakeCoordinator) Coordinate(ctx context.Context, id uuid.UUID, name string, a tailnet.TunnelAuth) (chan<- *proto.CoordinateRequest, <-chan *proto.CoordinateResponse) {
	reqs := make(chan *proto.CoordinateRequest, 100)
	resps := make(chan *proto.CoordinateResponse, 100)
	f.CoordinateCalls <- &FakeCoordinate{
		Ctx:   ctx,
		ID:    id,
		Name:  name,
		Auth:  a,
		Reqs:  reqs,
		Resps: resps,
	}
	return reqs, resps
}

func NewFakeCoordinator() *FakeCoordinator {
	return &FakeCoordinator{
		CoordinateCalls:  make(chan *FakeCoordinate, 100),
		ServeClientCalls: make(chan *FakeServeClient, 100),
	}
}

type FakeCoordinate struct {
	Ctx   context.Context
	ID    uuid.UUID
	Name  string
	Auth  tailnet.TunnelAuth
	Reqs  chan *proto.CoordinateRequest
	Resps chan *proto.CoordinateResponse
}

type FakeServeClient struct {
	Conn  net.Conn
	ID    uuid.UUID
	Agent uuid.UUID
	ErrCh chan error
}
