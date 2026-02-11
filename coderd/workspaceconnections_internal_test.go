package coderd

import (
	"net"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/tailnet"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
)

func testTailnetIP(last byte) netip.Addr {
	return netip.MustParseAddr("fd7a:115c:a1e0:49d6:b259:b6ac:b1e4:" + string(rune('0'+last)))
}

func makeTunnelPeer(addr netip.Addr, status tailnetproto.CoordinateResponse_PeerUpdate_Kind, start time.Time) *tailnet.TunnelPeerInfo {
	return &tailnet.TunnelPeerInfo{
		ID: uuid.New(),
		Node: &tailnetproto.Node{
			Addresses: []string{netip.PrefixFrom(addr, 128).String()},
		},
		Status: status,
		Start:  start,
	}
}

func makeConnectionLog(addr netip.Addr, connType database.ConnectionType, connectTime time.Time) database.GetOngoingAgentConnectionsLast24hRow {
	ip16 := addr.As16()
	return database.GetOngoingAgentConnectionsLast24hRow{
		ConnectTime: connectTime,
		Ip: pqtype.Inet{
			Valid: true,
			IPNet: net.IPNet{
				IP:   ip16[:],
				Mask: net.CIDRMask(128, 128),
			},
		},
		Type: connType,
	}
}

// func TestMergeWorkspaceConnections(t *testing.T) {
// 	t.Parallel()
// 	agentID := uuid.UUID{1}

// 	now := time.Now().UTC().Truncate(time.Millisecond)

// 	t.Run("BothEmpty", func(t *testing.T) {
// 		t.Parallel()
// 		result := mergeWorkspaceConnections(agentID, nil, nil)
// 		assert.Nil(t, result)
// 	})

// 	t.Run("LogsOnly", func(t *testing.T) {
// 		t.Parallel()

// 		ip1 := testTailnetIP(1)
// 		ip2 := testTailnetIP(2)
// 		logs := []database.GetOngoingAgentConnectionsLast24hRow{
// 			makeConnectionLog(ip1, database.ConnectionTypeSsh, now),
// 			makeConnectionLog(ip2, database.ConnectionTypeVscode, now.Add(-time.Minute)),
// 		}

// 		result := mergeWorkspaceConnections(agentID, nil, logs)
// 		require.Len(t, result, 2)

// 		assert.Equal(t, codersdk.ConnectionType("ssh"), result[0].Type)
// 		assert.Equal(t, codersdk.ConnectionStatusOngoing, result[0].Status)
// 		require.NotNil(t, result[0].IP)
// 		assert.Equal(t, ip1, *result[0].IP)

// 		assert.Equal(t, codersdk.ConnectionType("vscode"), result[1].Type)
// 		assert.Equal(t, codersdk.ConnectionStatusOngoing, result[1].Status)
// 		require.NotNil(t, result[1].IP)
// 		assert.Equal(t, ip2, *result[1].IP)
// 	})

// 	t.Run("PeersOnly", func(t *testing.T) {
// 		t.Parallel()

// 		ip1 := testTailnetIP(1)
// 		ip2 := testTailnetIP(2)
// 		peers := []*tailnet.TunnelPeerInfo{
// 			makeTunnelPeer(ip1, tailnetproto.CoordinateResponse_PeerUpdate_NODE, now),
// 			makeTunnelPeer(ip2, tailnetproto.CoordinateResponse_PeerUpdate_LOST, now.Add(-time.Minute)),
// 		}

// 		result := mergeWorkspaceConnections(agentID, peers, nil)
// 		require.Len(t, result, 2)

// 		// Map iteration order is nondeterministic, find each by IP.
// 		byIP := make(map[netip.Addr]codersdk.WorkspaceConnection, len(result))
// 		for _, c := range result {
// 			require.NotNil(t, c.IP)
// 			byIP[*c.IP] = c
// 		}

// 		c1 := byIP[ip1]
// 		assert.Equal(t, codersdk.ConnectionStatusOngoing, c1.Status)
// 		assert.Empty(t, c1.Type)
// 		assert.Equal(t, now, c1.CreatedAt)

// 		c2 := byIP[ip2]
// 		assert.Equal(t, codersdk.ConnectionStatusControlLost, c2.Status)
// 		assert.Empty(t, c2.Type)
// 	})

// 	t.Run("MatchedByIP", func(t *testing.T) {
// 		t.Parallel()

