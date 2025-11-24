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
)

type ConnLogAPI struct {
	AgentFn          func(context.Context) (database.WorkspaceAgent, error)
	ConnectionLogger *atomic.Pointer[connectionlog.ConnectionLogger]
	Database         database.Store
	Log              slog.Logger
}

func (a *ConnLogAPI) ReportConnection(ctx context.Context, req *agentproto.ReportConnectionRequest) (*emptypb.Empty, error) {
	// We use the connection ID to identify which connection log event to mark
	// as closed, when we receive a close action for that ID.
	connectionID, err := uuid.FromBytes(req.GetConnection().GetId())
	if err != nil {
		return nil, xerrors.Errorf("connection id from bytes: %w", err)
	}

	if connectionID == uuid.Nil {
		return nil, xerrors.New("connection ID cannot be nil")
	}
	action, err := db2sdk.ConnectionLogStatusFromAgentProtoConnectionAction(req.GetConnection().GetAction())
	if err != nil {
		return nil, err
	}
	connectionType, err := db2sdk.ConnectionLogConnectionTypeFromAgentProtoConnectionType(req.GetConnection().GetType())
	if err != nil {
		return nil, err
	}

	var code sql.NullInt32
	if action == database.ConnectionStatusDisconnected {
		code = sql.NullInt32{
			Int32: req.GetConnection().GetStatusCode(),
			Valid: true,
		}
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

	// Some older clients may incorrectly report "localhost" as the IP address.
	// Related to https://github.com/coder/coder/issues/20194
	logIPRaw := req.GetConnection().GetIp()
	if logIPRaw == "localhost" {
		logIPRaw = "127.0.0.1"
	}
	logIP := database.ParseIP(logIPRaw) // will return null if invalid

	reason := req.GetConnection().GetReason()
	connLogger := *a.ConnectionLogger.Load()
	err = connLogger.Upsert(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             req.GetConnection().GetTimestamp().AsTime(),
		OrganizationID:   workspace.OrganizationID,
		WorkspaceOwnerID: workspace.OwnerID,
		WorkspaceID:      workspace.ID,
		WorkspaceName:    workspace.Name,
		AgentName:        workspaceAgent.Name,
		Type:             connectionType,
		Code:             code,
		Ip:               logIP,
		ConnectionID: uuid.NullUUID{
			UUID:  connectionID,
			Valid: true,
		},
		DisconnectReason: sql.NullString{
			String: reason,
			Valid:  reason != "",
		},
		// We supply the action:
		// - So the DB can handle duplicate connections or disconnections properly.
		// - To make it clear whether this is a connection or disconnection
		//   prior to it's insertion into the DB (logs)
		ConnectionStatus: action,

		// It's not possible to tell which user connected. Once we have
		// the capability, this may be reported by the agent.
		UserID: uuid.NullUUID{
			Valid: false,
		},
		// N/A
		UserAgent: sql.NullString{},
		// N/A
		SlugOrPort: sql.NullString{},
	})
	if err != nil {
		return nil, xerrors.Errorf("export connection log: %w", err)
	}

	return &emptypb.Empty{}, nil
}
