package peer_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/transport/vnet"
	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/peer"
)

const (
	disconnectedTimeout = time.Second
	failedTimeout       = disconnectedTimeout * 5
	keepAliveInterval   = time.Millisecond * 2
)

var (
	// There's a global race in the vnet library allocation code.
	// This mutex locks around the creation of the vnet.
	vnetMutex = sync.Mutex{}
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestConn(t *testing.T) {
	t.Parallel()

	t.Run("Ping", func(t *testing.T) {
		t.Parallel()
		client, server, _ := createPair(t)
		_, err := client.Ping()
		require.NoError(t, err)
		_, err = server.Ping()
		require.NoError(t, err)
	})

	t.Run("PingNetworkOffline", func(t *testing.T) {
		t.Parallel()
		_, server, wan := createPair(t)
		_, err := server.Ping()
		require.NoError(t, err)
		err = wan.Stop()
		require.NoError(t, err)
		_, err = server.Ping()
		require.ErrorIs(t, err, peer.ErrFailed)
	})

	t.Run("PingReconnect", func(t *testing.T) {
		t.Parallel()
		_, server, wan := createPair(t)
		_, err := server.Ping()
		require.NoError(t, err)
		// Create a channel that closes on disconnect.
		channel, err := server.Dial(context.Background(), "wow", nil)
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
		cch, err := client.Dial(context.Background(), "hello", &peer.ChannelOpts{})
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
		cch, err := client.Dial(context.Background(), "hello", &peer.ChannelOpts{})
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
		cch, err := client.Dial(context.Background(), "hello", &peer.ChannelOpts{})
		require.NoError(t, err)
		sch, err := server.Accept(context.Background())
		require.NoError(t, err)
		defer sch.Close()
		go func() {
			for i := 0; i < 1024; i++ {
				_, err := cch.Write(make([]byte, 4096))
				require.NoError(t, err)
			}
			_ = cch.Close()
		}()
		for {
			_, err = sch.Read(make([]byte, 4096))
			if err != nil {
				require.ErrorIs(t, err, peer.ErrClosed)
				break
			}
		}
	})

	t.Run("NetConn", func(t *testing.T) {
		t.Parallel()
		client, server, _ := createPair(t)
		srv, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer srv.Close()
		go func() {
			sch, err := server.Accept(context.Background())
			require.NoError(t, err)
			nc2 := sch.NetConn()
			nc1, err := net.Dial("tcp", srv.Addr().String())
			require.NoError(t, err)
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

		defaultTransport := http.DefaultTransport.(*http.Transport).Clone()
		var cch *peer.Channel
		defaultTransport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			cch, err = client.Dial(ctx, "hello", &peer.ChannelOpts{})
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
		err := client.Close()
		require.NoError(t, err)
		err = server.Close()
		require.NoError(t, err)
	})

	t.Run("CloseWithError", func(t *testing.T) {
		conn, err := peer.Client([]webrtc.ICEServer{}, nil)
		require.NoError(t, err)
		expectedErr := errors.New("wow")
		_ = conn.CloseWithError(expectedErr)
		_, err = conn.Dial(context.Background(), "", nil)
		require.ErrorIs(t, err, expectedErr)
	})

	t.Run("PingConcurrent", func(t *testing.T) {
		t.Parallel()
		client, server, _ := createPair(t)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, err := client.Ping()
			require.NoError(t, err)
		}()
		go func() {
			defer wg.Done()
			_, err := server.Ping()
			require.NoError(t, err)
		}()
		wg.Wait()
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
	channel1, err := peer.Client([]webrtc.ICEServer{}, &peer.ConnOpts{
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
	channel2, err := peer.Server([]webrtc.ICEServer{}, &peer.ConnOpts{
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

	go func() {
		for {
			select {
			case c := <-channel2.LocalCandidate():
				_ = channel1.AddRemoteCandidate(c)
			case c := <-channel2.LocalSessionDescription():
				channel1.SetRemoteSessionDescription(c)
			case <-channel2.Closed():
				return
			}
		}
	}()

	go func() {
		for {
			select {
			case c := <-channel1.LocalCandidate():
				_ = channel2.AddRemoteCandidate(c)
			case c := <-channel1.LocalSessionDescription():
				channel2.SetRemoteSessionDescription(c)
			case <-channel1.Closed():
				return
			}
		}
	}()

	return channel1, channel2, wan
}
