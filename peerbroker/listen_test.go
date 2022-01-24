package peerbroker_test

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"storj.io/drpc/drpcconn"

	"github.com/coder/coder/peerbroker"
	"github.com/coder/coder/peerbroker/proto"
	"github.com/coder/coder/provisionersdk"
)

func TestListen(t *testing.T) {
	t.Parallel()
	// Ensures connections blocked on Accept() are
	// closed if the listener is.
	t.Run("NoAcceptClosed", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		client, server := provisionersdk.TransportPipe()
		defer client.Close()
		defer server.Close()

		listener, err := peerbroker.Listen(server, nil)
		require.NoError(t, err)

		api := proto.NewDRPCPeerBrokerClient(drpcconn.New(client))
		stream, err := api.NegotiateConnection(ctx)
		require.NoError(t, err)
		clientConn, err := peerbroker.Dial(stream, nil, nil)
		require.NoError(t, err)
		defer clientConn.Close()

		_ = listener.Close()
	})

	// Ensures Accept() properly exits when Close() is called.
	t.Run("AcceptClosed", func(t *testing.T) {
		t.Parallel()
		client, server := provisionersdk.TransportPipe()
		defer client.Close()
		defer server.Close()

		listener, err := peerbroker.Listen(server, nil)
		require.NoError(t, err)
		go listener.Close()
		_, err = listener.Accept()
		require.ErrorIs(t, err, io.EOF)
	})
}
