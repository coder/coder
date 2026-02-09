package coderd

import (
	"context"
	"net/netip"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
)

const (
	workspaceAgentConnectionsPerAgentLimit int64         = 50
	workspaceAgentConnectionsWindow        time.Duration = 24 * time.Hour
)

var workspaceAgentConnectionsTypes = []database.ConnectionType{
	database.ConnectionTypeSsh,
	database.ConnectionTypeVscode,
	database.ConnectionTypeJetbrains,
	database.ConnectionTypeReconnectingPty,
}

func getOngoingAgentConnectionsLast24h(ctx context.Context, db database.Store, workspaceIDs []uuid.UUID, agentNames []string) ([]database.GetOngoingAgentConnectionsLast24hRow, error) {
	return db.GetOngoingAgentConnectionsLast24h(ctx, database.GetOngoingAgentConnectionsLast24hParams{
		WorkspaceIds:  workspaceIDs,
		AgentNames:    agentNames,
		Types:         workspaceAgentConnectionsTypes,
		Since:         dbtime.Now().Add(-workspaceAgentConnectionsWindow),
		PerAgentLimit: workspaceAgentConnectionsPerAgentLimit,
	})
}

func groupConnectionLogsByWorkspaceAndAgent(logs []database.GetOngoingAgentConnectionsLast24hRow) map[uuid.UUID]map[string][]database.GetOngoingAgentConnectionsLast24hRow {
	byWorkspaceAndAgent := make(map[uuid.UUID]map[string][]database.GetOngoingAgentConnectionsLast24hRow)
	for _, l := range logs {
		byAgent, ok := byWorkspaceAndAgent[l.WorkspaceID]
		if !ok {
			byAgent = make(map[string][]database.GetOngoingAgentConnectionsLast24hRow)
			byWorkspaceAndAgent[l.WorkspaceID] = byAgent
		}
		byAgent[l.AgentName] = append(byAgent[l.AgentName], l)
	}
	return byWorkspaceAndAgent
}

func connectionFromLog(log database.GetOngoingAgentConnectionsLast24hRow) codersdk.WorkspaceConnection {
	connectTime := log.ConnectTime
	var ip *netip.Addr
	if log.Ip.Valid {
		if addr, ok := netip.AddrFromSlice(log.Ip.IPNet.IP); ok {
			addr = addr.Unmap()
			ip = &addr
		}
	}
	return codersdk.WorkspaceConnection{
		IP:          ip,
		Status:      codersdk.ConnectionStatusOngoing,
		CreatedAt:   connectTime,
		ConnectedAt: &connectTime,
		Type:        codersdk.ConnectionType(log.Type),
	}
}

func workspaceConnectionsFromLogs(logs []database.GetOngoingAgentConnectionsLast24hRow) []codersdk.WorkspaceConnection {
	connections := make([]codersdk.WorkspaceConnection, 0, len(logs))
	for _, log := range logs {
		connections = append(connections, connectionFromLog(log))
	}
	return connections
}

// mergeWorkspaceConnections combines coordinator tunnel peers with connection
// logs into a unified view. Tunnel peers provide real-time network status,
// connection logs provide the application-layer type (ssh, vscode, etc.).
// Entries are correlated by tailnet IP address.
func mergeWorkspaceConnections(
	tunnelPeers []*tailnet.TunnelPeerInfo,
	connectionLogs []database.GetOngoingAgentConnectionsLast24hRow,
) []codersdk.WorkspaceConnection {
	if len(tunnelPeers) == 0 && len(connectionLogs) == 0 {
		return nil
	}

	// Build IP -> tunnel peer lookup. A single peer has one tailnet IP.
	type peerEntry struct {
		ip     netip.Addr
		peer   *tailnet.TunnelPeerInfo
		status codersdk.WorkspaceConnectionStatus
	}
	peersByIP := make(map[netip.Addr]*peerEntry, len(tunnelPeers))
	for _, tp := range tunnelPeers {
		if tp.Node == nil || len(tp.Node.Addresses) == 0 {
			continue
		}
		prefix, err := netip.ParsePrefix(tp.Node.Addresses[0])
		if err != nil {
			continue
		}
		addr := prefix.Addr()
		status := codersdk.ConnectionStatusOngoing
		if tp.Status == tailnetproto.CoordinateResponse_PeerUpdate_LOST {
			status = codersdk.ConnectionStatusControlLost
		}
		peersByIP[addr] = &peerEntry{ip: addr, peer: tp, status: status}
	}

	matchedPeerIPs := make(map[netip.Addr]bool)
	var connections []codersdk.WorkspaceConnection

	// Connection logs enriched with coordinator status.
	for _, log := range connectionLogs {
		conn := connectionFromLog(log)

		// Try to match with a tunnel peer by IP.
		if conn.IP != nil {
			if pe, ok := peersByIP[*conn.IP]; ok {
				conn.Status = pe.status
				matchedPeerIPs[pe.ip] = true
			}
		}
		connections = append(connections, conn)
	}

	// Unmatched tunnel peers (active tunnel, no app-layer log).
	for ip, pe := range peersByIP {
		if matchedPeerIPs[ip] {
			continue
		}
		addr := pe.ip
		connections = append(connections, codersdk.WorkspaceConnection{
			IP:          &addr,
			Status:      pe.status,
			CreatedAt:   pe.peer.Start,
			ConnectedAt: &pe.peer.Start,
		})
	}

	return connections
}
