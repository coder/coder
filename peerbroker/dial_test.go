package peerbroker_test

import (
	"context"
	"testing"

	"github.com/coder/coder/peerbroker"
	"github.com/coder/coder/peerbroker/proto"
	"github.com/coder/coder/provisionersdk"
	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"storj.io/drpc/drpcconn"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestDial(t *testing.T) {
	t.Run("Connect", func(t *testing.T) {
		ctx := context.Background()
		client, server := provisionersdk.TransportPipe()
		defer client.Close()
		defer server.Close()

		listener, err := peerbroker.Listen(server, nil)
		require.NoError(t, err)

		api := proto.NewDRPCPeerBrokerClient(drpcconn.New(client))
		stream, err := api.NegotiateConnection(ctx)
		require.NoError(t, err)
		clientConn, err := peerbroker.Dial(stream, []webrtc.ICEServer{{
			URLs: []string{"stun:stun.l.google.com:19302"},
		}}, nil)
		require.NoError(t, err)
		defer clientConn.Close()

		serverConn, err := listener.Accept()
		require.NoError(t, err)
		defer serverConn.Close()
		_, err = serverConn.Ping()
		require.NoError(t, err)

		_, err = clientConn.Ping()
		require.NoError(t, err)
	})
}
