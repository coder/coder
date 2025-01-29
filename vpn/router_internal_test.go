package vpn

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
	"tailscale.com/wgengine/router"
)

func TestConvertRouterConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cfg      router.Config
		expected *NetworkSettingsRequest
	}{
		{
			name: "IPv4 and IPv6 configuration",
			cfg: router.Config{
				LocalAddrs:  []netip.Prefix{netip.MustParsePrefix("100.64.0.1/32"), netip.MustParsePrefix("fd7a:115c:a1e0::1/128")},
				Routes:      []netip.Prefix{netip.MustParsePrefix("192.168.0.0/24"), netip.MustParsePrefix("fd00::/64")},
				LocalRoutes: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("2001:db8::/32")},
				NewMTU:      1500,
			},
			expected: &NetworkSettingsRequest{
				Mtu: 1500,
				Ipv4Settings: &NetworkSettingsRequest_IPv4Settings{
					Addrs:       []string{"100.64.0.1"},
					SubnetMasks: []string{"255.255.255.255"},
					IncludedRoutes: []*NetworkSettingsRequest_IPv4Settings_IPv4Route{
						{Destination: "192.168.0.0", Mask: "255.255.255.0", Router: ""},
					},
					ExcludedRoutes: []*NetworkSettingsRequest_IPv4Settings_IPv4Route{
						{Destination: "10.0.0.0", Mask: "255.0.0.0", Router: ""},
					},
				},
				Ipv6Settings: &NetworkSettingsRequest_IPv6Settings{
					Addrs:         []string{"fd7a:115c:a1e0::1"},
					PrefixLengths: []uint32{128},
					IncludedRoutes: []*NetworkSettingsRequest_IPv6Settings_IPv6Route{
						{Destination: "fd00::", PrefixLength: 64, Router: ""},
					},
					ExcludedRoutes: []*NetworkSettingsRequest_IPv6Settings_IPv6Route{
						{Destination: "2001:db8::", PrefixLength: 32, Router: ""},
					},
				},
			},
		},
		{
			name: "Empty",
			cfg:  router.Config{},
			expected: &NetworkSettingsRequest{
				Ipv4Settings: nil,
				Ipv6Settings: nil,
			},
		},
	}
	//nolint:paralleltest // outdated rule
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := convertRouterConfig(tt.cfg)
			require.Equal(t, tt.expected, result)
		})
	}
}
