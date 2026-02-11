package coderd

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tailcfg "tailscale.com/tailcfg"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
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

func boolPtr(v bool) *bool {
	return &v
}

func durationPtr(v time.Duration) *time.Duration {
	return &v
}

func makeNetTelemetry(p2p *bool, derpLatency, p2pLatency *time.Duration, homeDERP int) *PeerNetworkTelemetry {
	return &PeerNetworkTelemetry{
		P2P:           p2p,
		DERPLatency:   derpLatency,
		P2PLatency:    p2pLatency,
		HomeDERP:      homeDERP,
		LastUpdatedAt: time.Now(),
	}
}

func TestMergeWorkspaceConnections(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Millisecond)
	testDERPMap := &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			1: {
				RegionID:   1,
				RegionName: "New York City",
			},
		},
	}

	t.Run("BothEmpty", func(t *testing.T) {
		t.Parallel()
		result := mergeWorkspaceConnections(nil, nil, nil, nil)
		assert.Nil(t, result)
	})

	t.Run("LogsOnly", func(t *testing.T) {
		t.Parallel()

		ip1 := testTailnetIP(1)
		ip2 := testTailnetIP(2)
		logs := []database.GetOngoingAgentConnectionsLast24hRow{
			makeConnectionLog(ip1, database.ConnectionTypeSsh, now),
			makeConnectionLog(ip2, database.ConnectionTypeVscode, now.Add(-time.Minute)),
		}

		result := mergeWorkspaceConnections(nil, logs, nil, nil)
		require.Len(t, result, 2)

		assert.Equal(t, codersdk.ConnectionType("ssh"), result[0].Type)
		assert.Equal(t, codersdk.ConnectionStatusOngoing, result[0].Status)
		require.NotNil(t, result[0].IP)
		assert.Equal(t, ip1, *result[0].IP)

		assert.Equal(t, codersdk.ConnectionType("vscode"), result[1].Type)
		assert.Equal(t, codersdk.ConnectionStatusOngoing, result[1].Status)
		require.NotNil(t, result[1].IP)
		assert.Equal(t, ip2, *result[1].IP)
	})

	t.Run("PeersOnly", func(t *testing.T) {
		t.Parallel()

		ip1 := testTailnetIP(1)
		ip2 := testTailnetIP(2)
		peers := []*tailnet.TunnelPeerInfo{
			makeTunnelPeer(ip1, tailnetproto.CoordinateResponse_PeerUpdate_NODE, now),
			makeTunnelPeer(ip2, tailnetproto.CoordinateResponse_PeerUpdate_LOST, now.Add(-time.Minute)),
		}

		result := mergeWorkspaceConnections(peers, nil, nil, nil)
		require.Len(t, result, 2)

		// Map iteration order is nondeterministic, find each by IP.
		byIP := make(map[netip.Addr]codersdk.WorkspaceConnection, len(result))
		for _, c := range result {
			require.NotNil(t, c.IP)
			byIP[*c.IP] = c
		}

		c1 := byIP[ip1]
		assert.Equal(t, codersdk.ConnectionStatusOngoing, c1.Status)
		assert.Equal(t, codersdk.ConnectionTypeSystem, c1.Type)
		assert.Equal(t, now, c1.CreatedAt)

		c2 := byIP[ip2]
		assert.Equal(t, codersdk.ConnectionStatusControlLost, c2.Status)
		assert.Equal(t, codersdk.ConnectionTypeSystem, c2.Type)
	})

	t.Run("MatchedByIP", func(t *testing.T) {
		t.Parallel()

		ip := testTailnetIP(1)
		peers := []*tailnet.TunnelPeerInfo{
			makeTunnelPeer(ip, tailnetproto.CoordinateResponse_PeerUpdate_NODE, now),
		}
		logs := []database.GetOngoingAgentConnectionsLast24hRow{
			makeConnectionLog(ip, database.ConnectionTypeSsh, now.Add(-time.Second)),
		}

		result := mergeWorkspaceConnections(peers, logs, nil, nil)
		require.Len(t, result, 1)

		assert.Equal(t, codersdk.ConnectionType("ssh"), result[0].Type)
		assert.Equal(t, codersdk.ConnectionStatusOngoing, result[0].Status)
		require.NotNil(t, result[0].IP)
		assert.Equal(t, ip, *result[0].IP)
	})

	t.Run("MatchedLostPeer", func(t *testing.T) {
		t.Parallel()

		ip := testTailnetIP(1)
		peers := []*tailnet.TunnelPeerInfo{
			makeTunnelPeer(ip, tailnetproto.CoordinateResponse_PeerUpdate_LOST, now),
		}
		logs := []database.GetOngoingAgentConnectionsLast24hRow{
			makeConnectionLog(ip, database.ConnectionTypeSsh, now),
		}

		result := mergeWorkspaceConnections(peers, logs, nil, nil)
		require.Len(t, result, 1)

		assert.Equal(t, codersdk.ConnectionType("ssh"), result[0].Type)
		assert.Equal(t, codersdk.ConnectionStatusControlLost, result[0].Status)
	})

	t.Run("PartialMatch", func(t *testing.T) {
		t.Parallel()

		ip1 := testTailnetIP(1)
		ip2 := testTailnetIP(2)
		peers := []*tailnet.TunnelPeerInfo{
			makeTunnelPeer(ip1, tailnetproto.CoordinateResponse_PeerUpdate_NODE, now),
			makeTunnelPeer(ip2, tailnetproto.CoordinateResponse_PeerUpdate_LOST, now.Add(-time.Minute)),
		}
		logs := []database.GetOngoingAgentConnectionsLast24hRow{
			makeConnectionLog(ip1, database.ConnectionTypeVscode, now.Add(-time.Second)),
		}

		result := mergeWorkspaceConnections(peers, logs, nil, nil)
		require.Len(t, result, 2)

		// First entry is the matched log (preserves log order).
		assert.Equal(t, codersdk.ConnectionType("vscode"), result[0].Type)
		assert.Equal(t, codersdk.ConnectionStatusOngoing, result[0].Status)
		require.NotNil(t, result[0].IP)
		assert.Equal(t, ip1, *result[0].IP)

		// Second entry is the unmatched peer.
		assert.Equal(t, codersdk.ConnectionTypeSystem, result[1].Type)
		assert.Equal(t, codersdk.ConnectionStatusControlLost, result[1].Status)
		require.NotNil(t, result[1].IP)
		assert.Equal(t, ip2, *result[1].IP)
	})

	t.Run("MultiSessionSameIP", func(t *testing.T) {
		t.Parallel()

		ip := testTailnetIP(1)
		peers := []*tailnet.TunnelPeerInfo{
			makeTunnelPeer(ip, tailnetproto.CoordinateResponse_PeerUpdate_LOST, now),
		}
		logs := []database.GetOngoingAgentConnectionsLast24hRow{
			makeConnectionLog(ip, database.ConnectionTypeSsh, now),
			makeConnectionLog(ip, database.ConnectionTypeVscode, now.Add(-time.Second)),
			makeConnectionLog(ip, database.ConnectionTypeJetbrains, now.Add(-2*time.Second)),
		}

		result := mergeWorkspaceConnections(peers, logs, nil, nil)
		require.Len(t, result, 3)

		// All three should inherit the peer's LOST status.
		for i, c := range result {
			assert.Equal(t, codersdk.ConnectionStatusControlLost, c.Status, "entry %d", i)
			require.NotNil(t, c.IP, "entry %d", i)
			assert.Equal(t, ip, *c.IP, "entry %d", i)
		}
		assert.Equal(t, codersdk.ConnectionType("ssh"), result[0].Type)
		assert.Equal(t, codersdk.ConnectionType("vscode"), result[1].Type)
		assert.Equal(t, codersdk.ConnectionType("jetbrains"), result[2].Type)
	})

	t.Run("NilTelemetry", func(t *testing.T) {
		t.Parallel()

		ip := testTailnetIP(1)
		peers := []*tailnet.TunnelPeerInfo{
			makeTunnelPeer(ip, tailnetproto.CoordinateResponse_PeerUpdate_NODE, now),
		}
		logs := []database.GetOngoingAgentConnectionsLast24hRow{
			makeConnectionLog(ip, database.ConnectionTypeSsh, now.Add(-time.Second)),
		}

		result := mergeWorkspaceConnections(peers, logs, nil, nil)
		require.Len(t, result, 1)
		assert.Equal(t, codersdk.ConnectionStatusOngoing, result[0].Status)
		assert.Nil(t, result[0].P2P)
		assert.Nil(t, result[0].LatencyMS)
		assert.Nil(t, result[0].HomeDERP)
	})

	t.Run("TelemetryAppliedToOngoing", func(t *testing.T) {
		t.Parallel()

		ip := testTailnetIP(1)
		peers := []*tailnet.TunnelPeerInfo{
			makeTunnelPeer(ip, tailnetproto.CoordinateResponse_PeerUpdate_NODE, now),
		}
		logs := []database.GetOngoingAgentConnectionsLast24hRow{
			makeConnectionLog(ip, database.ConnectionTypeSsh, now),
		}
		telemetry := makeNetTelemetry(
			boolPtr(true),
			nil,
			durationPtr(10*time.Millisecond),
			1,
		)

		peerTelemetry := map[uuid.UUID]*PeerNetworkTelemetry{
			peers[0].ID: telemetry,
		}
		result := mergeWorkspaceConnections(peers, logs, testDERPMap, peerTelemetry)
		require.Len(t, result, 1)
		require.NotNil(t, result[0].P2P)
		assert.True(t, *result[0].P2P)
		require.NotNil(t, result[0].LatencyMS)
		assert.InDelta(t, 10.0, *result[0].LatencyMS, 0.001)
		require.NotNil(t, result[0].HomeDERP)
		assert.Equal(t, 1, result[0].HomeDERP.ID)
		assert.Equal(t, "New York City", result[0].HomeDERP.Name)
	})

	t.Run("HomeDERPNameResolvedFromDERPMap", func(t *testing.T) {
		t.Parallel()

		ip := testTailnetIP(1)
		peers := []*tailnet.TunnelPeerInfo{
			makeTunnelPeer(ip, tailnetproto.CoordinateResponse_PeerUpdate_NODE, now),
		}
		logs := []database.GetOngoingAgentConnectionsLast24hRow{
			makeConnectionLog(ip, database.ConnectionTypeSsh, now),
		}
		telemetry := makeNetTelemetry(nil, nil, nil, 1)

		peerTelemetry := map[uuid.UUID]*PeerNetworkTelemetry{
			peers[0].ID: telemetry,
		}
		result := mergeWorkspaceConnections(peers, logs, testDERPMap, peerTelemetry)
		require.Len(t, result, 1)
		require.NotNil(t, result[0].HomeDERP)
		assert.Equal(t, 1, result[0].HomeDERP.ID)
		assert.Equal(t, "New York City", result[0].HomeDERP.Name)
	})

	t.Run("HomeDERPUnknownRegionFallback", func(t *testing.T) {
		t.Parallel()

		ip := testTailnetIP(1)
		peers := []*tailnet.TunnelPeerInfo{
			makeTunnelPeer(ip, tailnetproto.CoordinateResponse_PeerUpdate_NODE, now),
		}
		logs := []database.GetOngoingAgentConnectionsLast24hRow{
			makeConnectionLog(ip, database.ConnectionTypeSsh, now),
		}
		telemetry := makeNetTelemetry(nil, nil, nil, 99)

		peerTelemetry := map[uuid.UUID]*PeerNetworkTelemetry{
			peers[0].ID: telemetry,
		}
		result := mergeWorkspaceConnections(peers, logs, testDERPMap, peerTelemetry)
		require.Len(t, result, 1)
		require.NotNil(t, result[0].HomeDERP)
		assert.Equal(t, 99, result[0].HomeDERP.ID)
		assert.Equal(t, "Unnamed 99", result[0].HomeDERP.Name)
	})

	t.Run("HomeDERPZeroIsOmitted", func(t *testing.T) {
		t.Parallel()

		ip := testTailnetIP(1)
		peers := []*tailnet.TunnelPeerInfo{
			makeTunnelPeer(ip, tailnetproto.CoordinateResponse_PeerUpdate_NODE, now),
		}
		logs := []database.GetOngoingAgentConnectionsLast24hRow{
			makeConnectionLog(ip, database.ConnectionTypeSsh, now),
		}
		telemetry := makeNetTelemetry(nil, nil, nil, 0)

		peerTelemetry := map[uuid.UUID]*PeerNetworkTelemetry{
			peers[0].ID: telemetry,
		}
		result := mergeWorkspaceConnections(peers, logs, testDERPMap, peerTelemetry)
		require.Len(t, result, 1)
		assert.Nil(t, result[0].HomeDERP)
	})

	t.Run("TelemetrySkipsNonOngoing", func(t *testing.T) {
		t.Parallel()

		ip := testTailnetIP(1)
		peers := []*tailnet.TunnelPeerInfo{
			makeTunnelPeer(ip, tailnetproto.CoordinateResponse_PeerUpdate_LOST, now),
		}
		logs := []database.GetOngoingAgentConnectionsLast24hRow{
			makeConnectionLog(ip, database.ConnectionTypeSsh, now),
		}
		telemetry := makeNetTelemetry(
			boolPtr(true),
			durationPtr(50*time.Millisecond),
			durationPtr(10*time.Millisecond),
			1,
		)

		peerTelemetry := map[uuid.UUID]*PeerNetworkTelemetry{
			peers[0].ID: telemetry,
		}
		result := mergeWorkspaceConnections(peers, logs, testDERPMap, peerTelemetry)
		require.Len(t, result, 1)
		assert.Equal(t, codersdk.ConnectionStatusControlLost, result[0].Status)
		assert.Nil(t, result[0].P2P)
		assert.Nil(t, result[0].LatencyMS)
		assert.Nil(t, result[0].HomeDERP)
	})
	t.Run("P2PLatencyUsedWhenP2P", func(t *testing.T) {
		t.Parallel()

		ip := testTailnetIP(1)
		peers := []*tailnet.TunnelPeerInfo{
			makeTunnelPeer(ip, tailnetproto.CoordinateResponse_PeerUpdate_NODE, now),
		}
		logs := []database.GetOngoingAgentConnectionsLast24hRow{
			makeConnectionLog(ip, database.ConnectionTypeSsh, now),
		}
		telemetry := makeNetTelemetry(
			boolPtr(true),
			durationPtr(50*time.Millisecond),
			durationPtr(15*time.Millisecond),
			1,
		)

		peerTelemetry := map[uuid.UUID]*PeerNetworkTelemetry{
			peers[0].ID: telemetry,
		}
		result := mergeWorkspaceConnections(peers, logs, testDERPMap, peerTelemetry)
		require.Len(t, result, 1)
		require.NotNil(t, result[0].LatencyMS)
		assert.InDelta(t, 15.0, *result[0].LatencyMS, 0.001)
	})

	t.Run("DERPLatencyUsedWhenNotP2P", func(t *testing.T) {
		t.Parallel()

		ip := testTailnetIP(1)
		peers := []*tailnet.TunnelPeerInfo{
			makeTunnelPeer(ip, tailnetproto.CoordinateResponse_PeerUpdate_NODE, now),
		}
		logs := []database.GetOngoingAgentConnectionsLast24hRow{
			makeConnectionLog(ip, database.ConnectionTypeSsh, now),
		}
		telemetry := makeNetTelemetry(
			boolPtr(false),
			durationPtr(50*time.Millisecond),
			nil,
			1,
		)

		peerTelemetry := map[uuid.UUID]*PeerNetworkTelemetry{
			peers[0].ID: telemetry,
		}
		result := mergeWorkspaceConnections(peers, logs, testDERPMap, peerTelemetry)
		require.Len(t, result, 1)
		require.NotNil(t, result[0].LatencyMS)
		assert.InDelta(t, 50.0, *result[0].LatencyMS, 0.001)
	})

	t.Run("BothLatenciesNil", func(t *testing.T) {
		t.Parallel()

		ip := testTailnetIP(1)
		peers := []*tailnet.TunnelPeerInfo{
			makeTunnelPeer(ip, tailnetproto.CoordinateResponse_PeerUpdate_NODE, now),
		}
		logs := []database.GetOngoingAgentConnectionsLast24hRow{
			makeConnectionLog(ip, database.ConnectionTypeSsh, now),
		}
		telemetry := makeNetTelemetry(
			boolPtr(true),
			nil,
			nil,
			1,
		)

		peerTelemetry := map[uuid.UUID]*PeerNetworkTelemetry{
			peers[0].ID: telemetry,
		}
		result := mergeWorkspaceConnections(peers, logs, testDERPMap, peerTelemetry)
		require.Len(t, result, 1)
		assert.Nil(t, result[0].LatencyMS)
	})

	t.Run("TelemetryPerPeerAttribution", func(t *testing.T) {
		t.Parallel()

		ip1 := testTailnetIP(1)
		ip2 := testTailnetIP(2)
		peer1 := makeTunnelPeer(ip1, tailnetproto.CoordinateResponse_PeerUpdate_NODE, now)
		peer2 := makeTunnelPeer(ip2, tailnetproto.CoordinateResponse_PeerUpdate_NODE, now)
		peers := []*tailnet.TunnelPeerInfo{peer1, peer2}
		logs := []database.GetOngoingAgentConnectionsLast24hRow{
			makeConnectionLog(ip1, database.ConnectionTypeSsh, now),
			makeConnectionLog(ip2, database.ConnectionTypeVscode, now),
		}
		peerTelemetry := map[uuid.UUID]*PeerNetworkTelemetry{
			peer1.ID: makeNetTelemetry(
				boolPtr(true),
				durationPtr(80*time.Millisecond),
				durationPtr(12*time.Millisecond),
				1,
			),
			peer2.ID: makeNetTelemetry(
				boolPtr(false),
				durationPtr(45*time.Millisecond),
				durationPtr(5*time.Millisecond),
				99,
			),
		}

		result := mergeWorkspaceConnections(peers, logs, testDERPMap, peerTelemetry)
		require.Len(t, result, 2)

		byIP := make(map[netip.Addr]codersdk.WorkspaceConnection, len(result))
		for _, c := range result {
			require.NotNil(t, c.IP)
			byIP[*c.IP] = c
		}

		conn1 := byIP[ip1]
		require.NotNil(t, conn1.P2P)
		assert.True(t, *conn1.P2P)
		require.NotNil(t, conn1.LatencyMS)
		assert.InDelta(t, 12.0, *conn1.LatencyMS, 0.001)
		require.NotNil(t, conn1.HomeDERP)
		assert.Equal(t, 1, conn1.HomeDERP.ID)
		assert.Equal(t, "New York City", conn1.HomeDERP.Name)

		conn2 := byIP[ip2]
		require.NotNil(t, conn2.P2P)
		assert.False(t, *conn2.P2P)
		require.NotNil(t, conn2.LatencyMS)
		assert.InDelta(t, 45.0, *conn2.LatencyMS, 0.001)
		require.NotNil(t, conn2.HomeDERP)
		assert.Equal(t, 99, conn2.HomeDERP.ID)
		assert.Equal(t, "Unnamed 99", conn2.HomeDERP.Name)
	})

	t.Run("TelemetryNotAppliedToUnmatchedLogOnly", func(t *testing.T) {
		t.Parallel()

		ip := testTailnetIP(1)
		logs := []database.GetOngoingAgentConnectionsLast24hRow{
			makeConnectionLog(ip, database.ConnectionTypeSsh, now),
		}
		peerTelemetry := map[uuid.UUID]*PeerNetworkTelemetry{
			uuid.New(): makeNetTelemetry(
				boolPtr(true),
				durationPtr(40*time.Millisecond),
				durationPtr(10*time.Millisecond),
				1,
			),
		}

		result := mergeWorkspaceConnections(nil, logs, testDERPMap, peerTelemetry)
		require.Len(t, result, 1)
		assert.Equal(t, codersdk.ConnectionStatusOngoing, result[0].Status)
		assert.Nil(t, result[0].P2P)
		assert.Nil(t, result[0].LatencyMS)
		assert.Nil(t, result[0].HomeDERP)
	})
}
