package coderd

import (
	"cmp"
	"context"
	"fmt"
	"net/netip"
	"slices"
	"time"

	"github.com/google/uuid"
	tailcfg "tailscale.com/tailcfg"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
)

const (
	workspaceAgentConnectionsPerAgentLimit int64         = 50
	workspaceAgentConnectionsWindow        time.Duration = 24 * time.Hour
	// Web app connection logs have updated_at bumped on each token refresh
	// (~1/min for HTTP apps). Use 1m30s as the activity window.
	workspaceAppActiveWindow time.Duration = 90 * time.Second
)

var workspaceAgentConnectionsTypes = []database.ConnectionType{
	database.ConnectionTypeSsh,
	database.ConnectionTypeVscode,
	database.ConnectionTypeJetbrains,
	database.ConnectionTypeReconnectingPty,
	database.ConnectionTypeWorkspaceApp,
	database.ConnectionTypePortForwarding,
}

func getOngoingAgentConnectionsLast24h(ctx context.Context, db database.Store, workspaceIDs []uuid.UUID, agentNames []string) ([]database.GetOngoingAgentConnectionsLast24hRow, error) {
	return db.GetOngoingAgentConnectionsLast24h(ctx, database.GetOngoingAgentConnectionsLast24hParams{
		WorkspaceIds:   workspaceIDs,
		AgentNames:     agentNames,
		Types:          workspaceAgentConnectionsTypes,
		Since:          dbtime.Now().Add(-workspaceAgentConnectionsWindow),
		AppActiveSince: dbtime.Now().Add(-workspaceAppActiveWindow),
		PerAgentLimit:  workspaceAgentConnectionsPerAgentLimit,
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
	conn := codersdk.WorkspaceConnection{
		IP:          ip,
		Status:      codersdk.ConnectionStatusOngoing,
		CreatedAt:   connectTime,
		ConnectedAt: &connectTime,
		Type:        codersdk.ConnectionType(log.Type),
	}
	if log.SlugOrPort.Valid {
		conn.Detail = log.SlugOrPort.String
	}
	return conn
}

// mergeWorkspaceConnectionsIntoSessions groups ongoing connections into
// sessions. Connections are grouped by ClientHostname when available
// (so that SSH, Coder Desktop, and IDE connections from the same machine
// become one expandable session), falling back to IP when hostname is
// unknown. Live sessions don't have session_id yet - they're computed
// at query time.
func mergeWorkspaceConnectionsIntoSessions(
	tunnelPeers []*tailnet.TunnelPeerInfo,
	connectionLogs []database.GetOngoingAgentConnectionsLast24hRow,
	derpMap *tailcfg.DERPMap,
	peerTelemetry map[uuid.UUID]*PeerNetworkTelemetry,
) []codersdk.WorkspaceSession {
	if len(tunnelPeers) == 0 && len(connectionLogs) == 0 {
		return nil
	}

	// Build existing flat connections using the current merging logic.
	connections := mergeConnectionsFlat(tunnelPeers, connectionLogs)

	// Group by ClientHostname when available, otherwise by IP.
	// This ensures connections from the same machine (e.g. SSH +
	// Coder Desktop + IDE) collapse into a single session even if
	// they use different tailnet IPs.
	groups := make(map[string][]codersdk.WorkspaceConnection)

	for _, conn := range connections {
		var key string
		if conn.ClientHostname != "" {
			key = "host:" + conn.ClientHostname
		} else if conn.IP != nil {
			key = "ip:" + conn.IP.String()
		}
		groups[key] = append(groups[key], conn)
	}

	// Convert to sessions.
	var sessions []codersdk.WorkspaceSession
	for _, conns := range groups {
		if len(conns) == 0 {
			continue
		}
		sessions = append(sessions, codersdk.WorkspaceSession{
			// No ID for live sessions (ephemeral grouping).
			IP:               conns[0].IP,
			ClientHostname:   conns[0].ClientHostname,
			ShortDescription: conns[0].ShortDescription,
			Status:           deriveSessionStatus(conns),
			StartedAt:        earliestTime(conns),
			Connections:      conns,
		})
	}

	// Sort sessions by hostname first, then IP for stable ordering.
	slices.SortFunc(sessions, func(a, b codersdk.WorkspaceSession) int {
		if c := cmp.Compare(a.ClientHostname, b.ClientHostname); c != 0 {
			return c
		}
		aIP, bIP := "", ""
		if a.IP != nil {
			aIP = a.IP.String()
		}
		if b.IP != nil {
			bIP = b.IP.String()
		}
		return cmp.Compare(aIP, bIP)
	})

	return sessions
}

// mergeConnectionsFlat combines coordinator tunnel peers with connection logs
// into a unified view. Tunnel peers provide real-time network status,
// connection logs provide the application-layer type (ssh, vscode, etc.).
// Entries are correlated by tailnet IP address.
func mergeConnectionsFlat(
	tunnelPeers []*tailnet.TunnelPeerInfo,
	connectionLogs []database.GetOngoingAgentConnectionsLast24hRow,
) []codersdk.WorkspaceConnection {
	// Build IP -> tunnel peer lookup. A single peer has one tailnet IP.
	type peerEntry struct {
		ip     netip.Addr
		peer   *tailnet.TunnelPeerInfo
		status codersdk.WorkspaceConnectionStatus
	}
	type connectionEntry struct {
		connection codersdk.WorkspaceConnection
		peerID     uuid.UUID
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
	var connectionEntries []connectionEntry

	// Connection logs enriched with coordinator status.
	for _, log := range connectionLogs {
		conn := connectionFromLog(log)
		matchedPeerID := uuid.Nil

		// Try to match with a tunnel peer by IP.
		if conn.IP != nil {
			if pe, ok := peersByIP[*conn.IP]; ok {
				conn.Status = pe.status
				matchedPeerIPs[pe.ip] = true
				conn.ClientHostname = pe.peer.Node.Hostname
				conn.ShortDescription = pe.peer.Node.ShortDescription
				matchedPeerID = pe.peer.ID
			}
		}
		connectionEntries = append(connectionEntries, connectionEntry{
			connection: conn,
			peerID:     matchedPeerID,
		})
	}

	// Unmatched tunnel peers (active tunnel, no app-layer log).
	for ip, pe := range peersByIP {
		if matchedPeerIPs[ip] {
			continue
		}
		addr := pe.ip
		connectionEntries = append(connectionEntries, connectionEntry{
			connection: codersdk.WorkspaceConnection{
				IP:               &addr,
				Status:           pe.status,
				CreatedAt:        pe.peer.Start,
				ConnectedAt:      &pe.peer.Start,
				Type:             codersdk.ConnectionTypeSystem,
				ClientHostname:   pe.peer.Node.Hostname,
				ShortDescription: pe.peer.Node.ShortDescription,
			},
			peerID: pe.peer.ID,
		})
	}

	// Sort by IP then newest first for stable display order.
	slices.SortFunc(connectionEntries, func(a, b connectionEntry) int {
		aIP, bIP := "", ""
		if a.connection.IP != nil {
			aIP = a.connection.IP.String()
		}
		if b.connection.IP != nil {
			bIP = b.connection.IP.String()
		}
		return cmp.Or(
			cmp.Compare(aIP, bIP),
			b.connection.CreatedAt.Compare(a.connection.CreatedAt), // Newest first.
		)
	})

	// Apply network telemetry per peer to ongoing connections.
	if len(peerTelemetry) > 0 {
		for i := range connectionEntries {
			conn := &connectionEntries[i].connection
			if conn.Status != codersdk.ConnectionStatusOngoing {
				continue
			}
			if connectionEntries[i].peerID == uuid.Nil {
				continue
			}
			netTelemetry := peerTelemetry[connectionEntries[i].peerID]
			if netTelemetry == nil {
				continue
			}
			if netTelemetry.P2P != nil {
				p2p := *netTelemetry.P2P
				conn.P2P = &p2p
			}
			if netTelemetry.HomeDERP > 0 {
				regionID := netTelemetry.HomeDERP
				name := fmt.Sprintf("Unnamed %d", regionID)
				if derpMap != nil {
					if region, ok := derpMap.Regions[regionID]; ok && region != nil && region.RegionName != "" {
						name = region.RegionName
					}
				}
				conn.HomeDERP = &codersdk.WorkspaceConnectionHomeDERP{
					ID:   regionID,
					Name: name,
				}
			}
			if netTelemetry.P2P != nil && *netTelemetry.P2P && netTelemetry.P2PLatency != nil {
				ms := float64(*netTelemetry.P2PLatency) / float64(time.Millisecond)
				conn.LatencyMS = &ms
			} else if netTelemetry.DERPLatency != nil {
				ms := float64(*netTelemetry.DERPLatency) / float64(time.Millisecond)
				conn.LatencyMS = &ms
			}
		}
	}

	connections := make([]codersdk.WorkspaceConnection, 0, len(connectionEntries))
	for _, entry := range connectionEntries {
		connections = append(connections, entry.connection)
	}
	return connections
}

func deriveSessionStatus(conns []codersdk.WorkspaceConnection) codersdk.WorkspaceConnectionStatus {
	for _, c := range conns {
		if c.Status == codersdk.ConnectionStatusOngoing {
			return codersdk.ConnectionStatusOngoing
		}
	}
	if len(conns) > 0 {
		return conns[0].Status
	}
	return codersdk.ConnectionStatusCleanDisconnected
}

func earliestTime(conns []codersdk.WorkspaceConnection) time.Time {
	if len(conns) == 0 {
		return time.Time{}
	}
	earliest := conns[0].CreatedAt
	for _, c := range conns[1:] {
		if c.CreatedAt.Before(earliest) {
			earliest = c.CreatedAt
		}
	}
	return earliest
}
