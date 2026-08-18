package mcpssrf

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestIsBlockedAddr(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		addr    string
		allowed []netip.Prefix
		blocked bool
	}{
		{name: "LoopbackIPv4", addr: "127.0.0.1", blocked: true},
		{name: "LoopbackIPv4Alias", addr: "127.0.0.2", blocked: true},
		{name: "LoopbackIPv6", addr: "::1", blocked: true},
		{addr: "10.0.0.1", blocked: true},
		{addr: "172.16.5.4", blocked: true},
		{addr: "192.168.1.1", blocked: true},
		{addr: "fd12:3456::1", blocked: true},
		{addr: "169.254.169.254", blocked: true},
		{addr: "fe80::1", blocked: true},
		{addr: "0.0.0.0", blocked: true},
		{addr: "::", blocked: true},
		{addr: "224.0.0.1", blocked: true},
		{addr: "ff02::1", blocked: true},
		{addr: "100.64.0.1", blocked: true},
		{addr: "192.0.0.8", blocked: true},
		{addr: "192.0.2.1", blocked: true},
		{addr: "192.31.196.1", blocked: true},
		{addr: "192.52.193.1", blocked: true},
		{addr: "192.88.99.1", blocked: true},
		{addr: "192.175.48.1", blocked: true},
		{addr: "198.18.0.1", blocked: true},
		{addr: "198.51.100.1", blocked: true},
		{addr: "203.0.113.1", blocked: true},
		{addr: "240.0.0.1", blocked: true},
		{addr: "0.1.2.3", blocked: true},
		{addr: "64:ff9b::1", blocked: true},
		{addr: "100::1", blocked: true},
		{addr: "100:0:0:1::1", blocked: true},
		{addr: "2001::1", blocked: true},
		{addr: "2001:db8::1", blocked: true},
		{addr: "2002::1", blocked: true},
		{addr: "2620:4f:8000::1", blocked: true},
		{addr: "3fff::1", blocked: true},
		{addr: "5f00::1", blocked: true},
		{addr: "64:ff9b:1::1", blocked: true},
		{addr: "::ffff:169.254.169.254", blocked: true},
		{addr: "::ffff:127.0.0.1", blocked: true},
		{addr: "8.8.8.8", blocked: false},
		{addr: "1.1.1.1", blocked: false},
		{addr: "2606:4700:4700::1111", blocked: false},
		{
			name:    "AllowlistedLoopback",
			addr:    "127.0.0.1",
			allowed: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
			blocked: false,
		},
		{
			name:    "AllowlistedSpecialPurposeRange",
			addr:    "192.0.2.1",
			allowed: []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")},
			blocked: false,
		},
		{
			name:    "AllowlistDoesNotCoverPeer",
			addr:    "127.0.0.2",
			allowed: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
			blocked: true,
		},
		{
			name:    "AllowlistDoesNotCoverMetadata",
			addr:    "169.254.169.254",
			allowed: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
			blocked: true,
		},
	}
	for _, tc := range cases {
		name := tc.name
		if name == "" {
			name = tc.addr
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.blocked, isBlockedAddr(netip.MustParseAddr(tc.addr), tc.allowed))
		})
	}
}

func TestParseAllowedPrefix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		raw        string
		wantPrefix string
		addr       string
		wantErr    string
	}{
		{
			name:       "MappedIPv4Address",
			raw:        "::ffff:127.0.0.1/128",
			wantPrefix: "127.0.0.1/32",
			addr:       "127.0.0.1",
		},
		{
			name:       "MappedIPv4Network",
			raw:        "::ffff:10.0.0.0/104",
			wantPrefix: "10.0.0.0/8",
			addr:       "10.23.45.67",
		},
		{
			name:    "MappedPrefixTooShort",
			raw:     "::ffff:0.0.0.0/95",
			wantErr: "IPv4-mapped IPv6 prefix length must be at least 96 bits",
		},
		{
			name:       "IPv4Unchanged",
			raw:        "127.0.0.0/8",
			wantPrefix: "127.0.0.0/8",
			addr:       "127.0.0.1",
		},
		{
			name:       "IPv6Unchanged",
			raw:        "::1/128",
			wantPrefix: "::1/128",
			addr:       "::1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			prefix, err := ParseAllowedPrefix(tc.raw)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, netip.MustParsePrefix(tc.wantPrefix), prefix)
			require.False(t, isBlockedAddr(netip.MustParseAddr(tc.addr), []netip.Prefix{prefix}))
		})
	}
}

