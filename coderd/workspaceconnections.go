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
	if log.Os.Valid {
		conn.OS = log.Os.String
	}
	return conn
}

type peeringRecord struct {
	agentID        uuid.UUID
	controlEvents  []database.TailnetPeeringEvent
	connectionLogs []database.GetOngoingAgentConnectionsLast24hRow
	peerTelemetry  *PeerNetworkTelemetry
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
	peeringEvents []database.TailnetPeeringEvent,
	connectionLogs []database.GetOngoingAgentConnectionsLast24hRow,
	derpMap *tailcfg.DERPMap,
	peerTelemetry map[uuid.UUID]*PeerNetworkTelemetry,
) []codersdk.WorkspaceSession {
	if len(peeringEvents) == 0 && len(connectionLogs) == 0 {
		return nil
	}

	// Build flat connections using peering events and tunnel peers.
	connections := mergeConnectionsFlat(agentID, peeringEvents, connectionLogs, derpMap, peerTelemetry)

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
	peeringEvents []database.TailnetPeeringEvent,
	connectionLogs []database.GetOngoingAgentConnectionsLast24hRow,
	derpMap *tailcfg.DERPMap,
	peerTelemetry map[uuid.UUID]*PeerNetworkTelemetry,
) []codersdk.WorkspaceConnection {
	agentAddr := tailnet.CoderServicePrefix.AddrFromUUID(agentID)

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

	var connections []codersdk.WorkspaceConnection

	for _, log := range connectionLogs {
		if !log.Ip.Valid {
			connections = append(connections, connectionFromLog(log))
			continue
		}
		clientIP, ok := netip.AddrFromSlice(log.Ip.IPNet.IP)
		if !ok || clientIP.Is4() {
			connections = append(connections, connectionFromLog(log))
			continue
		}
		peeringID := tailnet.PeeringIDFromAddrs(agentAddr, clientIP)
		record, ok := peeringRecords[string(peeringID)]
		if !ok {
			record = &peeringRecord{
				agentID: agentID,
			}
			peeringRecords[string(peeringID)] = record
		}
		record.connectionLogs = append(record.connectionLogs, log)
	}

	// Apply network telemetry per peer to ongoing connections.
	for clientID, peerTelemetry := range peerTelemetry {
		peeringID := tailnet.PeeringIDFromUUIDs(agentID, clientID)
		record, ok := peeringRecords[string(peeringID)]
		if !ok {
			continue
		}
		record.peerTelemetry = peerTelemetry
	}

	for _, record := range peeringRecords {
		connections = append(connections, connectionFromRecord(record, derpMap))
	}

	// Sort by creation time
	slices.SortFunc(connections, func(a, b codersdk.WorkspaceConnection) int {
		return b.CreatedAt.Compare(a.CreatedAt) // Newest first.
	})

	return connections
}

func connectionFromRecord(record *peeringRecord, derpMap *tailcfg.DERPMap) codersdk.WorkspaceConnection {
	slices.SortFunc(record.controlEvents, func(a, b database.TailnetPeeringEvent) int {
		return a.OccurredAt.Compare(b.OccurredAt)
	})
	slices.SortFunc(record.connectionLogs, func(a, b database.GetOngoingAgentConnectionsLast24hRow) int {
		return a.ConnectTime.Compare(b.ConnectTime)
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
					conn.OS = pNode.Os
				}
			}
		}
	}
	for _, log := range record.connectionLogs {
		if conn.CreatedAt.IsZero() {
			conn.CreatedAt = log.ConnectTime
		}
		if log.Ip.Valid {
			if addr, ok := netip.AddrFromSlice(log.Ip.IPNet.IP); ok {
				addr = addr.Unmap()
				conn.IP = &addr
			}
		}
		if log.SlugOrPort.Valid {
			conn.Detail = log.SlugOrPort.String
		}
		if log.Type.Valid() {
			conn.Type = codersdk.ConnectionType(log.Type)
		}
		if conn.Status != codersdk.ConnectionStatusControlLost &&
			conn.Status != codersdk.ConnectionStatusCleanDisconnected && log.DisconnectTime.Valid {
			conn.Status = codersdk.ConnectionStatusClientDisconnected
		}
		if conn.EndedAt == nil && log.DisconnectTime.Valid {
			conn.EndedAt = &log.DisconnectTime.Time
		}
	}
	if record.peerTelemetry == nil {
		return conn
	}
	if record.peerTelemetry.P2P != nil {
		p2p := *record.peerTelemetry.P2P
		conn.P2P = &p2p
	}
	if record.peerTelemetry.HomeDERP > 0 {
		regionID := record.peerTelemetry.HomeDERP
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
	if record.peerTelemetry.P2P != nil && *record.peerTelemetry.P2P && record.peerTelemetry.P2PLatency != nil {
		ms := float64(*record.peerTelemetry.P2PLatency) / float64(time.Millisecond)
		conn.LatencyMS = &ms
	} else if record.peerTelemetry.DERPLatency != nil {
		ms := float64(*record.peerTelemetry.DERPLatency) / float64(time.Millisecond)
		conn.LatencyMS = &ms
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
