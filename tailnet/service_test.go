package tailnet_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"testing"

	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/tailnet"
)

func TestValidateVersion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		version   string
		supported bool
	}{
		{
			name:      "Current",
			version:   fmt.Sprintf("%d.%d", tailnet.CurrentMajor, tailnet.CurrentMinor),
			supported: true,
		},
		{
			name:    "TooNewMinor",
			version: fmt.Sprintf("%d.%d", tailnet.CurrentMajor, tailnet.CurrentMinor+1),
		},
		{
			name:    "TooNewMajor",
			version: fmt.Sprintf("%d.%d", tailnet.CurrentMajor+1, tailnet.CurrentMinor),
		},
		{
			name:      "1.0",
			version:   "1.0",
			supported: true,
		},
		{
			name:      "2.0",
			version:   "2.0",
			supported: true,
		},
		{
			name:    "Malformed0",
			version: "cats",
		},
		{
			name:    "Malformed1",
			version: "cats.dogs",
		},
		{
			name:    "Malformed2",
			version: "1.0.1",
		},
		{
			name:    "Malformed3",
			version: "11",
		},
		{
			name:    "TooOld",
			version: "0.8",
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tailnet.ValidateVersion(tc.version)
			if tc.supported {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestClientService_ServeClient_V2(t *testing.T) {
	t.Parallel()
	fCoord := newFakeCoordinator()
	var coord tailnet.Coordinator = fCoord
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coord)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	uut, err := tailnet.NewClientService(logger, &coordPtr)
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
	stream, err := client.CoordinateTailnet(ctx)
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
			Uuid: agentID[:],
		},
	}}
	resp, err := stream.Recv()
	require.NoError(t, err)
	u := resp.GetPeerUpdates()
	require.Len(t, u, 1)
	require.Equal(t, int32(22), u[0].GetNode().GetPreferredDerp())

	err = stream.Close()
	require.NoError(t, err)

	// stream ^^ is just one RPC; we need to close the Conn to end the session.
	err = c.Close()
	require.NoError(t, err)
	err = testutil.RequireRecvCtx(ctx, t, errCh)
	require.ErrorIs(t, err, io.EOF)
}

func TestClientService_ServeClient_V1(t *testing.T) {
	t.Parallel()
	fCoord := newFakeCoordinator()
	var coord tailnet.Coordinator = fCoord
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coord)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	uut, err := tailnet.NewClientService(logger, &coordPtr)
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
