package peer_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/transport/vnet"
	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/peer"
)

var (
	disconnectedTimeout = func() time.Duration {
		// Connection state is unfortunately time-based. When resources are
		// contended, a connection can take greater than this timeout to
		// handshake, which results in a test flake.
		//
		// During local testing resources are rarely contended. Reducing this
		// timeout leads to faster local development.
		//
		// In CI resources are frequently contended, so increasing this value
		// results in less flakes.
		if os.Getenv("CI") == "true" {
			return time.Second
		}
		return 100 * time.Millisecond
	}()
	failedTimeout     = disconnectedTimeout * 3
	keepAliveInterval = time.Millisecond * 2

	// There's a global race in the vnet library allocation code.
	// This mutex locks around the creation of the vnet.
	vnetMutex = sync.Mutex{}
)

func TestMain(m *testing.M) {
	// pion/ice doesn't properly close immediately. The solution for this isn't yet known. See:
	// https://github.com/pion/ice/pull/413
	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("github.com/pion/ice/v2.(*Agent).startOnConnectionStateChangeRoutine.func1"),
		goleak.IgnoreTopFunction("github.com/pion/ice/v2.(*Agent).startOnConnectionStateChangeRoutine.func2"),
		goleak.IgnoreTopFunction("github.com/pion/ice/v2.(*Agent).taskLoop"),
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
	)
}

func TestConn(t *testing.T) {
	t.Skip("known flake -- https://github.com/coder/coder/issues/1644")
	t.Parallel()

	t.Run("Ping", func(t *testing.T) {
		t.Parallel()
		client, server, _ := createPair(t)
		exchange(t, client, server)
		_, err := client.Ping()
		require.NoError(t, err)
		_, err = server.Ping()
		require.NoError(t, err)
	})

	t.Run("PingNetworkOffline", func(t *testing.T) {
		t.Parallel()
		client, server, wan := createPair(t)
		exchange(t, client, server)
		_, err := server.Ping()
		require.NoError(t, err)
		err = wan.Stop()
		require.NoError(t, err)
		_, err = server.Ping()
		require.ErrorIs(t, err, peer.ErrFailed)
	})

	t.Run("PingReconnect", func(t *testing.T) {
		t.Parallel()
		client, server, wan := createPair(t)
		exchange(t, client, server)
		_, err := server.Ping()
		require.NoError(t, err)
		// Create a channel that closes on disconnect.
		channel, err := server.CreateChannel(context.Background(), "wow", nil)
		assert.NoError(t, err)
		err = wan.Stop()
		require.NoError(t, err)
		// Once the connection is marked as disconnected, this
		// channel will be closed.
		_, err = channel.Read(make([]byte, 4))
		assert.ErrorIs(t, err, peer.ErrClosed)
		err = wan.Start()
		require.NoError(t, err)
		_, err = server.Ping()
		require.NoError(t, err)
	})

	t.Run("Accept", func(t *testing.T) {
		t.Parallel()
		client, server, _ := createPair(t)
		exchange(t, client, server)
		cch, err := client.CreateChannel(context.Background(), "hello", &peer.ChannelOptions{})
		require.NoError(t, err)

		sch, err := server.Accept(context.Background())
		require.NoError(t, err)
		defer sch.Close()

		_ = cch.Close()
		_, err = sch.Read(make([]byte, 4))
		require.ErrorIs(t, err, peer.ErrClosed)
	})

	t.Run("AcceptNetworkOffline", func(t *testing.T) {
		t.Parallel()
		client, server, wan := createPair(t)
		exchange(t, client, server)
		cch, err := client.CreateChannel(context.Background(), "hello", &peer.ChannelOptions{})
		require.NoError(t, err)
		sch, err := server.Accept(context.Background())
		require.NoError(t, err)
		defer sch.Close()

		err = wan.Stop()
		require.NoError(t, err)
		_ = cch.Close()
		_, err = sch.Read(make([]byte, 4))
		require.ErrorIs(t, err, peer.ErrClosed)
	})

	t.Run("Buffering", func(t *testing.T) {
		t.Parallel()
		client, server, _ := createPair(t)
		exchange(t, client, server)
		cch, err := client.CreateChannel(context.Background(), "hello", &peer.ChannelOptions{})
		require.NoError(t, err)
		sch, err := server.Accept(context.Background())
		require.NoError(t, err)
		defer sch.Close()
		go func() {
			bytes := make([]byte, 4096)
			for i := 0; i < 1024; i++ {
				_, err := cch.Write(bytes)
				require.NoError(t, err)
			}
			_ = cch.Close()
		}()
		bytes := make([]byte, 4096)
		for {
			_, err = sch.Read(bytes)
			if err != nil {
				require.ErrorIs(t, err, peer.ErrClosed)
				break
			}
		}
	})

	t.Run("NetConn", func(t *testing.T) {
		t.Parallel()
		client, server, _ := createPair(t)
		exchange(t, client, server)
		srv, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer srv.Close()
		go func() {
			sch, err := server.Accept(context.Background())
			assert.NoError(t, err)
			nc2 := sch.NetConn()
			nc1, err := net.Dial("tcp", srv.Addr().String())
			assert.NoError(t, err)
			go func() {
				_, _ = io.Copy(nc1, nc2)
			}()
			_, _ = io.Copy(nc2, nc1)
		}()
		go func() {
			server := http.Server{
				Handler: http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
					rw.WriteHeader(200)
				}),
			}
			defer server.Close()
			_ = server.Serve(srv)
		}()

		//nolint:forcetypeassert
		defaultTransport := http.DefaultTransport.(*http.Transport).Clone()
		var cch *peer.Channel
		defaultTransport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			cch, err = client.CreateChannel(ctx, "hello", &peer.ChannelOptions{})
			if err != nil {
				return nil, err
			}
			return cch.NetConn(), nil
		}
		c := http.Client{
			Transport: defaultTransport,
		}
		req, err := http.NewRequestWithContext(context.Background(), "GET", "http://localhost/", nil)
		require.NoError(t, err)
		resp, err := c.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, resp.StatusCode, 200)
		// Triggers any connections to close.
		// This test below ensures the DataChannel actually closes.
		defaultTransport.CloseIdleConnections()
		err = cch.Close()
		require.ErrorIs(t, err, peer.ErrClosed)
	})

	t.Run("CloseBeforeNegotiate", func(t *testing.T) {
		t.Parallel()
		client, server, _ := createPair(t)
		exchange(t, client, server)
		err := client.Close()
		require.NoError(t, err)
		err = server.Close()
		require.NoError(t, err)
	})

	t.Run("CloseWithError", func(t *testing.T) {
		t.Parallel()
		conn, err := peer.Client([]webrtc.ICEServer{}, nil)
		require.NoError(t, err)
		expectedErr := xerrors.New("wow")
		_ = conn.CloseWithError(expectedErr)
		_, err = conn.CreateChannel(context.Background(), "", nil)
		require.ErrorIs(t, err, expectedErr)
	})

	t.Run("PingConcurrent", func(t *testing.T) {
		t.Parallel()
		client, server, _ := createPair(t)
		exchange(t, client, server)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, err := client.Ping()
			assert.NoError(t, err)
		}()
		go func() {
			defer wg.Done()
			_, err := server.Ping()
			assert.NoError(t, err)
		}()
		wg.Wait()
	})

	t.Run("CandidateBeforeSessionDescription", func(t *testing.T) {
		t.Parallel()
		client, server, _ := createPair(t)
		server.SetRemoteSessionDescription(<-client.LocalSessionDescription())
		sdp := <-server.LocalSessionDescription()
		client.AddRemoteCandidate(<-server.LocalCandidate())
		client.SetRemoteSessionDescription(sdp)
		server.AddRemoteCandidate(<-client.LocalCandidate())
		_, err := client.Ping()
		require.NoError(t, err)
	})

	t.Run("ShortBuffer", func(t *testing.T) {
		t.Parallel()
		client, server, _ := createPair(t)
		exchange(t, client, server)
		go func() {
			channel, err := client.CreateChannel(context.Background(), "test", nil)
			assert.NoError(t, err)
			_, err = channel.Write([]byte{1, 2})
			assert.NoError(t, err)
		}()
		channel, err := server.Accept(context.Background())
		require.NoError(t, err)
		data := make([]byte, 1)
		_, err = channel.Read(data)
		require.NoError(t, err)
		require.Equal(t, uint8(0x1), data[0])
		_, err = channel.Read(data)
		require.NoError(t, err)
		require.Equal(t, uint8(0x2), data[0])
	})
}