func startCanaryServer(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.2:0")
	if err != nil {
		t.Skipf("cannot bind 127.0.0.2: %v", err)
	}
	var hits atomic.Int64
	canary := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	_ = canary.Listener.Close()
	canary.Listener = ln
	canary.Start()
	t.Cleanup(canary.Close)
	return canary, &hits
}

var allowOnly127001 = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}

func TestCheckSameOriginRedirect(t *testing.T) {
	t.Parallel()

	req := func(method, rawURL string) *http.Request {
		u, err := url.Parse(rawURL)
		require.NoError(t, err)
		return &http.Request{Method: method, URL: u}
	}

	origin := "https://provider.example/revoke"
	cases := []struct {
		name    string
		req     *http.Request
		origin  string
		wantErr string
	}{
		{name: "SameOrigin", req: req(http.MethodPost, "https://provider.example/revoke2")},
		{name: "ExplicitDefaultPort", req: req(http.MethodPost, "https://provider.example:443/revoke2")},
		{name: "DifferentPort", req: req(http.MethodPost, "https://provider.example:8443/collect"), wantErr: "must stay on origin"},
		{name: "DifferentHost", req: req(http.MethodPost, "https://attacker.example/collect"), wantErr: "must stay on origin"},
		{name: "MethodChanged", req: req(http.MethodGet, "https://provider.example/other"), wantErr: "changed method"},
		{name: "ProtocolDowngrade", req: req(http.MethodPost, "http://provider.example/revoke"), wantErr: "must stay on origin"},
		{
			name:    "DifferentLoopbackOrigin",
			req:     req(http.MethodPost, "http://127.0.0.1:9999/revoke"),
			origin:  "http://localhost:1234/revoke",
			wantErr: "must stay on origin",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := tc.origin
			if o == "" {
				o = origin
			}
			err := CheckSameOriginRedirect(tc.req, []*http.Request{req(http.MethodPost, o)})
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestNewHTTPClientTimeout(t *testing.T) {
	t.Parallel()

	require.Zero(t, NewHTTPClient(&http.Client{}, nil).Timeout)
	require.Equal(t, 30*time.Second, NewHTTPClient(nil, nil).Timeout)
	require.Equal(t, 5*time.Second, NewHTTPClient(&http.Client{Timeout: 5 * time.Second}, nil).Timeout)
}

func TestHTTPClientSSRF(t *testing.T) {
	t.Parallel()

	t.Run("BlocksLoopback", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		var hits atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(server.Close)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		require.NoError(t, err)
		resp, err := NewHTTPClient(nil, nil).Do(req)
		if resp != nil {
			defer resp.Body.Close()
		}
		require.Error(t, err)
		require.Contains(t, err.Error(), "not permitted for MCP traffic")
		require.Zero(t, hits.Load())
	})

	t.Run("BlocksHostnameResolvingToLoopback", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		var hits atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(server.Close)
		_, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:"+port, nil)
		require.NoError(t, err)
		resp, err := NewHTTPClient(nil, nil).Do(req)
		if resp != nil {
			defer resp.Body.Close()
		}
		require.Error(t, err)
		require.Zero(t, hits.Load())
	})

	t.Run("BlocksRedirectToLoopback", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		canary, hits := startCanaryServer(t)
		attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, canary.URL, http.StatusFound)
		}))
		t.Cleanup(attacker.Close)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, attacker.URL, nil)
		require.NoError(t, err)
		resp, err := NewHTTPClient(nil, allowOnly127001).Do(req)
		if resp != nil {
			defer resp.Body.Close()
		}
		require.Error(t, err)
		require.Zero(t, hits.Load())
	})

	t.Run("BlocksPostRedirectToLoopback", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		canary, hits := startCanaryServer(t)
		attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, canary.URL, http.StatusTemporaryRedirect)
		}))
		t.Cleanup(attacker.Close)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, attacker.URL, strings.NewReader("secret"))
		require.NoError(t, err)
		resp, err := NewHTTPClient(nil, allowOnly127001).Do(req)
		if resp != nil {
			defer resp.Body.Close()
		}
		require.Error(t, err)
		require.Zero(t, hits.Load())
	})

	t.Run("BlocksRedirectToMetadataIP", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
		}))
		t.Cleanup(attacker.Close)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, attacker.URL, nil)
		require.NoError(t, err)
		resp, err := NewHTTPClient(nil, allowOnly127001).Do(req)
		if resp != nil {
			defer resp.Body.Close()
		}
		require.Error(t, err)
	})

	t.Run("AllowsAllowlistedLoopback", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(server.Close)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		require.NoError(t, err)
		resp, err := NewHTTPClient(nil, allowOnly127001).Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusNoContent, resp.StatusCode)
	})
}
