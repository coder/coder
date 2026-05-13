package azureidentity

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsPrivateIP(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		ip      string
		blocked bool
	}{
		{"loopback v4", "127.0.0.1", true},
		{"loopback v6", "::1", true},
		{"link local v4 (azure metadata)", "169.254.169.254", true},
		{"link local v6", "fe80::1", true},
		{"rfc1918 10/8", "10.0.0.1", true},
		{"rfc1918 172.16/12", "172.16.0.1", true},
		{"rfc1918 192.168/16", "192.168.0.1", true},
		{"ipv6 ula", "fc00::1", true},
		{"unspecified v4", "0.0.0.0", true},
		{"unspecified v6", "::", true},
		{"this-network 0.0.0.0/8", "0.1.2.3", true},
		{"cgnat 100.64/10", "100.64.0.1", true},
		{"benchmarking 198.18/15", "198.18.0.1", true},
		{"multicast v4", "224.0.0.1", true},
		{"ipv6 nat64 well-known", "64:ff9b:1::1", true},
		{"ipv6 discard-only", "100::1", true},
		{"ipv6 benchmarking", "2001:2::1", true},
		{"ipv6 documentation", "2001:db8::1", true},
		// IPv4-mapped IPv6: must canonicalize to v4 before
		// classification, otherwise an attacker could bypass
		// the metadata block via ::ffff:169.254.169.254.
		{"ipv4-mapped metadata", "::ffff:169.254.169.254", true},
		{"ipv4-mapped rfc1918", "::ffff:10.0.0.1", true},

		{"public v4", "8.8.8.8", false},
		{"public v6", "2606:4700:4700::1111", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ip := net.ParseIP(tc.ip)
			require.NotNil(t, ip, "parse %q", tc.ip)
			require.Equal(t, tc.blocked, isPrivateIP(ip))
		})
	}
}

// TestCertFetchClientRejectsLoopback proves the dialer refuses
// to connect even when the URL itself would have passed an
// allowlist (httptest.Server always binds to 127.0.0.1, so a
// successful fetch here would mean the SSRF guard had failed).
func TestCertFetchClientRejectsLoopback(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("should never be reached"))
	}))
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	resp, err := certFetchClient.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	require.Error(t, err)
	require.Contains(t, err.Error(), "private IP")
}
