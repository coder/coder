package tailnet

import (
	"net"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	agpl "github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
)

func TestServeExitNodeClientRejectsLegacyAddress(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)
	coordinator := tailnettest.NewFakeCoordinator()
	var coordinatorInterface agpl.Coordinator = coordinator
	var coordinatorPointer atomic.Pointer[agpl.Coordinator]
	coordinatorPointer.Store(&coordinatorInterface)
	service, err := NewClientService(agpl.ClientServiceOptions{
		Logger:   logger,
		CoordPtr: &coordinatorPointer,
	})
	require.NoError(t, err)

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()
	peerID := uuid.New()
	auth := &ExitNodeCoordinateeAuth{PeerID: peerID}
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- service.ServeExitNodeClient(ctx, "2.0", serverConn, peerID, auth)
	}()

	client, err := agpl.NewDRPCClient(clientConn, logger)
	require.NoError(t, err)
	stream, err := client.Coordinate(ctx)
	require.NoError(t, err)
	defer stream.Close()
	legacyUpdate := &proto.CoordinateRequest{
		UpdateSelf: &proto.CoordinateRequest_UpdateSelf{Node: &proto.Node{
			Addresses: []string{"fd7a:115c:a1e0:49d6:b259:b7ac:b1b2:48f4/128"},
		}},
	}
	require.NoError(t, stream.Send(legacyUpdate))
	call := testutil.TryReceive(ctx, t, coordinator.CoordinateCalls)
	require.Equal(t, peerID, call.ID)
	require.Error(t, call.Auth.Authorize(ctx, legacyUpdate))

	require.NoError(t, clientConn.Close())
	select {
	case <-serveErr:
	case <-ctx.Done():
		t.Fatal("exit node client did not stop")
	}
}
