package peerbroker_test

import (
	"context"
	"sync"
	"testing"

	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/peer"
	"github.com/coder/coder/peerbroker"
	"github.com/coder/coder/peerbroker/proto"
	"github.com/coder/coder/provisionersdk"
)

func TestProxy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	channelID := "hello"
	pubsub := database.NewPubsubInMemory()
	dialerClient, dialerServer := provisionersdk.TransportPipe()
	defer dialerClient.Close()
	defer dialerServer.Close()
	listenerClient, listenerServer := provisionersdk.TransportPipe()
	defer listenerClient.Close()
	defer listenerServer.Close()

	listener, err := peerbroker.Listen(listenerServer, func(ctx context.Context) ([]webrtc.ICEServer, *peer.ConnOptions, error) {
		return nil, &peer.ConnOptions{
			Logger: slogtest.Make(t, nil).Named("server").Leveled(slog.LevelDebug),
		}, nil
	})
	require.NoError(t, err)

	proxyCloser, err := peerbroker.ProxyDial(proto.NewDRPCPeerBrokerClient(provisionersdk.Conn(listenerClient)), peerbroker.ProxyOptions{
		ChannelID: channelID,
		Logger:    slogtest.Make(t, nil).Named("proxy-listen").Leveled(slog.LevelDebug),
		Pubsub:    pubsub,
	})
	require.NoError(t, err)
	defer func() {
		_ = proxyCloser.Close()
	}()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err = peerbroker.ProxyListen(ctx, dialerServer, peerbroker.ProxyOptions{
			ChannelID: channelID,
			Logger:    slogtest.Make(t, nil).Named("proxy-dial").Leveled(slog.LevelDebug),
			Pubsub:    pubsub,
		})
		assert.NoError(t, err)
	}()

	api := proto.NewDRPCPeerBrokerClient(provisionersdk.Conn(dialerClient))
	stream, err := api.NegotiateConnection(ctx)
	require.NoError(t, err)
	clientConn, err := peerbroker.Dial(stream, []webrtc.ICEServer{{
		URLs: []string{"stun:stun.l.google.com:19302"},
	}}, &peer.ConnOptions{
		Logger: slogtest.Make(t, nil).Named("client").Leveled(slog.LevelDebug),
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

	_ = dialerServer.Close()
	wg.Wait()
}
