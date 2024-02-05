package tailnet_test

import (
	"encoding/json"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"

	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
)

func TestNode(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		node tailnet.Node
	}{
		{
			name: "Zero",
		},
		{
			name: "AllFields",
			node: tailnet.Node{
				ID:            33,
				AsOf:          time.Now(),
				Key:           key.NewNode().Public(),
				DiscoKey:      key.NewDisco().Public(),
				PreferredDERP: 12,
				DERPLatency: map[string]float64{
					"1":  0.2,
					"12": 0.3,
				},
				DERPForcedWebsocket: map[int]string{
					1: "forced",
				},
				Addresses: []netip.Prefix{
					netip.MustParsePrefix("10.0.0.0/8"),
					netip.MustParsePrefix("ff80::aa:1/128"),
				},
				AllowedIPs: []netip.Prefix{
					netip.MustParsePrefix("10.0.0.0/8"),
					netip.MustParsePrefix("ff80::aa:1/128"),
				},
				Endpoints: []string{
					"192.168.0.1:3305",
					"[ff80::aa:1]:2049",
				},
			},
		},
		{
			name: "dbtime",
			node: tailnet.Node{
				AsOf: dbtime.Now(),
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, err := tailnet.NodeToProto(&tc.node)
			require.NoError(t, err)

			inv, err := tailnet.ProtoToNode(p)
			require.NoError(t, err)
			require.Equal(t, tc.node.ID, inv.ID)
			require.True(t, tc.node.AsOf.Equal(inv.AsOf))
			require.Equal(t, tc.node.Key, inv.Key)
			require.Equal(t, tc.node.DiscoKey, inv.DiscoKey)
			require.Equal(t, tc.node.PreferredDERP, inv.PreferredDERP)
			require.Equal(t, tc.node.DERPLatency, inv.DERPLatency)
			require.Equal(t, len(tc.node.DERPForcedWebsocket), len(inv.DERPForcedWebsocket))
			for k, v := range inv.DERPForcedWebsocket {
				nv, ok := tc.node.DERPForcedWebsocket[k]
				require.True(t, ok)
				require.Equal(t, nv, v)
			}
			require.ElementsMatch(t, tc.node.Addresses, inv.Addresses)
			require.ElementsMatch(t, tc.node.AllowedIPs, inv.AllowedIPs)
			require.ElementsMatch(t, tc.node.Endpoints, inv.Endpoints)
		})
	}
}

func TestUUIDToByteSlice(t *testing.T) {
	t.Parallel()
	u := uuid.New()
	b := tailnet.UUIDToByteSlice(u)
	u2, err := uuid.FromBytes(b)
	require.NoError(t, err)
	require.Equal(t, u, u2)

	b = tailnet.UUIDToByteSlice(uuid.Nil)
	u2, err = uuid.FromBytes(b)
	require.NoError(t, err)
	require.Equal(t, uuid.Nil, u2)
}

func TestOnlyNodeUpdates(t *testing.T) {
	t.Parallel()
	node := &tailnet.Node{ID: tailcfg.NodeID(1)}
	p, err := tailnet.NodeToProto(node)
	require.NoError(t, err)
	resp := &proto.CoordinateResponse{
		PeerUpdates: []*proto.CoordinateResponse_PeerUpdate{
			{
				Id:   []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
				Kind: proto.CoordinateResponse_PeerUpdate_NODE,
				Node: p,
			},
			{
				Id:     []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2},
				Kind:   proto.CoordinateResponse_PeerUpdate_DISCONNECTED,
				Reason: "disconnected",
			},
			{
				Id:     []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3},
				Kind:   proto.CoordinateResponse_PeerUpdate_LOST,
				Reason: "disconnected",
			},
			{
				Id: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4},
			},
		},
	}
	nodes, err := tailnet.OnlyNodeUpdates(resp)
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	require.Equal(t, tailcfg.NodeID(1), nodes[0].ID)
}

func TestSingleNodeUpdate(t *testing.T) {
	t.Parallel()
	node := &tailnet.Node{ID: tailcfg.NodeID(1)}
	u := uuid.New()
	resp, err := tailnet.SingleNodeUpdate(u, node, "unit test")
	require.NoError(t, err)
	require.Len(t, resp.PeerUpdates, 1)
	up := resp.PeerUpdates[0]
	require.Equal(t, proto.CoordinateResponse_PeerUpdate_NODE, up.Kind)
	u2, err := uuid.FromBytes(up.Id)
	require.NoError(t, err)
	require.Equal(t, u, u2)
	require.Equal(t, "unit test", up.Reason)
	require.EqualValues(t, 1, up.Node.Id)
}

func TestDERPMap(t *testing.T) {
	t.Parallel()

	// Tailscale DERP map on 2023-11-20 for testing purposes.
	tailscaleDERPMap, err := os.ReadFile("testdata/tailscale_derpmap.json")
	require.NoError(t, err)

	derpMap := &tailcfg.DERPMap{}
	err = json.Unmarshal(tailscaleDERPMap, derpMap)
	require.NoError(t, err)

	// The tailscale DERPMap doesn't have HomeParams.
	derpMap.HomeParams = &tailcfg.DERPHomeParams{
		RegionScore: map[int]float64{
			1: 2,
			2: 3,
		},
	}

	// Add a region and node that uses every single field.
	derpMap.Regions[999] = &tailcfg.DERPRegion{
		RegionID:      999,
		EmbeddedRelay: true,
		RegionCode:    "zzz",
		RegionName:    "Cool Region",
		Avoid:         true,

		Nodes: []*tailcfg.DERPNode{
			{
				Name:             "zzz1",
				RegionID:         999,
				HostName:         "coolderp.com",
				IPv4:             "1.2.3.4",
				IPv6:             "2001:db8::1",
				STUNPort:         1234,
				STUNOnly:         true,
				DERPPort:         5678,
				InsecureForTests: true,
				ForceHTTP:        true,
				STUNTestIP:       "5.6.7.8",
				CanPort80:        true,
			},
		},
	}

	protoMap := tailnet.DERPMapToProto(derpMap)
	require.NotNil(t, protoMap)

	derpMap2 := tailnet.DERPMapFromProto(protoMap)
	require.NotNil(t, derpMap2)

	require.Equal(t, derpMap, derpMap2)
}
