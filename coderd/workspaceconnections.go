package coderd

import (
	"context"
	"net/netip"
	"slices"
	"time"

	"github.com/google/uuid"
	gProto "google.golang.org/protobuf/proto"

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
	return conn
}

type peeringRecord struct {
	agentID        uuid.UUID
	controlEvents  []database.TailnetPeeringEvent
	connectionLogs []database.GetOngoingAgentConnectionsLast24hRow
}

// mergeWorkspaceConnections combines coordinator peering events with connection
// logs into a unified view. Peering events provide control plane events,
// connection logs provide the application-layer type (ssh, vscode, etc.).
// Entries are correlated by tailnet IP address.
func mergeWorkspaceConnections(
	agentID uuid.UUID,
	peeringEvents []database.TailnetPeeringEvent,
	connectionLogs []database.GetOngoingAgentConnectionsLast24hRow,
) []codersdk.WorkspaceConnection {
	if len(peeringEvents) == 0 && len(connectionLogs) == 0 {
		return nil
	}
	agentAddr := tailnet.CoderServicePrefix.AddrFromUUID(agentID)

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

	for _, record := range peeringRecords {
		connections = append(connections, connectionFromRecord(record))
	}

	// Sort by creation time
	slices.SortFunc(connections, func(a, b codersdk.WorkspaceConnection) int {
		return b.CreatedAt.Compare(a.CreatedAt) // Newest first.
	})

	return connections
}

func connectionFromRecord(record *peeringRecord) codersdk.WorkspaceConnection {
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

	return conn
}
