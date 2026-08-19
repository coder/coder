package agentapi

import (
	"context"
	"database/sql"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/emptypb"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/connectionlog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
)

type ConnLogAPI struct {
	AgentID          uuid.UUID
	AgentName        string
	ConnectionLogger *atomic.Pointer[connectionlog.ConnectionLogger]
	Workspace        *CachedWorkspaceFields
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

	var ws database.WorkspaceIdentity
	if dbws, ok := a.Workspace.AsWorkspaceIdentity(); ok {
		ws = dbws
	}
	if ws.Equal(database.WorkspaceIdentity{}) {
		workspace, err := a.Database.GetWorkspaceByAgentID(ctx, a.AgentID)
		if err != nil {
			return nil, xerrors.Errorf("get workspace by agent id: %w", err)
		}
		ws = database.WorkspaceIdentityFromWorkspace(workspace)
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
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		WorkspaceName:    ws.Name,
		AgentName:        a.AgentName,
		Type:             connectionType,
		Code:             code,
		IP:               logIP,
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
		// Only set for file operation events.
		FileProtocol: database.NullConnectionLogFileProtocol{},
		FileAction:   database.NullConnectionLogFileAction{},
		FilePath:     sql.NullString{},
		FileTarget:   sql.NullString{},
	})
	if err != nil {
		return nil, xerrors.Errorf("export connection log: %w", err)
	}

	return &emptypb.Empty{}, nil
}

// ReportFileOperations records file operations observed during a
// file-transfer session (SFTP, SCP, rsync) as point-in-time connection
// log entries. Each operation shares the connection_id of its parent
// session so they can be grouped together.
func (a *ConnLogAPI) ReportFileOperations(ctx context.Context, req *agentproto.ReportFileOperationsRequest) (*agentproto.ReportFileOperationsResponse, error) {
	if len(req.GetOperations()) == 0 {
		return &agentproto.ReportFileOperationsResponse{}, nil
	}

	var ws database.WorkspaceIdentity
	if dbws, ok := a.Workspace.AsWorkspaceIdentity(); ok {
		ws = dbws
	}
	if ws.Equal(database.WorkspaceIdentity{}) {
		workspace, err := a.Database.GetWorkspaceByAgentID(ctx, a.AgentID)
		if err != nil {
			return nil, xerrors.Errorf("get workspace by agent id: %w", err)
		}
		ws = database.WorkspaceIdentityFromWorkspace(workspace)
	}

	connLogger := *a.ConnectionLogger.Load()
	for _, op := range req.GetOperations() {
		connectionID, err := uuid.FromBytes(op.GetConnectionId())
		if err != nil {
			return nil, xerrors.Errorf("connection id from bytes: %w", err)
		}
		if connectionID == uuid.Nil {
			return nil, xerrors.New("connection ID cannot be nil")
		}
		protocol, err := db2sdk.ConnectionLogFileProtocolFromAgentProtoFileTransferProtocol(op.GetProtocol())
		if err != nil {
			return nil, err
		}
		action, err := db2sdk.ConnectionLogFileActionFromAgentProtoFileTransferAction(op.GetAction())
		if err != nil {
			return nil, err
		}
		if op.GetPath() == "" {
			return nil, xerrors.New("file operation path cannot be empty")
		}

		err = connLogger.Upsert(ctx, database.UpsertConnectionLogParams{
			ID:               uuid.New(),
			Time:             op.GetTimestamp().AsTime(),
			OrganizationID:   ws.OrganizationID,
			WorkspaceOwnerID: ws.OwnerID,
			WorkspaceID:      ws.ID,
			WorkspaceName:    ws.Name,
			AgentName:        a.AgentName,
			Type:             database.ConnectionTypeFileOperation,
			FileProtocol: database.NullConnectionLogFileProtocol{
				ConnectionLogFileProtocol: protocol,
				Valid:                     true,
			},
			FileAction: database.NullConnectionLogFileAction{
				ConnectionLogFileAction: action,
				Valid:                   true,
			},
			FilePath: sql.NullString{String: op.GetPath(), Valid: true},
			FileTarget: sql.NullString{
				String: op.GetTarget(),
				Valid:  op.GetTarget() != "",
			},
			// The parent file-transfer session's connection ID, so the
			// operation can be grouped with it. File operation rows are
			// excluded from the connect/disconnect pairing index, so
			// sharing the ID never merges rows.
			ConnectionID: uuid.NullUUID{
				UUID:  connectionID,
				Valid: true,
			},
			// File operations are point-in-time events: they are always
			// "connected" and never receive a disconnect event.
			ConnectionStatus: database.ConnectionStatusConnected,

			// It's not possible to tell which user connected. Once we have
			// the capability, this may be reported by the agent.
			UserID: uuid.NullUUID{Valid: false},
			// N/A
			Code: sql.NullInt32{},
			// N/A
			IP: pqtype.Inet{},
			// N/A
			UserAgent: sql.NullString{},
			// N/A
			SlugOrPort: sql.NullString{},
			// N/A
			DisconnectReason: sql.NullString{},
		})
		if err != nil {
			return nil, xerrors.Errorf("export file operation log: %w", err)
		}
	}

	return &agentproto.ReportFileOperationsResponse{}, nil
}
