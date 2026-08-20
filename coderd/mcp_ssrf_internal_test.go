package coderd

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestIsBlockedMCPDiscoveryAddr(t *testing.T) {
	t.Parallel()

	cases := []struct {
		addr    string
		allowed []netip.Prefix
		blocked bool
	}{
		// Loopback.
		{addr: "127.0.0.1", blocked: true},
		{addr: "127.0.0.2", blocked: true},
		{addr: "::1", blocked: true},
		// Private (RFC 1918 / ULA).
		{addr: "10.0.0.1", blocked: true},
		{addr: "172.16.5.4", blocked: true},
		{addr: "192.168.1.1", blocked: true},
		{addr: "fd12:3456::1", blocked: true},
		// Link-local, incl. cloud metadata.
		{addr: "169.254.169.254", blocked: true},
		{addr: "fe80::1", blocked: true},
		// Unspecified and multicast.
		{addr: "0.0.0.0", blocked: true},
		{addr: "::", blocked: true},
		{addr: "224.0.0.1", blocked: true},
		{addr: "ff02::1", blocked: true},
		// Special-use ranges not covered by stdlib checks.
		{addr: "100.64.0.1", blocked: true},   // Carrier-grade NAT.
		{addr: "198.18.0.1", blocked: true},   // Benchmarking.
		{addr: "0.1.2.3", blocked: true},      // "This network".
		{addr: "100::1", blocked: true},       // Discard-only.
		{addr: "2001:db8::1", blocked: true},  // Documentation.
		{addr: "64:ff9b:1::1", blocked: true}, // NAT64 translation.
		// IPv4-mapped IPv6 must not bypass IPv4 ranges.
		{addr: "::ffff:169.254.169.254", blocked: true},
		{addr: "::ffff:127.0.0.1", blocked: true},
		// Public addresses are not blocked.
		{addr: "8.8.8.8", blocked: false},
		{addr: "1.1.1.1", blocked: false},
		{addr: "2606:4700:4700::1111", blocked: false},
		// Allowlisted prefixes exempt their range only.
		{
			addr:    "127.0.0.1",
			allowed: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
			blocked: false,
		},
		{
			addr:    "127.0.0.2",
			allowed: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
			blocked: true,
		},
		{
			addr:    "169.254.169.254",
			allowed: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
			blocked: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.addr, func(t *testing.T) {
			t.Parallel()
			got := isBlockedMCPDiscoveryAddr(netip.MustParseAddr(tc.addr), tc.allowed)
			require.Equal(t, tc.blocked, got)
		})
	}
}

