package agentegress

import (
	"context"
	"net"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

// mapResolver answers lookups from a fixed table so tests never hit DNS.
type mapResolver map[string]netip.Addr

func (r mapResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	addr, ok := r[host]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return []netip.Addr{addr}, nil
}

func TestBuildRules(t *testing.T) {
	t.Parallel()

	toStrings := func(rules []rule) []string {
		out := make([]string, 0, len(rules))
		for _, r := range rules {
			out = append(out, r.String())
		}
		return out
	}

	tests := []struct {
		name       string
		port       uint16
		exemptions []hostExemption
		wantNat    []string
		wantFilter []string
	}{
		{
			name: "no exemptions",
			port: 40001,
			wantNat: []string{
				"-A CODER_EGRESS -o lo -j RETURN",
				"-A CODER_EGRESS -p tcp -j REDIRECT --to-ports 40001",
			},
			wantFilter: []string{
				"-A CODER_EGRESS_FILTER -o lo -j ACCEPT",
				"-A CODER_EGRESS_FILTER -p udp --dport 53 -j ACCEPT",
				"-A CODER_EGRESS_FILTER -p udp -j DROP",
				"-A CODER_EGRESS_FILTER -p tcp -j ACCEPT",
			},
		},
		{
			name: "control plane exemptions with and without ports",
			port: 40001,
			exemptions: []hostExemption{
				{addr: netip.MustParseAddr("203.0.113.10"), port: 443},
				{addr: netip.MustParseAddr("198.51.100.7")},
			},
			wantNat: []string{
				"-A CODER_EGRESS -o lo -j RETURN",
				"-A CODER_EGRESS -d 203.0.113.10 -p tcp --dport 443 -j RETURN",
				"-A CODER_EGRESS -d 198.51.100.7 -p tcp -j RETURN",
				"-A CODER_EGRESS -p tcp -j REDIRECT --to-ports 40001",
			},
			wantFilter: []string{
				"-A CODER_EGRESS_FILTER -o lo -j ACCEPT",
				"-A CODER_EGRESS_FILTER -p udp --dport 53 -j ACCEPT",
				"-A CODER_EGRESS_FILTER -d 203.0.113.10 -p udp -j ACCEPT",
				"-A CODER_EGRESS_FILTER -d 198.51.100.7 -p udp -j ACCEPT",
				"-A CODER_EGRESS_FILTER -p udp -j DROP",
				"-A CODER_EGRESS_FILTER -p tcp -j ACCEPT",
			},
		},
		{
			name: "ipv6 exemption",
			port: 8080,
			exemptions: []hostExemption{
				{addr: netip.MustParseAddr("2001:db8::1"), port: 443},
			},
			wantNat: []string{
				"-A CODER_EGRESS -o lo -j RETURN",
				"-A CODER_EGRESS -d 2001:db8::1 -p tcp --dport 443 -j RETURN",
				"-A CODER_EGRESS -p tcp -j REDIRECT --to-ports 8080",
			},
			wantFilter: []string{
				"-A CODER_EGRESS_FILTER -o lo -j ACCEPT",
				"-A CODER_EGRESS_FILTER -p udp --dport 53 -j ACCEPT",
				"-A CODER_EGRESS_FILTER -d 2001:db8::1 -p udp -j ACCEPT",
				"-A CODER_EGRESS_FILTER -p udp -j DROP",
				"-A CODER_EGRESS_FILTER -p tcp -j ACCEPT",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rs := buildRules(tt.port, tt.exemptions)
			require.Equal(t, tt.wantNat, toStrings(rs.nat))
			require.Equal(t, tt.wantFilter, toStrings(rs.filter))
		})
	}
}

func TestSplitByFamily(t *testing.T) {
	t.Parallel()

	v4, v6 := splitByFamily([]hostExemption{
		{addr: netip.MustParseAddr("203.0.113.10"), port: 443},
		{addr: netip.MustParseAddr("2001:db8::1")},
		{addr: netip.MustParseAddr("198.51.100.7")},
	})
	require.Len(t, v4, 2)
	require.Len(t, v6, 1)
	require.Equal(t, "2001:db8::1", v6[0].addr.String())
}

func TestResolveExemptions(t *testing.T) {
	t.Parallel()

	e, err := NewEnforcer(testutil.Logger(t), EnforcerOptions{
		ProxyPort: 1,
		ControlPlaneHosts: []string{
			"coder.example.com:443",
			"198.51.100.7",
			"[2001:db8::1]:8443",
			"missing.example.com:443",
			"",
		},
		Resolver: mapResolver{
			"coder.example.com": netip.MustParseAddr("203.0.113.10"),
		},
	})
	require.NoError(t, err)
	got := e.resolveExemptions(t.Context())
	require.Equal(t, []hostExemption{
		{addr: netip.MustParseAddr("203.0.113.10"), port: 443},
		{addr: netip.MustParseAddr("198.51.100.7")},
		{addr: netip.MustParseAddr("2001:db8::1"), port: 8443},
	}, got)
}
