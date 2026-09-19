package agentegress

import (
	"context"
	"net"
	"net/netip"
	"strings"
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
			out = append(out, strings.Join(r, " "))
		}
		return out
	}

	// Both families are always offered; each rule set must keep only its
	// own.
	exemptions := []resolvedExemption{
		{Proto: "tcp", Addr: netip.MustParseAddr("203.0.113.10"), Port: 443},
		{Proto: "udp", Addr: netip.MustParseAddr("198.51.100.7"), Port: 41641},
		{Proto: "tcp", Addr: netip.MustParseAddr("2001:db8::1"), Port: 443},
	}
	tests := []struct {
		name       string
		ports      proxyPorts
		exemptions []resolvedExemption
		family     ipFamily
		wantNat    []string
		wantFilter []string
	}{
		{
			name:  "no exemptions",
			ports: proxyPorts{tcp: 40001, dns: 40002, udp: 40003},
			wantNat: []string{
				"-A CODER_EGRESS -m mark --mark 0x4350 -j RETURN",
				"-A CODER_EGRESS -o lo -j RETURN",
				"-A CODER_EGRESS -p udp --dport 53 -j REDIRECT --to-ports 40002",
				"-A CODER_EGRESS -p tcp --dport 53 -j REDIRECT --to-ports 40002",
				"-A CODER_EGRESS -p udp -j REDIRECT --to-ports 40003",
				"-A CODER_EGRESS -p tcp -j REDIRECT --to-ports 40001",
			},
			wantFilter: []string{
				"-A CODER_EGRESS_FILTER -o lo -j ACCEPT",
				"-A CODER_EGRESS_FILTER -p icmp -j DROP",
			},
		},
		{
			name:       "ipv4 control plane exemptions and resolvers",
			ports:      proxyPorts{tcp: 40001, dns: 40002, udp: 40003},
			exemptions: exemptions,
			wantNat: []string{
				"-A CODER_EGRESS -m mark --mark 0x4350 -j RETURN",
				"-A CODER_EGRESS -o lo -j RETURN",
				"-A CODER_EGRESS -d 203.0.113.10 -p tcp --dport 443 -j RETURN",
				"-A CODER_EGRESS -d 198.51.100.7 -p udp --dport 41641 -j RETURN",
				"-A CODER_EGRESS -p udp --dport 53 -j REDIRECT --to-ports 40002",
				"-A CODER_EGRESS -p tcp --dport 53 -j REDIRECT --to-ports 40002",
				"-A CODER_EGRESS -p udp -j REDIRECT --to-ports 40003",
				"-A CODER_EGRESS -p tcp -j REDIRECT --to-ports 40001",
			},
			wantFilter: []string{
				"-A CODER_EGRESS_FILTER -o lo -j ACCEPT",
				"-A CODER_EGRESS_FILTER -p icmp -j DROP",
			},
		},
		{
			name:       "ipv6 exemption keeps neighbor discovery",
			ports:      proxyPorts{tcp: 8080, dns: 8081, udp: 8082},
			exemptions: exemptions,
			family:     ipv6,
			wantNat: []string{
				"-A CODER_EGRESS -m mark --mark 0x4350 -j RETURN",
				"-A CODER_EGRESS -o lo -j RETURN",
				"-A CODER_EGRESS -d 2001:db8::1 -p tcp --dport 443 -j RETURN",
				"-A CODER_EGRESS -p udp --dport 53 -j REDIRECT --to-ports 8081",
				"-A CODER_EGRESS -p tcp --dport 53 -j REDIRECT --to-ports 8081",
				"-A CODER_EGRESS -p udp -j REDIRECT --to-ports 8082",
				"-A CODER_EGRESS -p tcp -j REDIRECT --to-ports 8080",
			},
			wantFilter: []string{
				"-A CODER_EGRESS_FILTER -o lo -j ACCEPT",
				"-A CODER_EGRESS_FILTER -p icmpv6 --icmpv6-type 133 -j ACCEPT",
				"-A CODER_EGRESS_FILTER -p icmpv6 --icmpv6-type 134 -j ACCEPT",
				"-A CODER_EGRESS_FILTER -p icmpv6 --icmpv6-type 135 -j ACCEPT",
				"-A CODER_EGRESS_FILTER -p icmpv6 --icmpv6-type 136 -j ACCEPT",
				"-A CODER_EGRESS_FILTER -p icmpv6 --icmpv6-type 137 -j ACCEPT",
				"-A CODER_EGRESS_FILTER -p icmpv6 -j DROP",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rs := buildRules(tt.ports, tt.exemptions, tt.family, udpModeRedirect)
			require.Equal(t, tt.wantNat, toStrings(rs["nat"]))
			require.Equal(t, tt.wantFilter, toStrings(rs["filter"]))
		})
	}
}

