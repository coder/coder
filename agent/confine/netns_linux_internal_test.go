//go:build linux

package confine

import (
	"context"
	"net/netip"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubnetAllocator(t *testing.T) {
	t.Parallel()

	pool := netip.MustParsePrefix("192.0.2.0/29")
	tests := []struct {
		name        string
		used        []netip.Prefix
		allocations int
		want        []subnetLease
		wantErr     string
	}{
		{
			name:        "allocates sequential subnets",
			allocations: 2,
			want: []subnetLease{
				{
					Prefix: netip.MustParsePrefix("192.0.2.0/30"),
					Host:   netip.MustParseAddr("192.0.2.1"),
					Peer:   netip.MustParseAddr("192.0.2.2"),
				},
				{
					Prefix: netip.MustParsePrefix("192.0.2.4/30"),
					Host:   netip.MustParseAddr("192.0.2.5"),
					Peer:   netip.MustParseAddr("192.0.2.6"),
				},
			},
		},
		{
			name: "skips route collision",
			used: []netip.Prefix{
				netip.MustParsePrefix("192.0.2.0/31"),
			},
			allocations: 1,
			want: []subnetLease{
				{
					Prefix: netip.MustParsePrefix("192.0.2.4/30"),
					Host:   netip.MustParseAddr("192.0.2.5"),
					Peer:   netip.MustParseAddr("192.0.2.6"),
				},
			},
		},
		{
			name: "skips interface address collision",
			used: []netip.Prefix{
				netip.MustParsePrefix("192.0.2.2/32"),
			},
			allocations: 1,
			want: []subnetLease{
				{
					Prefix: netip.MustParsePrefix("192.0.2.4/30"),
					Host:   netip.MustParseAddr("192.0.2.5"),
					Peer:   netip.MustParseAddr("192.0.2.6"),
				},
			},
		},
		{
			name: "exhausted by collisions",
			used: []netip.Prefix{
				netip.MustParsePrefix("192.0.2.0/29"),
			},
			allocations: 1,
			wantErr:     `netns subnet pool "192.0.2.0/29" is exhausted`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			allocator := newSubnetAllocator(func(context.Context) ([]netip.Prefix, error) {
				return tt.used, nil
			})
			var got []subnetLease
			for range tt.allocations {
				lease, err := allocator.Allocate(t.Context(), pool)
				if tt.wantErr != "" {
					require.EqualError(t, err, tt.wantErr)
					return
				}
				require.NoError(t, err)
				got = append(got, lease)
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestSubnetAllocatorExhaustion(t *testing.T) {
	t.Parallel()

	allocator := newSubnetAllocator(func(context.Context) ([]netip.Prefix, error) {
		return nil, nil
	})
	pool := netip.MustParsePrefix("192.0.2.0/30")

	_, err := allocator.Allocate(t.Context(), pool)
	require.NoError(t, err)
	_, err = allocator.Allocate(t.Context(), pool)
	require.EqualError(t, err, `netns subnet pool "192.0.2.0/30" is exhausted`)
}

func TestSubnetAllocatorReleaseReuse(t *testing.T) {
	t.Parallel()

	allocator := newSubnetAllocator(func(context.Context) ([]netip.Prefix, error) {
		return nil, nil
	})
	pool := netip.MustParsePrefix("192.0.2.0/29")

	first, err := allocator.Allocate(t.Context(), pool)
	require.NoError(t, err)
	second, err := allocator.Allocate(t.Context(), pool)
	require.NoError(t, err)
	require.NotEqual(t, first.Prefix, second.Prefix)

	allocator.Release(first.Prefix)
	reused, err := allocator.Allocate(t.Context(), pool)
	require.NoError(t, err)
	require.Equal(t, first, reused)
}

func TestSubnetAllocatorListerError(t *testing.T) {
	t.Parallel()

	allocator := newSubnetAllocator(func(context.Context) ([]netip.Prefix, error) {
		return nil, context.Canceled
	})
	_, err := allocator.Allocate(t.Context(), netip.MustParsePrefix("192.0.2.0/30"))
	require.ErrorContains(t, err, "list host network prefixes: context canceled")
}

func TestSubnetAllocatorRejectsInvalidPools(t *testing.T) {
	t.Parallel()

	allocator := newSubnetAllocator(func(context.Context) ([]netip.Prefix, error) {
		return nil, nil
	})

	_, err := allocator.Allocate(t.Context(), netip.MustParsePrefix("2001:db8::/64"))
	require.EqualError(t, err, `netns subnet pool "2001:db8::/64" is not IPv4`)

	_, err = allocator.Allocate(t.Context(), netip.MustParsePrefix("192.0.2.0/31"))
	require.EqualError(t, err, `netns subnet pool "192.0.2.0/31" is smaller than /30`)
}

func TestChildIPv4Rules(t *testing.T) {
	t.Parallel()

	ports := NetworkNamespacePorts{HTTP: 18080, SNI: 18443, DNS: 1053}
	require.Equal(t, [][]string{
		{"-w", "-t", "nat", "-A", "OUTPUT", "-d", "127.0.0.0/8", "-j", "RETURN"},
		{"-w", "-t", "nat", "-A", "OUTPUT", "-d", "100.115.92.1", "-p", "tcp", "-m", "multiport", "--dports", "18080,18443,1053", "-j", "RETURN"},
		{"-w", "-t", "nat", "-A", "OUTPUT", "-d", "100.115.92.1", "-p", "udp", "--dport", "1053", "-j", "RETURN"},
		{"-w", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "80", "-j", "DNAT", "--to-destination", "100.115.92.1:18080"},
		{"-w", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "443", "-j", "DNAT", "--to-destination", "100.115.92.1:18443"},
		{"-w", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "53", "-j", "DNAT", "--to-destination", "100.115.92.1:1053"},
		{"-w", "-t", "nat", "-A", "OUTPUT", "-p", "udp", "--dport", "53", "-j", "DNAT", "--to-destination", "100.115.92.1:1053"},
		{"-w", "-P", "INPUT", "DROP"},
		{"-w", "-P", "OUTPUT", "DROP"},
		{"-w", "-P", "FORWARD", "DROP"},
		{"-w", "-A", "OUTPUT", "-o", "lo", "-j", "ACCEPT"},
		{"-w", "-A", "INPUT", "-i", "lo", "-j", "ACCEPT"},
		{"-w", "-A", "OUTPUT", "-d", "100.115.92.1", "-p", "tcp", "-m", "multiport", "--dports", "18080,18443,1053", "-m", "conntrack", "--ctstate", "NEW,ESTABLISHED", "-j", "ACCEPT"},
		{"-w", "-A", "OUTPUT", "-d", "100.115.92.1", "-p", "udp", "--dport", "1053", "-m", "conntrack", "--ctstate", "NEW,ESTABLISHED", "-j", "ACCEPT"},
		{"-w", "-A", "INPUT", "-s", "100.115.92.1", "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"},
	}, childIPv4Rules("100.115.92.1", ports))
}

func TestHostIPv4Rules(t *testing.T) {
	t.Parallel()

	ports := NetworkNamespacePorts{HTTP: 18080, SNI: 18443, DNS: 1053}
	require.Equal(t, [][]string{
		{"-w", "-I", "INPUT", "1", "-i", "cc-h-deadbeef", "-j", "DROP"},
		{"-w", "-I", "INPUT", "1", "-i", "cc-h-deadbeef", "-s", "100.115.92.2", "-d", "100.115.92.1", "-p", "udp", "--dport", "1053", "-m", "conntrack", "--ctstate", "NEW,ESTABLISHED", "-j", "ACCEPT"},
		{"-w", "-I", "INPUT", "1", "-i", "cc-h-deadbeef", "-s", "100.115.92.2", "-d", "100.115.92.1", "-p", "tcp", "-m", "multiport", "--dports", "18080,18443,1053", "-m", "conntrack", "--ctstate", "NEW,ESTABLISHED", "-j", "ACCEPT"},
		{"-w", "-I", "FORWARD", "1", "-i", "cc-h-deadbeef", "-j", "DROP"},
		{"-w", "-I", "FORWARD", "1", "-o", "cc-h-deadbeef", "-j", "DROP"},
	}, hostIPv4Rules("cc-h-deadbeef", "100.115.92.1", "100.115.92.2", ports))
}

func TestIPv6Rules(t *testing.T) {
	t.Parallel()

	require.Equal(t, [][]string{
		{"-w", "-P", "INPUT", "DROP"},
		{"-w", "-P", "OUTPUT", "DROP"},
		{"-w", "-P", "FORWARD", "DROP"},
	}, childIPv6Rules())
	require.Equal(t, [][]string{
		{"-w", "-I", "INPUT", "1", "-i", "cc-h-deadbeef", "-j", "DROP"},
		{"-w", "-I", "OUTPUT", "1", "-o", "cc-h-deadbeef", "-j", "DROP"},
		{"-w", "-I", "FORWARD", "1", "-i", "cc-h-deadbeef", "-j", "DROP"},
		{"-w", "-I", "FORWARD", "2", "-o", "cc-h-deadbeef", "-j", "DROP"},
	}, hostIPv6Rules("cc-h-deadbeef"))
}

func TestDeleteArgsForInsertedRule(t *testing.T) {
	t.Parallel()

	for _, rules := range [][][]string{
		hostIPv4Rules("cc-h-deadbeef", "100.115.92.1", "100.115.92.2", NetworkNamespacePorts{HTTP: 18080, SNI: 18443, DNS: 1053}),
		hostIPv6Rules("cc-h-deadbeef"),
	} {
		for _, insertArgs := range rules {
			deleteArgs, err := deleteArgsForInsertedRule(insertArgs)
			require.NoError(t, err)
			require.Equal(t, append([]string{"-w", "-D", insertArgs[2]}, insertArgs[4:]...), deleteArgs)
		}
	}

	_, err := deleteArgsForInsertedRule([]string{"-w", "-A", "INPUT", "-j", "DROP"})
	require.ErrorContains(t, err, "invalid inserted firewall rule")
}

func TestNetworkNamespacePortsValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ports   NetworkNamespacePorts
		wantErr string
	}{
		{name: "valid", ports: NetworkNamespacePorts{HTTP: 1, SNI: 2, DNS: 3}},
		{name: "missing HTTP", ports: NetworkNamespacePorts{SNI: 2, DNS: 3}, wantErr: "HTTP listener port must be non-zero"},
		{name: "missing SNI", ports: NetworkNamespacePorts{HTTP: 1, DNS: 3}, wantErr: "SNI listener port must be non-zero"},
		{name: "missing DNS", ports: NetworkNamespacePorts{HTTP: 1, SNI: 2}, wantErr: "DNS listener port must be non-zero"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.ports.validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tt.wantErr)
		})
	}
}

func TestNetworkNamespaceCommandArgsRequiresConfiguration(t *testing.T) {
	t.Parallel()

	netns := &NetworkNamespace{ipPath: "/sbin/ip", name: "test-netns"}
	_, err := netns.CommandArgs([]string{"echo", "hello"})
	require.EqualError(t, err, "execute in network namespace before egress is configured")

	netns.configured = true
	args, err := netns.CommandArgs([]string{"echo", "hello"})
	require.NoError(t, err)
	require.Equal(t, []string{"/sbin/ip", "netns", "exec", "test-netns", "echo", "hello"}, args)
}

func TestNetworkNamespaceIntegration(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("network namespace integration test is disabled in short mode")
	}
	if os.Geteuid() != 0 {
		t.Skip("network namespace integration test requires root")
	}
	if err := PreflightNetworkNamespace(t.Context()); err != nil {
		t.Skipf("network namespace enforcement is unsupported: %v", err)
	}

	netns, err := OpenNetworkNamespace(t.Context(), NetworkNamespaceOptions{})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, netns.Close())
	})

	route, err := runCommandOutput(t.Context(), netns.ipPath, "-n", netns.name, "route", "show", "default")
	require.NoError(t, err)
	require.Contains(t, string(route), "default via "+netns.hostIP+" dev "+netns.peerVeth)

	err = netns.ConfigureEgress(t.Context(), NetworkNamespacePorts{HTTP: 18080, SNI: 18443, DNS: 1053})
	require.NoError(t, err)
	_, err = netns.CommandArgs([]string{"true"})
	require.NoError(t, err)
}
