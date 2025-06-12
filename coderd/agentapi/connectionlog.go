package agentapi

import (
	"context"
	"database/sql"
	"sync/atomic"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/emptypb"

	"cdr.dev/slog"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/connectionlog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

type ConnLogAPI struct {
	AgentFn          func(context.Context) (database.WorkspaceAgent, error)
	ConnectionLogger *atomic.Pointer[connectionlog.ConnectionLogger]
	Database         database.Store
	Log              slog.Logger
}

func (a *ConnLogAPI) ReportConnection(ctx context.Context, req *agentproto.ReportConnectionRequest) (*emptypb.Empty, error) {
	// We will use connection ID as request ID, typically this is the
	// SSH session ID as reported by the agent.
	connectionID, err := uuid.FromBytes(req.GetConnection().GetId())
	if err != nil {
		return nil, xerrors.Errorf("connection id from bytes: %w", err)
	}

	action, err := db2sdk.ConnectionLogActionFromAgentProtoConnectionAction(req.GetConnection().GetAction())
	if err != nil {
		return nil, err
	}
	connectionType, err := agentsdk.ConnectionTypeFromProto(req.GetConnection().GetType())
	if err != nil {
		return nil, err
	}

	// Fetch contextual data for this connection log event.
	workspaceAgent, err := a.AgentFn(ctx)
	if err != nil {
		return nil, xerrors.Errorf("get agent: %w", err)
	}
	workspace, err := a.Database.GetWorkspaceByAgentID(ctx, workspaceAgent.ID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace by agent id: %w", err)
	}

	reason := req.GetConnection().GetReason()
	connLogger := *a.ConnectionLogger.Load()
	err = connLogger.Export(ctx, database.ConnectionLog{
		ID:               uuid.New(),
		Time:             req.GetConnection().GetTimestamp().AsTime(),
		ConnectionID:     connectionID,
		OrganizationID:   workspace.OrganizationID,
		WorkspaceOwnerID: workspace.OwnerID,
		WorkspaceID:      workspace.ID,
		WorkspaceName:    workspace.Name,
		AgentName:        workspaceAgent.Name,
		Action:           action,
		Code:             req.GetConnection().GetStatusCode(),
		Ip:               database.ParseIP(req.GetConnection().GetIp()),
		ConnectionType: sql.NullString{
			String: string(connectionType),
			Valid:  true,
		},
		Reason: sql.NullString{
			String: reason,
			Valid:  reason != "",
		},

		// It's not possible to tell which user connected. Once we have
		// the capability, this may be reported by the agent.
		UserID: uuid.Nil,
	})
	if err != nil {
		return nil, xerrors.Errorf("export connection log: %w", err)
	}

	return &emptypb.Empty{}, nil
}