func TestBuildTPROXYRules(t *testing.T) {
	t.Parallel()

	rs := buildTPROXYRules(proxyPorts{udp: 40003}, []resolvedExemption{
		{Proto: "tcp", Addr: netip.MustParseAddr("203.0.113.10"), Port: 443},
		{Proto: "udp", Addr: netip.MustParseAddr("198.51.100.7"), Port: 41641},
		{Proto: "udp", Addr: netip.MustParseAddr("2001:db8::1"), Port: 41641},
	})
	join := func(rules []rule) []string {
		out := make([]string, 0, len(rules))
		for _, r := range rules {
			out = append(out, strings.Join(r, " "))
		}
		return out
	}
	require.Equal(t, []string{
		"-A CODER_EGRESS_UDP -m mark --mark 0x4350 -j RETURN",
		"-A CODER_EGRESS_UDP -o lo -j RETURN",
		"-A CODER_EGRESS_UDP -d 127.0.0.0/8 -j RETURN",
		"-A CODER_EGRESS_UDP -d 198.51.100.7 -p udp --dport 41641 -j RETURN",
		"-A CODER_EGRESS_UDP -p udp --dport 53 -j RETURN",
		"-A CODER_EGRESS_UDP -p udp -j MARK --set-mark 0x434f",
	}, join(rs["output"]))
	require.Equal(t, []string{
		"-A CODER_EGRESS_TPROXY -p udp -m mark --mark 0x434f -j TPROXY --on-ip 127.0.0.1 --on-port 40003 --tproxy-mark 0x434f",
	}, join(rs["prerouting"]))
}

func TestParseResolvConf(t *testing.T) {
	t.Parallel()

	got := parseResolvConf(`
# Generated by something.
nameserver 1.1.1.1
nameserver 8.8.8.8 ; trailing comment
nameserver 1.1.1.1
nameserver fe80::1%eth0
nameserver not-an-address
search example.com
options edns0
nameserver
`)
	require.Equal(t, []netip.Addr{
		netip.MustParseAddr("1.1.1.1"),
		netip.MustParseAddr("8.8.8.8"),
		netip.MustParseAddr("fe80::1"),
	}, got)
	require.Empty(t, parseResolvConf(""))
}

func TestResolveExemptions(t *testing.T) {
	t.Parallel()

	e, err := NewEnforcer(testutil.Logger(t), EnforcerOptions{
		ProxyPort: 1,
		DNSPort:   2,
		UDPPort:   3,
		ControlPlaneHosts: []string{
			"tcp/coder.example.com:443",
			"udp/198.51.100.7:41641",
			"tcp/[2001:db8::1]:8443",
			"tcp/missing.example.com:443",
			"tcp/coder.example.com:443",
		},
		Resolver: mapResolver{
			"coder.example.com": netip.MustParseAddr("203.0.113.10"),
		},
	})
	require.NoError(t, err)
	require.Equal(t, []resolvedExemption{
		{Proto: "tcp", Addr: netip.MustParseAddr("203.0.113.10"), Port: 443},
		{Proto: "udp", Addr: netip.MustParseAddr("198.51.100.7"), Port: 41641},
		{Proto: "tcp", Addr: netip.MustParseAddr("2001:db8::1"), Port: 8443},
	}, e.resolveExemptions(t.Context()))
}
