package peerbroker_test

import (
	"context"
	"testing"

	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/peer"
	"github.com/coder/coder/peerbroker"
	"github.com/coder/coder/peerbroker/proto"
	"github.com/coder/coder/provisionersdk"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestDial(t *testing.T) {
	t.Parallel()

	t.Run("Connect", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		client, server := provisionersdk.TransportPipe()
		defer client.Close()
		defer server.Close()

		settingEngine := webrtc.SettingEngine{}
		listener, err := peerbroker.Listen(server, func(ctx context.Context) ([]webrtc.ICEServer, *peer.ConnOptions, error) {
			return []webrtc.ICEServer{{
					URLs: []string{"stun:stun.l.google.com:19302"},
				}}, &peer.ConnOptions{
					Logger:        slogtest.Make(t, nil).Named("server").Leveled(slog.LevelDebug),
					SettingEngine: settingEngine,
				}, nil
		})
		require.NoError(t, err)

		api := proto.NewDRPCPeerBrokerClient(provisionersdk.Conn(client))
		stream, err := api.NegotiateConnection(ctx)
		require.NoError(t, err)

		clientConn, err := peerbroker.Dial(stream, []webrtc.ICEServer{{
			URLs: []string{"stun:stun.l.google.com:19302"},
		}}, &peer.ConnOptions{
			Logger:        slogtest.Make(t, nil).Named("client").Leveled(slog.LevelDebug),
			SettingEngine: settingEngine,
		})
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
