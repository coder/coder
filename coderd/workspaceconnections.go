package coderd

import (
	"cmp"
	"context"
	"fmt"
	"net/netip"
	"slices"
	"time"

	"github.com/google/uuid"
	gProto "google.golang.org/protobuf/proto"
	tailcfg "tailscale.com/tailcfg"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
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
	if log.ClientHostname.Valid {
		conn.ClientHostname = log.ClientHostname.String
	}
	if log.ShortDescription.Valid {
		conn.ShortDescription = log.ShortDescription.String
	}
	return conn
}

type peeringRecord struct {
	agentID        uuid.UUID
	controlEvents  []database.TailnetPeeringEvent
	connectionLogs []database.GetOngoingAgentConnectionsLast24hRow
}

// mergeWorkspaceConnectionsIntoSessions groups ongoing connections into
// sessions. Connections are grouped by ClientHostname when available
// (so that SSH, Coder Desktop, and IDE connections from the same machine
// become one expandable session), falling back to IP when hostname is
// unknown. Live sessions don't have session_id yet - they're computed
// at query time.
//
// This function combines three data sources:
//   - tunnelPeers: live coordinator state for real-time network status
//   - peeringEvents: DB-persisted control plane events for historical status
//   - connectionLogs: application-layer connection records (ssh, vscode, etc.)
func mergeWorkspaceConnectionsIntoSessions(
	agentID uuid.UUID,
	tunnelPeers []*tailnet.TunnelPeerInfo,
	peeringEvents []database.TailnetPeeringEvent,
	connectionLogs []database.GetOngoingAgentConnectionsLast24hRow,
	derpMap *tailcfg.DERPMap,
	peerTelemetry map[uuid.UUID]*PeerNetworkTelemetry,
) []codersdk.WorkspaceSession {
	if len(tunnelPeers) == 0 && len(peeringEvents) == 0 && len(connectionLogs) == 0 {
		return nil
	}

	// Build flat connections using peering events and tunnel peers.
	connections := mergeConnectionsFlat(agentID, tunnelPeers, peeringEvents, connectionLogs, derpMap, peerTelemetry)

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

// mergeConnectionsFlat combines coordinator tunnel peers, DB peering events,
// and connection logs into a unified view. Tunnel peers provide real-time
// network status, peering events provide persisted control plane history,
// and connection logs provide the application-layer type (ssh, vscode, etc.).
// Entries are correlated by tailnet IP address.
func mergeConnectionsFlat(
	agentID uuid.UUID,
	tunnelPeers []*tailnet.TunnelPeerInfo,
	peeringEvents []database.TailnetPeeringEvent,
	connectionLogs []database.GetOngoingAgentConnectionsLast24hRow,
	derpMap *tailcfg.DERPMap,
	peerTelemetry map[uuid.UUID]*PeerNetworkTelemetry,
) []codersdk.WorkspaceConnection {
	agentAddr := tailnet.CoderServicePrefix.AddrFromUUID(agentID)

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

	// Build peering records from DB events, keyed by peering ID.
	peeringRecords := make(map[string]*peeringRecord)
	for _, pe := range peeringEvents {
		record, ok := peeringRecords[string(pe.PeeringID)]
		if !ok {
			record = &peeringRecord{
				agentID: agentID,
			}
			peeringRecords[string(pe.PeeringID)] = record
		}
		record.controlEvents = append(record.controlEvents, pe)
	}

	matchedPeerIPs := make(map[netip.Addr]bool)
	var connectionEntries []connectionEntry

	// Connection logs enriched with coordinator status and peering events.
	for _, log := range connectionLogs {
		conn := connectionFromLog(log)
		matchedPeerID := uuid.Nil

		// Try to match with a tunnel peer by IP (live status).
		if conn.IP != nil {
			if pe, ok := peersByIP[*conn.IP]; ok {
				conn.Status = pe.status
				matchedPeerIPs[pe.ip] = true
				conn.ClientHostname = pe.peer.Node.Hostname
				conn.ShortDescription = pe.peer.Node.ShortDescription
				matchedPeerID = pe.peer.ID
			}
		}

		// Try to enrich with peering event data (DB-persisted state).
		if conn.IP != nil {
			clientIP := *conn.IP
			if !clientIP.Is4() {
				peeringID := tailnet.PeeringIDFromAddrs(agentAddr, clientIP)
				if record, ok := peeringRecords[string(peeringID)]; ok {
					enrichConnectionFromPeeringRecord(&conn, record)
					record.connectionLogs = append(record.connectionLogs, log)
				}
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

	// Peering records with no matching connection logs or tunnel peers
	// (control plane events with no app-layer or live tunnel activity).
	for _, record := range peeringRecords {
		if len(record.connectionLogs) > 0 {
			// Already matched above.
			continue
		}
		conn := connectionFromRecord(record)
		// Check if this IP was already covered by a tunnel peer.
		if conn.IP != nil && matchedPeerIPs[*conn.IP] {
			continue
		}
		connectionEntries = append(connectionEntries, connectionEntry{
			connection: conn,
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

// enrichConnectionFromPeeringRecord applies status and metadata from
// persisted peering events to an existing connection record.
func enrichConnectionFromPeeringRecord(conn *codersdk.WorkspaceConnection, record *peeringRecord) {
	slices.SortFunc(record.controlEvents, func(a, b database.TailnetPeeringEvent) int {
		return a.OccurredAt.Compare(b.OccurredAt)
	})
	for _, ce := range record.controlEvents {
		switch ce.EventType {
		case database.TailnetPeeringEventTypePeerUpdateLost:
			conn.Status = codersdk.ConnectionStatusControlLost
		case database.TailnetPeeringEventTypePeerUpdateDisconnected:
			conn.Status = codersdk.ConnectionStatusCleanDisconnected
			conn.EndedAt = &ce.OccurredAt
		case database.TailnetPeeringEventTypeRemovedTunnel:
			conn.Status = codersdk.ConnectionStatusCleanDisconnected
			conn.EndedAt = &ce.OccurredAt
		case database.TailnetPeeringEventTypePeerUpdateNode:
			if ce.SrcPeerID.Valid && ce.SrcPeerID.UUID != record.agentID && ce.Node != nil {
				pNode := new(proto.Node)
				if err := gProto.Unmarshal(ce.Node, pNode); err == nil {
					if conn.ClientHostname == "" {
						conn.ClientHostname = pNode.Hostname
					}
					if conn.ShortDescription == "" {
						conn.ShortDescription = pNode.ShortDescription
					}
				}
			}
		}
	}
}

// connectionFromRecord builds a WorkspaceConnection from a peering record
// that has control events but no matching connection log or tunnel peer.
func connectionFromRecord(record *peeringRecord) codersdk.WorkspaceConnection {
	slices.SortFunc(record.controlEvents, func(a, b database.TailnetPeeringEvent) int {
		return a.OccurredAt.Compare(b.OccurredAt)
	})
	conn := codersdk.WorkspaceConnection{
		Status: codersdk.ConnectionStatusOngoing,
	}
	for _, ce := range record.controlEvents {
		if conn.CreatedAt.IsZero() {
			conn.CreatedAt = ce.OccurredAt
		}
		switch ce.EventType {
		case database.TailnetPeeringEventTypePeerUpdateLost:
			conn.Status = codersdk.ConnectionStatusControlLost
		case database.TailnetPeeringEventTypePeerUpdateDisconnected:
			conn.Status = codersdk.ConnectionStatusCleanDisconnected
			conn.EndedAt = &ce.OccurredAt
		case database.TailnetPeeringEventTypeRemovedTunnel:
			conn.Status = codersdk.ConnectionStatusCleanDisconnected
			conn.EndedAt = &ce.OccurredAt
		case database.TailnetPeeringEventTypeAddedTunnel:
			clientIP := tailnet.CoderServicePrefix.AddrFromUUID(ce.SrcPeerID.UUID)
			conn.IP = &clientIP
		case database.TailnetPeeringEventTypePeerUpdateNode:
			if ce.SrcPeerID.Valid && ce.SrcPeerID.UUID != record.agentID && ce.Node != nil {
				pNode := new(proto.Node)
				if err := gProto.Unmarshal(ce.Node, pNode); err == nil {
					conn.ClientHostname = pNode.Hostname
					conn.ShortDescription = pNode.ShortDescription
				}
			}
		}
	}
	return conn
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
