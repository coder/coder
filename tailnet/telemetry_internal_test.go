package tailnet

import (
	"fmt"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
	"tailscale.com/tailcfg"
	"tailscale.com/types/netmap"

	"github.com/coder/coder/v2/tailnet/proto"
)

func TestTelemetryStore(t *testing.T) {
	t.Parallel()

	t.Run("CreateEvent", func(t *testing.T) {
		t.Parallel()

		remotePrefix := TailscaleServicePrefix.RandomPrefix()
		remoteIP := remotePrefix.Addr()
		application := "test"

		nm := &netmap.NetworkMap{
			SelfNode: &tailcfg.Node{
				ID:   0,
				DERP: "127.3.3.40:999",
			},
			Peers: []*tailcfg.Node{
				{
					ID: 1,
					Addresses: []netip.Prefix{
						TailscaleServicePrefix.RandomPrefix(),
						TailscaleServicePrefix.RandomPrefix(),
					},
				},
				{
					ID: 2,
					Addresses: []netip.Prefix{
						remotePrefix,
						TailscaleServicePrefix.RandomPrefix(),
						TailscaleServicePrefix.RandomPrefix(),
					},
				},
			},
			DERPMap: &tailcfg.DERPMap{
				HomeParams: &tailcfg.DERPHomeParams{
					RegionScore: map[int]float64{
						999: 1.0,
					},
				},
				Regions: map[int]*tailcfg.DERPRegion{
					999: {
						RegionID:      999,
						RegionCode:    "zzz",
						RegionName:    "Cool Region",
						EmbeddedRelay: true,
						Avoid:         false,
					},
				},
				OmitDefaultRegions: false,
			},
		}

		telemetry, err := newTelemetryStore()
		require.NoError(t, err)
		telemetry.markConnected(&remoteIP, application)
		telemetry.updateNetworkMap(nm)
		e := telemetry.newEvent()
		// DERPMapToProto already tested
		require.Equal(t, DERPMapToProto(nm.DERPMap), e.DerpMap)
		// #nosec G115 - Safe conversion in test code as node IDs are within uint64 range
		require.Equal(t, uint64(nm.Peers[1].ID), e.NodeIdRemote)
		// #nosec G115 - Safe conversion in test code as node IDs are within uint64 range
		require.Equal(t, uint64(nm.SelfNode.ID), e.NodeIdSelf)
		require.Equal(t, application, e.Application)
		require.Equal(t, nm.SelfNode.DERP, fmt.Sprintf("127.3.3.40:%d", e.HomeDerp))
	})

	t.Run("CleanIPs", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name            string
			ipv4            string
			ipv6            string
			expectedVersion int32
			expectedClass   proto.IPFields_IPClass
		}{
			{
				name:          "Public",
				ipv4:          "142.250.71.78",
				ipv6:          "2404:6800:4006:812::200e",
				expectedClass: proto.IPFields_PUBLIC,
			},
			{
				name:          "Private",
				ipv4:          "192.168.0.1",
				ipv6:          "fd12:3456:789a:1::1",
				expectedClass: proto.IPFields_PRIVATE,
			},
			{
				name:          "LinkLocal",
				ipv4:          "169.254.1.1",
				ipv6:          "fe80::1",
				expectedClass: proto.IPFields_LINK_LOCAL,
			},
			{
				name:          "Loopback",
				ipv4:          "127.0.0.1",
				ipv6:          "::1",
				expectedClass: proto.IPFields_LOOPBACK,
			},
			{
				name:          "IPv4Mapped",
				ipv4:          "1.2.3.4",
				ipv6:          "::ffff:1.2.3.4",
				expectedClass: proto.IPFields_PUBLIC,
			},
		}

		for _, c := range cases {
			c := c
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()
				telemetry, err := newTelemetryStore()
				require.NoError(t, err)

				telemetry.setNetInfo(&tailcfg.NetInfo{
					GlobalV4: c.ipv4,
					GlobalV6: c.ipv6,
				})

				event := telemetry.newEvent()

				v4hash := telemetry.hashAddrorHostname(c.ipv4)
				require.Equal(t, &proto.Netcheck_NetcheckIP{
					Hash: v4hash,
					Fields: &proto.IPFields{
						Version: 4,
						Class:   c.expectedClass,
					},
				}, event.LatestNetcheck.GlobalV4)

				v6hash := telemetry.hashAddrorHostname(c.ipv6)
				require.Equal(t, &proto.Netcheck_NetcheckIP{
					Hash: v6hash,
					Fields: &proto.IPFields{
						Version: 6,
						Class:   c.expectedClass,
					},
				}, event.LatestNetcheck.GlobalV6)
			})
		}
	})

	t.Run("DerpMapClean", func(t *testing.T) {
		t.Parallel()
		telemetry, err := newTelemetryStore()
		require.NoError(t, err)

		derpMap := &tailcfg.DERPMap{
			Regions: make(map[int]*tailcfg.DERPRegion),
		}
		derpMap.Regions[998] = &tailcfg.DERPRegion{
			RegionID:      998,
			EmbeddedRelay: true,
			RegionCode:    "zzz",
			RegionName:    "Cool Region",
			Avoid:         true,

			Nodes: []*tailcfg.DERPNode{
				{
					Name:       "zzz1",
					RegionID:   998,
					HostName:   "coolderp.com",
					CertName:   "coolderpcert",
					IPv4:       "1.2.3.4",
					IPv6:       "2001:db8::1",
					STUNTestIP: "5.6.7.8",
				},
			},
		}
		derpMap.Regions[999] = &tailcfg.DERPRegion{
			RegionID:      999,
			EmbeddedRelay: true,
			RegionCode:    "zzo",
			RegionName:    "Other Cool Region",
			Avoid:         true,
			Nodes: []*tailcfg.DERPNode{
				{
					Name:       "zzo1",
					HostName:   "coolderp.com",
					CertName:   "coolderpcert",
					IPv4:       "1.2.3.4",
					IPv6:       "2001:db8::1",
					STUNTestIP: "5.6.7.8",
				},
			},
		}
		telemetry.updateDerpMap(derpMap)

		event := telemetry.newEvent()
		require.Len(t, event.DerpMap.Regions[999].Nodes, 1)
		node := event.DerpMap.Regions[999].Nodes[0]
		require.NotContains(t, node.HostName, "coolderp.com")
		require.NotContains(t, node.Ipv4, "1.2.3.4")
		require.NotContains(t, node.Ipv6, "2001:db8::1")
		require.NotContains(t, node.StunTestIp, "5.6.7.8")
		otherNode := event.DerpMap.Regions[998].Nodes[0]
		require.Equal(t, otherNode.HostName, node.HostName)
		require.Equal(t, otherNode.Ipv4, node.Ipv4)
		require.Equal(t, otherNode.Ipv6, node.Ipv6)
		require.Equal(t, otherNode.StunTestIp, node.StunTestIp)
	})
}
