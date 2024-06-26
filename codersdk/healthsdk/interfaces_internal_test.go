package healthsdk

import (
	"net"
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
	"tailscale.com/net/interfaces"

	"github.com/coder/coder/v2/coderd/healthcheck/health"
)

func Test_generateInterfacesReport(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name               string
		state              interfaces.State
		severity           health.Severity
		expectedInterfaces []string
		expectedWarnings   []string
	}{
		{
			name:               "Empty",
			state:              interfaces.State{},
			severity:           health.SeverityOK,
			expectedInterfaces: []string{},
		},
		{
			name: "Normal",
			state: interfaces.State{
				Interface: map[string]interfaces.Interface{
					"en0": {Interface: &net.Interface{
						MTU:   1500,
						Name:  "en0",
						Flags: net.FlagUp,
					}},
					"lo0": {Interface: &net.Interface{
						MTU:   65535,
						Name:  "lo0",
						Flags: net.FlagUp,
					}},
				},
				InterfaceIPs: map[string][]netip.Prefix{
					"en0": {
						netip.MustParsePrefix("192.168.100.1/24"),
						netip.MustParsePrefix("fe80::c13:1a92:3fa5:dd7e/64"),
					},
					"lo0": {
						netip.MustParsePrefix("127.0.0.1/8"),
						netip.MustParsePrefix("::1/128"),
						netip.MustParsePrefix("fe80::1/64"),
					},
				},
			},
			severity:           health.SeverityOK,
			expectedInterfaces: []string{"en0", "lo0"},
		},
		{
			name: "IgnoreDisabled",
			state: interfaces.State{
				Interface: map[string]interfaces.Interface{
					"en0": {Interface: &net.Interface{
						MTU:   1300,
						Name:  "en0",
						Flags: 0,
					}},
					"lo0": {Interface: &net.Interface{
						MTU:   65535,
						Name:  "lo0",
						Flags: net.FlagUp,
					}},
				},
				InterfaceIPs: map[string][]netip.Prefix{
					"en0": {netip.MustParsePrefix("192.168.100.1/24")},
					"lo0": {netip.MustParsePrefix("127.0.0.1/8")},
				},
			},
			severity:           health.SeverityOK,
			expectedInterfaces: []string{"lo0"},
		},
		{
			name: "IgnoreLinkLocalOnly",
			state: interfaces.State{
				Interface: map[string]interfaces.Interface{
					"en0": {Interface: &net.Interface{
						MTU:   1300,
						Name:  "en0",
						Flags: net.FlagUp,
					}},
					"lo0": {Interface: &net.Interface{
						MTU:   65535,
						Name:  "lo0",
						Flags: net.FlagUp,
					}},
				},
				InterfaceIPs: map[string][]netip.Prefix{
					"en0": {netip.MustParsePrefix("fe80::1:1/64")},
					"lo0": {netip.MustParsePrefix("127.0.0.1/8")},
				},
			},
			severity:           health.SeverityOK,
			expectedInterfaces: []string{"lo0"},
		},
		{
			name: "IgnoreNoAddress",
			state: interfaces.State{
				Interface: map[string]interfaces.Interface{
					"en0": {Interface: &net.Interface{
						MTU:   1300,
						Name:  "en0",
						Flags: net.FlagUp,
					}},
					"lo0": {Interface: &net.Interface{
						MTU:   65535,
						Name:  "lo0",
						Flags: net.FlagUp,
					}},
				},
				InterfaceIPs: map[string][]netip.Prefix{
					"en0": {},
					"lo0": {netip.MustParsePrefix("127.0.0.1/8")},
				},
			},
			severity:           health.SeverityOK,
			expectedInterfaces: []string{"lo0"},
		},
		{
			name: "SmallMTUTunnel",
			state: interfaces.State{
				Interface: map[string]interfaces.Interface{
					"en0": {Interface: &net.Interface{
						MTU:   1500,
						Name:  "en0",
						Flags: net.FlagUp,
					}},
					"lo0": {Interface: &net.Interface{
						MTU:   65535,
						Name:  "lo0",
						Flags: net.FlagUp,
					}},
					"tun0": {Interface: &net.Interface{
						MTU:   1280,
						Name:  "tun0",
						Flags: net.FlagUp,
					}},
				},
				InterfaceIPs: map[string][]netip.Prefix{
					"en0":  {netip.MustParsePrefix("192.168.100.1/24")},
					"tun0": {netip.MustParsePrefix("10.3.55.9/8")},
					"lo0":  {netip.MustParsePrefix("127.0.0.1/8")},
				},
			},
			severity:           health.SeverityWarning,
			expectedInterfaces: []string{"en0", "lo0", "tun0"},
			expectedWarnings:   []string{"tun0"},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := generateInterfacesReport(&tc.state)
			require.Equal(t, tc.severity, r.Severity)
			gotInterfaces := []string{}
			for _, i := range r.Interfaces {
				gotInterfaces = append(gotInterfaces, i.Name)
			}
			slices.Sort(gotInterfaces)
			slices.Sort(tc.expectedInterfaces)
			require.Equal(t, tc.expectedInterfaces, gotInterfaces)

			require.Len(t, r.Warnings, len(tc.expectedWarnings),
				"expected %d warnings, got %d", len(tc.expectedWarnings), len(r.Warnings))
			for _, name := range tc.expectedWarnings {
				found := false
				for _, w := range r.Warnings {
					if strings.Contains(w.String(), name) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("missing warning for %s", name)
				}
			}
		})
	}
}
