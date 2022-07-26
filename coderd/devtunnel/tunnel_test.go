package devtunnel_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"cdr.dev/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd/devtunnel"
)

const (
	ipByte1 = 0xfc
	ipByte2 = 0xca
	wgPort  = 48732
)

var (
	serverIP = netip.AddrFrom16([16]byte{ipByte1, ipByte2, 15: 0x1})
	dnsIP    = netip.AddrFrom4([4]byte{1, 1, 1, 1})
	clientIP = netip.AddrFrom16([16]byte{ipByte1, ipByte2, 15: 0x2})
)

// The tunnel leaks a few goroutines that aren't impactful to production scenarios.
// func TestMain(m *testing.M) {
// 	goleak.VerifyTestMain(m)
// }

// TestTunnel cannot run in parallel because we hardcode the UDP port used by the wireguard server.
// nolint: paralleltest
func TestTunnel(t *testing.T) {
	ctx, cancelTun := context.WithCancel(context.Background())
	defer cancelTun()

	server := http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Log("got request for", r.URL)
			// Going to use something _slightly_ exotic so that we can't accidentally get some
			// default behavior creating a false positive on the test
			w.WriteHeader(http.StatusAccepted)
		}),
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	fTunServer := newFakeTunnelServer(t)
	cfg := fTunServer.config()

	tun, errCh, err := devtunnel.NewWithConfig(ctx, slogtest.Make(t, nil).Leveled(slog.LevelDebug), cfg)
	require.NoError(t, err)
	t.Log(tun.URL)

	go func() {
		err := server.Serve(tun.Listener)
		assert.Equal(t, http.ErrServerClosed, err)
	}()
	defer func() { _ = server.Close() }()
	defer func() { tun.Listener.Close() }()

	require.Eventually(t, func() bool {
		res, err := fTunServer.requestHTTP()
		if !assert.NoError(t, err) {
			return false
		}
		defer res.Body.Close()
		_, _ = io.Copy(io.Discard, res.Body)

		return res.StatusCode == http.StatusAccepted
	}, time.Minute, time.Second)

	assert.NoError(t, server.Close())
	cancelTun()

	select {
	case <-errCh:
	case <-time.After(10 * time.Second):
		t.Error("tunnel did not close after 10 seconds")
	}
}

// fakeTunnelServer is a fake version of the real dev tunnel server.  It fakes 2 client interactions
// that we want to test:
//   1. Responding to a POST /tun from the client
//   2. Sending an HTTP request down the wireguard connection
//
// Note that for 2, we don't implement a full proxy that accepts arbitrary requests, we just send
// a test request over the Wireguard tunnel to make sure that we can listen.  The proxy behavior is
// outside of the scope of the dev tunnel client, which is what we are testing here.
type fakeTunnelServer struct {
	t       *testing.T
	pub     device.NoisePublicKey
	priv    device.NoisePrivateKey
	tnet    *netstack.Net
	device  *device.Device
	clients int
	server  *httptest.Server
}

func newFakeTunnelServer(t *testing.T) *fakeTunnelServer {
	t.Helper()

	priv, err := wgtypes.GeneratePrivateKey()
	require.NoError(t, err)
	privBytes := [32]byte(priv)
	pub := priv.PublicKey()
	pubBytes := [32]byte(pub)
	tun, tnet, err := netstack.CreateNetTUN(
		[]netip.Addr{serverIP},
		[]netip.Addr{dnsIP},
		1280,
	)
	require.NoError(t, err)

	ctx := context.Background()
	slogger := slogtest.Make(t, nil).Leveled(slog.LevelDebug).Named("server")
	logger := &device.Logger{
		Verbosef: slog.Stdlib(ctx, slogger, slog.LevelDebug).Printf,
		Errorf:   slog.Stdlib(ctx, slogger, slog.LevelError).Printf,
	}
	dev := device.NewDevice(tun, conn.NewDefaultBind(), logger)
	t.Cleanup(func() {
		dev.RemoveAllPeers()
		dev.Close()
		slogger.Debug(ctx, "dev.Close()")
	})
	err = dev.IpcSet(fmt.Sprintf(`private_key=%s
listen_port=%d`,
		hex.EncodeToString(privBytes[:]),
		wgPort,
	))
	require.NoError(t, err)

	err = dev.Up()
	require.NoError(t, err)

	server := newFakeTunnelHTTPSServer(t, pubBytes)

	return &fakeTunnelServer{
		t:      t,
		pub:    device.NoisePublicKey(pub),
		priv:   device.NoisePrivateKey(priv),
		tnet:   tnet,
		device: dev,
		server: server,
	}
}

func newFakeTunnelHTTPSServer(t *testing.T, pubBytes [32]byte) *httptest.Server {
	handler := http.NewServeMux()
	handler.HandleFunc("/tun", func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "POST", request.Method)

		resp := devtunnel.ServerResponse{
			Hostname:        fmt.Sprintf("[%s]", serverIP.String()),
			ServerIP:        serverIP,
			ServerPublicKey: hex.EncodeToString(pubBytes[:]),
			ClientIP:        clientIP,
		}
		b, err := json.Marshal(&resp)
		assert.NoError(t, err)
		writer.WriteHeader(200)
		_, err = writer.Write(b)
		assert.NoError(t, err)
	})

	server := httptest.NewTLSServer(handler)
	t.Cleanup(func() {
		server.Close()
	})
	return server
}

func (f *fakeTunnelServer) config() devtunnel.Config {
	priv, err := wgtypes.GeneratePrivateKey()
	require.NoError(f.t, err)
	pub := priv.PublicKey()
	f.clients++
	assert.Equal(f.t, 1, f.clients) // only allow one client as we hardcode the address

	err = f.device.IpcSet(fmt.Sprintf(`public_key=%x
allowed_ip=%s/128`,
		pub[:],
		clientIP.String(),
	))
	require.NoError(f.t, err)
	return devtunnel.Config{
		Version:    1,
		PrivateKey: device.NoisePrivateKey(priv),
		PublicKey:  device.NoisePublicKey(pub),
		Tunnel: devtunnel.Node{
			HostnameHTTPS:     strings.TrimPrefix(f.server.URL, "https://"),
			HostnameWireguard: "localhost",
			WireguardPort:     wgPort,
		},
		HTTPClient: f.server.Client(),
	}
}

func (f *fakeTunnelServer) requestHTTP() (*http.Response, error) {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			f.t.Log("Dial", network, addr)
			nc, err := f.tnet.DialContextTCPAddrPort(ctx, netip.AddrPortFrom(clientIP, 8090))
			assert.NoError(f.t, err)
			return nc, err
		},
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}
	return client.Get(fmt.Sprintf("http://[%s]:8090", clientIP))
}
