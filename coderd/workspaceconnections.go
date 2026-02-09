package coderd

import (
	"context"
	"net/netip"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
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

func workspaceConnectionsFromLogs(logs []database.GetOngoingAgentConnectionsLast24hRow) []codersdk.WorkspaceConnection {
	connections := make([]codersdk.WorkspaceConnection, 0, len(logs))
	for _, log := range logs {
		connectTime := log.ConnectTime

		var ip *netip.Addr
		if log.Ip.Valid {
			if addr, ok := netip.AddrFromSlice(log.Ip.IPNet.IP); ok {
				addr = addr.Unmap()
				ip = &addr
			}
		}

		connections = append(connections, codersdk.WorkspaceConnection{
			IP:          ip,
			Status:      codersdk.ConnectionStatusOngoing,
			CreatedAt:   connectTime,
			ConnectedAt: &connectTime,
			EndedAt:     nil,
			Type:        codersdk.ConnectionType(log.Type),
		})
	}
	return connections
}