// startCanaryServer binds an HTTP server to 127.0.0.2, standing in
// for an internal-only service that MCP OAuth2 discovery must never
// reach. Returns the server and a hit counter.
func startCanaryServer(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.2:0")
	if err != nil {
		t.Skipf("cannot bind 127.0.0.2 (loopback aliasing unsupported?): %v", err)
	}
	var hits atomic.Int64
	canary := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"internal":"secret"}`))
	}))
	_ = canary.Listener.Close()
	canary.Listener = ln
	canary.Start()
	t.Cleanup(canary.Close)
	return canary, &hits
}

// allowOnly127001 allows exactly 127.0.0.1 so tests can reach their
// attacker-controlled httptest server while all other loopback and
// internal addresses stay blocked.
var allowOnly127001 = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}

func TestMCPDiscoveryHTTPClientSSRF(t *testing.T) {
	t.Parallel()

	// Regression for CDM-02-002: discovery must not reach an MCP
	// server URL that points at a private/internal address.
	t.Run("BlocksLoopbackDiscovery", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		var hits atomic.Int64
		mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(mcpServer.Close)

		client := newMCPSSRFHTTPClient(nil, nil)
		_, err := discoverAndRegisterMCPOAuth2(ctx, client, mcpServer.URL+"/v1/mcp", "https://coder.example.com/callback")
		require.Error(t, err)
		require.Contains(t, err.Error(), "not permitted for MCP requests")
		require.EqualValues(t, 0, hits.Load(), "loopback MCP server must never be contacted")
	})

	// Regression for CDM-02-002: a hostname that resolves to an
	// internal address must be blocked at dial time (DNS rebinding
	// cannot bypass URL-level checks).
	t.Run("BlocksHostnameResolvingToLoopback", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		var hits atomic.Int64
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)
		_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
		require.NoError(t, err)

		client := newMCPSSRFHTTPClient(nil, nil)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:"+port+"/", nil)
		require.NoError(t, err)
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
		require.Error(t, err)
		require.Contains(t, err.Error(), "not permitted for MCP requests")
		require.EqualValues(t, 0, hits.Load(), "server behind loopback-resolving hostname must never be contacted")
	})

	// Regression for CDM-02-002: an allowed MCP server that redirects
	// discovery fetches to an internal loopback address must not
	// cause that address to be contacted. Before the fix, the canary
	// received the redirected requests.
	t.Run("BlocksDiscoveryRedirectToLoopbackCanary", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		canary, canaryHits := startCanaryServer(t)
		attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, canary.URL+r.URL.Path, http.StatusFound)
		}))
		t.Cleanup(attacker.Close)

		client := newMCPSSRFHTTPClient(nil, allowOnly127001)
		_, err := discoverAndRegisterMCPOAuth2(ctx, client, attacker.URL+"/v1/mcp", "https://coder.example.com/callback")
		require.Error(t, err)
		require.Contains(t, err.Error(), "blocked")
		require.EqualValues(t, 0, canaryHits.Load(), "internal canary must never be contacted via redirect")
	})

	// Regression for CDM-02-002: same as above for the Dynamic Client
	// Registration POST (RFC 7591), which the finding called out
	// explicitly.
	t.Run("BlocksRegistrationRedirectToLoopbackCanary", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		canary, canaryHits := startCanaryServer(t)
		attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := "http://" + r.Host
			switch r.URL.Path {
			case "/.well-known/oauth-protected-resource":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"resource": "` + origin + `", "authorization_servers": ["` + origin + `"]}`))
			case "/.well-known/oauth-authorization-server":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"issuer": "` + origin + `",
					"authorization_endpoint": "` + origin + `/authorize",
					"token_endpoint": "` + origin + `/token",
					"registration_endpoint": "` + origin + `/register"
				}`))
			case "/register":
				// Redirect the DCR POST to the internal canary.
				http.Redirect(w, r, canary.URL+"/register", http.StatusTemporaryRedirect)
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(attacker.Close)

		client := newMCPSSRFHTTPClient(nil, allowOnly127001)
		_, err := discoverAndRegisterMCPOAuth2(ctx, client, attacker.URL, "https://coder.example.com/callback")
		require.Error(t, err)
		require.Contains(t, err.Error(), "blocked")
		require.EqualValues(t, 0, canaryHits.Load(), "internal canary must never receive the registration POST")
	})

	// Regression for CDM-02-002: redirects to the cloud metadata IP
	// are rejected before any connection is attempted.
	t.Run("BlocksRedirectToMetadataIP", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
		}))
		t.Cleanup(attacker.Close)

		client := newMCPSSRFHTTPClient(nil, allowOnly127001)
		_, err := discoverAndRegisterMCPOAuth2(ctx, client, attacker.URL+"/v1/mcp", "https://coder.example.com/callback")
		require.Error(t, err)
		require.Contains(t, err.Error(), "blocked")
	})

	// Allowlisted ranges (used by tests and coderdtest) still permit
	// the full discovery + registration flow.
	t.Run("AllowlistedDiscoverySucceeds", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := "http://" + r.Host
			switch r.URL.Path {
			case "/.well-known/oauth-protected-resource":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"resource": "` + origin + `", "authorization_servers": ["` + origin + `"]}`))
			case "/.well-known/oauth-authorization-server":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"issuer": "` + origin + `",
					"authorization_endpoint": "` + origin + `/authorize",
					"token_endpoint": "` + origin + `/token",
					"registration_endpoint": "` + origin + `/register"
				}`))
			case "/register":
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"client_id": "test-client", "client_secret": "test-secret"}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(server.Close)

		client := newMCPSSRFHTTPClient(nil, allowOnly127001)
		result, err := discoverAndRegisterMCPOAuth2(ctx, client, server.URL, "https://coder.example.com/callback")
		require.NoError(t, err)
		require.Equal(t, "test-client", result.clientID)
		require.Equal(t, server.URL+"/authorize", result.authURL)
		require.Equal(t, server.URL+"/token", result.tokenURL)
	})
}
