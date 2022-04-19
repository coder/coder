package turnconn_test

import (
	"net"
	"sync"
	"testing"

	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd/turnconn"
	"github.com/coder/coder/peer"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestTURNConn(t *testing.T) {
	t.Parallel()
	turnServer, err := turnconn.New(nil)
	require.NoError(t, err)
	defer turnServer.Close()

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	clientDialer, clientTURN := net.Pipe()
	turnServer.Accept(clientTURN, &net.TCPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 16000,
	}, nil)
	require.NoError(t, err)
	clientSettings := webrtc.SettingEngine{}
	clientSettings.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeTCP4, webrtc.NetworkTypeTCP6})
	clientSettings.SetRelayAcceptanceMinWait(0)
	clientSettings.SetICEProxyDialer(turnconn.ProxyDialer(func() (net.Conn, error) {
		return clientDialer, nil
	}))
	client, err := peer.Client([]webrtc.ICEServer{turnconn.Proxy}, &peer.ConnOptions{
		SettingEngine: clientSettings,
		Logger:        logger.Named("client"),
	})
	require.NoError(t, err)
	defer func() {
		_ = client.Close()
	}()

	serverDialer, serverTURN := net.Pipe()
	turnServer.Accept(serverTURN, &net.TCPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 16001,
	}, nil)
	require.NoError(t, err)
	serverSettings := webrtc.SettingEngine{}
	serverSettings.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeTCP4, webrtc.NetworkTypeTCP6})
	serverSettings.SetRelayAcceptanceMinWait(0)
	serverSettings.SetICEProxyDialer(turnconn.ProxyDialer(func() (net.Conn, error) {
		return serverDialer, nil
	}))
	server, err := peer.Server([]webrtc.ICEServer{turnconn.Proxy}, &peer.ConnOptions{
		SettingEngine: serverSettings,
		Logger:        logger.Named("server"),
	})
	require.NoError(t, err)
	defer func() {
		_ = server.Close()
	}()
	exchange(t, client, server)

	_, err = client.Ping()
	require.NoError(t, err)
}

func exchange(t *testing.T, client, server *peer.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	t.Cleanup(wg.Wait)
	go func() {
		defer wg.Done()
		for {
			select {
			case c := <-server.LocalCandidate():
				client.AddRemoteCandidate(c)
			case c := <-server.LocalSessionDescription():
				client.SetRemoteSessionDescription(c)
			case <-server.Closed():
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for {
			select {
			case c := <-client.LocalCandidate():
				server.AddRemoteCandidate(c)
			case c := <-client.LocalSessionDescription():
				server.SetRemoteSessionDescription(c)
			case <-client.Closed():
				return
			}
		}
	}()
}
