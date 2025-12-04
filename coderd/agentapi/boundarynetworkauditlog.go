package agentapi

import (
	"time"
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/emptypb"

	"cdr.dev/slog"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
)

type BoundaryNetworkAuditLogAPI struct {
	AgentFn  func(context.Context) (database.WorkspaceAgent, error)
	Database database.Store
	Log      slog.Logger
}

func (a *BoundaryNetworkAuditLogAPI) ReportBoundaryNetworkAuditLogs(ctx context.Context, req *agentproto.ReportBoundaryNetworkAuditLogsRequest) (*emptypb.Empty, error) {
	logs := req.GetLogs()
	if len(logs) == 0 {
		return &emptypb.Empty{}, nil
	}

	// Fetch contextual data for these logs.
	workspaceAgent, err := a.AgentFn(ctx)
	if err != nil {
		return nil, xerrors.Errorf("get agent: %w", err)
	}
	workspace, err := a.Database.GetWorkspaceByAgentID(ctx, workspaceAgent.ID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace by agent id: %w", err)
	}

	// Build the bulk insert parameters.
	ids := make([]uuid.UUID, len(logs))
	times := make([]time.Time, len(logs))
	organizationIDs := make([]uuid.UUID, len(logs))
	workspaceIDs := make([]uuid.UUID, len(logs))
	workspaceOwnerIDs := make([]uuid.UUID, len(logs))
	workspaceNames := make([]string, len(logs))
	agentIDs := make([]uuid.UUID, len(logs))
	agentNames := make([]string, len(logs))
	domains := make([]string, len(logs))
	actions := make([]database.BoundaryNetworkAction, len(logs))

	for i, log := range logs {
		ids[i] = uuid.New()
		times[i] = log.GetTimestamp().AsTime()
		organizationIDs[i] = workspace.OrganizationID
		workspaceIDs[i] = workspace.ID
		workspaceOwnerIDs[i] = workspace.OwnerID
		workspaceNames[i] = workspace.Name
		agentIDs[i] = workspaceAgent.ID
		agentNames[i] = workspaceAgent.Name
		domains[i] = log.GetDomain()
		if log.GetAllowed() {
			actions[i] = database.BoundaryNetworkActionAllow
		} else {
			actions[i] = database.BoundaryNetworkActionDeny
		}
	}

	err = a.Database.InsertBoundaryNetworkAuditLogs(ctx, database.InsertBoundaryNetworkAuditLogsParams{
		ID:               ids,
		Time:             times,
		OrganizationID:   organizationIDs,
		WorkspaceID:      workspaceIDs,
		WorkspaceOwnerID: workspaceOwnerIDs,
		WorkspaceName:    workspaceNames,
		AgentID:          agentIDs,
		AgentName:        agentNames,
		Domain:           domains,
		Action:           actions,
	})
	if err != nil {
		return nil, xerrors.Errorf("insert boundary network audit logs: %w", err)
	}

	a.Log.Debug(ctx, "reported boundary network audit logs",
		slog.F("count", len(logs)),
		slog.F("workspace_id", workspace.ID),
		slog.F("agent_id", workspaceAgent.ID),
	)

	return &emptypb.Empty{}, nil
}
