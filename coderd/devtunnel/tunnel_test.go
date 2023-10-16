package devtunnel_test

import (
	"context"
	"crypto/tls"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"cdr.dev/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/devtunnel"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/wgtunnel/tunneld"
	"github.com/coder/wgtunnel/tunnelsdk"
)

// The tunnel leaks a few goroutines that aren't impactful to production scenarios.
// func TestMain(m *testing.M) {
// 	goleak.VerifyTestMain(m)
// }

func TestTunnel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		version tunnelsdk.TunnelVersion
	}{
		{
			name:    "V1",
			version: tunnelsdk.TunnelVersion1,
		},
		{
			name:    "V2",
			version: tunnelsdk.TunnelVersion2,
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancelTun := context.WithCancel(context.Background())
			defer cancelTun()

			server := http.Server{
				ReadHeaderTimeout: time.Minute,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					t.Log("got request for", r.URL)
					// Going to use something _slightly_ exotic so that we can't
					// accidentally get some default behavior creating a false
					// positive on the test
					w.WriteHeader(http.StatusAccepted)
				}),
				BaseContext: func(_ net.Listener) context.Context {
					return ctx
				},
			}

			tunServer := newTunnelServer(t)
			cfg := tunServer.config(t, c.version)

			tun, err := devtunnel.NewWithConfig(ctx, slogtest.Make(t, nil).Leveled(slog.LevelDebug), cfg)
			require.NoError(t, err)
			require.Len(t, tun.OtherURLs, 1)
			t.Log(tun.URL, tun.OtherURLs[0])

			hostSplit := strings.SplitN(tun.URL.Host, ".", 2)
			require.Len(t, hostSplit, 2)
			require.Equal(t, hostSplit[1], tunServer.api.BaseURL.Host)

			// Verify the hostname using the same logic as the tunnel server.
			ip1, urls := tunServer.api.WireguardPublicKeyToIPAndURLs(cfg.PublicKey, c.version)
			require.Len(t, urls, 2)
			require.Equal(t, urls[0].String(), tun.URL.String())
			require.Equal(t, urls[1].String(), tun.OtherURLs[0].String())

			ip2, err := tunServer.api.HostnameToWireguardIP(hostSplit[0])
			require.NoError(t, err)
			require.Equal(t, ip1, ip2)

			// Manually verify the hostname.
			switch c.version {
			case tunnelsdk.TunnelVersion1:
				// The subdomain should be a 32 character hex string.
				require.Len(t, hostSplit[0], 32)
				_, err := hex.DecodeString(hostSplit[0])
				require.NoError(t, err)
			case tunnelsdk.TunnelVersion2:
				// The subdomain should be a base32 encoded string containing
				// 16 bytes once decoded.
				dec, err := base32.HexEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(hostSplit[0]))
				require.NoError(t, err)
				require.Len(t, dec, 8)
			}

			go func() {
				err := server.Serve(tun.Listener)
				assert.Equal(t, http.ErrServerClosed, err)
			}()
			defer func() { _ = server.Close() }()
			defer func() { tun.Listener.Close() }()

			require.Eventually(t, func() bool {
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, tun.URL.String(), nil)
				if !assert.NoError(t, err) {
					return false
				}
				res, err := tunServer.requestTunnel(tun, req)
				if !assert.NoError(t, err) {
					return false
				}
				defer res.Body.Close()
				_, _ = io.Copy(io.Discard, res.Body)

				return res.StatusCode == http.StatusAccepted
			}, testutil.WaitShort, testutil.IntervalSlow)

			assert.NoError(t, server.Close())
			cancelTun()

			select {
			case <-tun.Wait():
			case <-time.After(testutil.WaitLong):
				t.Errorf("tunnel did not close after %s", testutil.WaitLong)
			}
		})
	}
}

func freeUDPPort(t *testing.T) uint16 {
	t.Helper()

	l, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 0,
	})
	require.NoError(t, err, "listen on random UDP port")

	_, port, err := net.SplitHostPort(l.LocalAddr().String())
	require.NoError(t, err, "split host port")

	portUint, err := strconv.ParseUint(port, 10, 16)
	require.NoError(t, err, "parse port")

	// This is prone to races, but since we have to tell wireguard to create the
	// listener and can't pass in a net.Listener, we have to do this.
	err = l.Close()
	require.NoError(t, err, "close UDP listener")

	return uint16(portUint)
}

type tunnelServer struct {
	api *tunneld.API

	server *httptest.Server
}

func newTunnelServer(t *testing.T) *tunnelServer {
	var handler http.Handler
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handler != nil {
			handler.ServeHTTP(w, r)
		}

		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	baseURLParsed, err := url.Parse(srv.URL)
	require.NoError(t, err)
	require.Equal(t, "https", baseURLParsed.Scheme)
	baseURLParsed.Host = net.JoinHostPort("tunnel.coder.com", baseURLParsed.Port())

	key, err := tunnelsdk.GeneratePrivateKey()
	require.NoError(t, err)

	// Sadly the tunnel server needs to be passed a port number and can't be
	// passed an active listener (because wireguard needs to make the listener),
	// so we may need to try a few times to get a free port.
	var td *tunneld.API
	for i := 0; i < 10; i++ {
		wireguardPort := freeUDPPort(t)
		options := &tunneld.Options{
			BaseURL:                baseURLParsed,
			WireguardEndpoint:      fmt.Sprintf("127.0.0.1:%d", wireguardPort),
			WireguardPort:          wireguardPort,
			WireguardKey:           key,
			WireguardMTU:           tunneld.DefaultWireguardMTU,
			WireguardServerIP:      tunneld.DefaultWireguardServerIP,
			WireguardNetworkPrefix: tunneld.DefaultWireguardNetworkPrefix,
		}

		td, err = tunneld.New(options)
		if err == nil {
			break
		}
		t.Logf("failed to create tunnel server on port %d: %s", wireguardPort, err)
	}
	if td == nil {
		t.Fatal("failed to create tunnel server in 10 attempts")
	}
	handler = td.Router()
	t.Cleanup(func() {
		_ = td.Close()
	})

	return &tunnelServer{
		api:    td,
		server: srv,
	}
}

func (s *tunnelServer) client() *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", s.server.Listener.Addr().String())
		},
		TLSClientConfig: &tls.Config{
			//nolint:gosec
			InsecureSkipVerify: true,
		},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   testutil.WaitLong,
	}
}

func (s *tunnelServer) config(t *testing.T, version tunnelsdk.TunnelVersion) devtunnel.Config {
	priv, err := tunnelsdk.GeneratePrivateKey()
	require.NoError(t, err)

	privNoise, err := priv.NoisePrivateKey()
	require.NoError(t, err)
	pubNoise := priv.NoisePublicKey()

	if version == 0 {
		version = tunnelsdk.TunnelVersionLatest
	}

	return devtunnel.Config{
		Version:    version,
		PrivateKey: privNoise,
		PublicKey:  pubNoise,
		Tunnel: devtunnel.Node{
			RegionID:      0,
			ID:            1,
			HostnameHTTPS: s.api.BaseURL.Host,
		},
		HTTPClient: s.client(),
	}
}

// requestTunnel performs the given request against the tunnel. The Host header
// will be set to the tunnel's hostname.
func (s *tunnelServer) requestTunnel(tunnel *tunnelsdk.Tunnel, req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "https"
	req.URL.Host = tunnel.URL.Host
	req.Host = tunnel.URL.Host
	return s.client().Do(req)
}