// 		ip := testTailnetIP(1)
// 		peers := []*tailnet.TunnelPeerInfo{
// 			makeTunnelPeer(ip, tailnetproto.CoordinateResponse_PeerUpdate_NODE, now),
// 		}
// 		logs := []database.GetOngoingAgentConnectionsLast24hRow{
// 			makeConnectionLog(ip, database.ConnectionTypeSsh, now.Add(-time.Second)),
// 		}

// 		result := mergeWorkspaceConnections(peers, logs)
// 		require.Len(t, result, 1)

// 		assert.Equal(t, codersdk.ConnectionType("ssh"), result[0].Type)
// 		assert.Equal(t, codersdk.ConnectionStatusOngoing, result[0].Status)
// 		require.NotNil(t, result[0].IP)
// 		assert.Equal(t, ip, *result[0].IP)
// 	})

// 	t.Run("MatchedLostPeer", func(t *testing.T) {
// 		t.Parallel()

// 		ip := testTailnetIP(1)
// 		peers := []*tailnet.TunnelPeerInfo{
// 			makeTunnelPeer(ip, tailnetproto.CoordinateResponse_PeerUpdate_LOST, now),
// 		}
// 		logs := []database.GetOngoingAgentConnectionsLast24hRow{
// 			makeConnectionLog(ip, database.ConnectionTypeSsh, now),
// 		}

// 		result := mergeWorkspaceConnections(peers, logs)
// 		require.Len(t, result, 1)

// 		assert.Equal(t, codersdk.ConnectionType("ssh"), result[0].Type)
// 		assert.Equal(t, codersdk.ConnectionStatusControlLost, result[0].Status)
// 	})

// 	t.Run("PartialMatch", func(t *testing.T) {
// 		t.Parallel()

// 		ip1 := testTailnetIP(1)
// 		ip2 := testTailnetIP(2)
// 		peers := []*tailnet.TunnelPeerInfo{
// 			makeTunnelPeer(ip1, tailnetproto.CoordinateResponse_PeerUpdate_NODE, now),
// 			makeTunnelPeer(ip2, tailnetproto.CoordinateResponse_PeerUpdate_LOST, now.Add(-time.Minute)),
// 		}
// 		logs := []database.GetOngoingAgentConnectionsLast24hRow{
// 			makeConnectionLog(ip1, database.ConnectionTypeVscode, now.Add(-time.Second)),
// 		}

// 		result := mergeWorkspaceConnections(peers, logs)
// 		require.Len(t, result, 2)

// 		// First entry is the matched log (preserves log order).
// 		assert.Equal(t, codersdk.ConnectionType("vscode"), result[0].Type)
// 		assert.Equal(t, codersdk.ConnectionStatusOngoing, result[0].Status)
// 		require.NotNil(t, result[0].IP)
// 		assert.Equal(t, ip1, *result[0].IP)

// 		// Second entry is the unmatched peer.
// 		assert.Empty(t, result[1].Type)
// 		assert.Equal(t, codersdk.ConnectionStatusControlLost, result[1].Status)
// 		require.NotNil(t, result[1].IP)
// 		assert.Equal(t, ip2, *result[1].IP)
// 	})

// 	t.Run("MultiSessionSameIP", func(t *testing.T) {
// 		t.Parallel()

// 		ip := testTailnetIP(1)
// 		peers := []*tailnet.TunnelPeerInfo{
// 			makeTunnelPeer(ip, tailnetproto.CoordinateResponse_PeerUpdate_LOST, now),
// 		}
// 		logs := []database.GetOngoingAgentConnectionsLast24hRow{
// 			makeConnectionLog(ip, database.ConnectionTypeSsh, now),
// 			makeConnectionLog(ip, database.ConnectionTypeVscode, now.Add(-time.Second)),
// 			makeConnectionLog(ip, database.ConnectionTypeJetbrains, now.Add(-2*time.Second)),
// 		}

// 		result := mergeWorkspaceConnections(peers, logs)
// 		require.Len(t, result, 3)

// 		// All three should inherit the peer's LOST status.
// 		for i, c := range result {
// 			assert.Equal(t, codersdk.ConnectionStatusControlLost, c.Status, "entry %d", i)
// 			require.NotNil(t, c.IP, "entry %d", i)
// 			assert.Equal(t, ip, *c.IP, "entry %d", i)
// 		}
// 		assert.Equal(t, codersdk.ConnectionType("ssh"), result[0].Type)
// 		assert.Equal(t, codersdk.ConnectionType("vscode"), result[1].Type)
// 		assert.Equal(t, codersdk.ConnectionType("jetbrains"), result[2].Type)
// 	})
// }