func createPair(t *testing.T) (client *peer.Conn, server *peer.Conn, wan *vnet.Router) {
	loggingFactory := logging.NewDefaultLoggerFactory()
	loggingFactory.DefaultLogLevel = logging.LogLevelDisabled
	vnetMutex.Lock()
	defer vnetMutex.Unlock()
	wan, err := vnet.NewRouter(&vnet.RouterConfig{
		CIDR:          "1.2.3.0/24",
		LoggerFactory: loggingFactory,
	})
	require.NoError(t, err)
	c1Net := vnet.NewNet(&vnet.NetConfig{
		StaticIPs: []string{"1.2.3.4"},
	})
	err = wan.AddNet(c1Net)
	require.NoError(t, err)
	c2Net := vnet.NewNet(&vnet.NetConfig{
		StaticIPs: []string{"1.2.3.5"},
	})
	err = wan.AddNet(c2Net)
	require.NoError(t, err)

	c1SettingEngine := webrtc.SettingEngine{}
	c1SettingEngine.SetVNet(c1Net)
	c1SettingEngine.SetPrflxAcceptanceMinWait(0)
	c1SettingEngine.SetICETimeouts(disconnectedTimeout, failedTimeout, keepAliveInterval)
	channel1, err := peer.Client([]webrtc.ICEServer{{}}, &peer.ConnOptions{
		SettingEngine: c1SettingEngine,
		Logger:        slogtest.Make(t, nil).Named("client").Leveled(slog.LevelDebug),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		channel1.Close()
	})
	c2SettingEngine := webrtc.SettingEngine{}
	c2SettingEngine.SetVNet(c2Net)
	c2SettingEngine.SetPrflxAcceptanceMinWait(0)
	c2SettingEngine.SetICETimeouts(disconnectedTimeout, failedTimeout, keepAliveInterval)
	channel2, err := peer.Server([]webrtc.ICEServer{{}}, &peer.ConnOptions{
		SettingEngine: c2SettingEngine,
		Logger:        slogtest.Make(t, nil).Named("server").Leveled(slog.LevelDebug),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		channel2.Close()
	})

	err = wan.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = wan.Stop()
	})

	return channel1, channel2, wan
}

func exchange(t *testing.T, client, server *peer.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()

		wg.Wait()
	})
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
