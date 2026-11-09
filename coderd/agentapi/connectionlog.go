package agentapi

import (
	"context"
	"database/sql"
	"sync/atomic"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/emptypb"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/connectionlog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/sdk2db"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
)

type ConnLogAPI struct {
	AgentID   uuid.UUID
	AgentName string
	// WorkspaceBuildID is the build that created the agent.
	WorkspaceBuildID uuid.UUID
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
	method, appName, err := sdk2db.ConnectionLogFromAgentConnection(req.GetConnection())
	if err != nil {
		return nil, err
	}

	var (
		code    sql.NullInt32
		rxBytes sql.NullInt64
		txBytes sql.NullInt64
	)
	if action == database.ConnectionStatusDisconnected {
		code = sql.NullInt32{
			Int32: req.GetConnection().GetStatusCode(),
			Valid: true,
		}
		rxBytes = sql.NullInt64{
			Int64: req.GetConnection().GetRxBytes(),
			Valid: req.GetConnection().RxBytes != nil,
		}
		txBytes = sql.NullInt64{
			Int64: req.GetConnection().GetTxBytes(),
			Valid: req.GetConnection().TxBytes != nil,
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
	if action == database.ConnectionStatusDisconnected && reason == string(codersdk.DisconnectReasonServerShutdown) {
		reason = a.workspaceShutdownReason(ctx, ws.ID, req.GetConnection())
	}
	clientSessionID := req.GetConnection().GetClientSessionId()
	if !tracing.ValidSessionID(clientSessionID) {
		clientSessionID = ""
	}
	connLogger := *a.ConnectionLogger.Load()
	err = connLogger.Upsert(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             req.GetConnection().GetTimestamp().AsTime(),
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		WorkspaceName:    ws.Name,
		AgentName:        a.AgentName,
		ConnectionMethod: method,
		AppNameOrPort:    sql.NullString{String: appName, Valid: appName != ""},
		Code:             code,
		IP:               logIP,
		ConnectionID: uuid.NullUUID{
			UUID:  connectionID,
			Valid: true,
		},
		ClientSessionID: sql.NullString{
			String: clientSessionID,
			Valid:  clientSessionID != "",
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
		RxBytes:   rxBytes,
		TxBytes:   txBytes,
	})
	if err != nil {
		return nil, xerrors.Errorf("export connection log: %w", err)
	}

	return &emptypb.Empty{}, nil
}

func (a *ConnLogAPI) workspaceShutdownReason(ctx context.Context, workspaceID uuid.UUID, connection *agentproto.Connection) string {
	reason := string(codersdk.DisconnectReasonServerShutdown)
	clientSessionID := connection.GetClientSessionId()
	if !tracing.ValidSessionID(clientSessionID) {
		clientSessionID = ""
	}
	logger := a.Log.With(
		slog.F("workspace_id", workspaceID),
		slog.F("connection_id", uuid.UUID(connection.GetId())),
		slog.F("client_session_id", clientSessionID),
	)

	build, err := a.Database.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspaceID)
	if err != nil {
		logger.Warn(ctx, "get shutdown workspace build", slog.Error(err))
		return reason
	}
	// The agent stays connected after a newer build exists only while that
	// build stops or deletes the workspace (see checkBuildIsLatest), so a newer
	// build is the shutdown cause.
	replaced := build.ID != a.WorkspaceBuildID
	// A delayed report must not be attributed to a build requested after the
	// connection closed.
	requestedAfterClose := build.CreatedAt.After(connection.GetTimestamp().AsTime())
	if !replaced || requestedAfterClose {
		return reason
	}

	job, err := a.Database.GetProvisionerJobByID(ctx, build.JobID)
	if err != nil {
		logger.Warn(ctx, "get shutdown provisioner job", slog.Error(err))
		return reason
	}
	// Queued, failed, and canceled jobs do not establish a shutdown cause.
	active := job.JobStatus == database.ProvisionerJobStatusRunning ||
		job.JobStatus == database.ProvisionerJobStatusSucceeded
	if !active {
		return reason
	}
	return string(codersdk.DisconnectReasonWorkspaceStopped)
}
