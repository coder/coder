package agentapi

import (
	"context"
	"encoding/json"
	"strconv"
	"sync/atomic"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/emptypb"

	"cdr.dev/slog"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

type AuditAPI struct {
	AgentFn  func(context.Context) (database.WorkspaceAgent, error)
	Auditor  *atomic.Pointer[audit.Auditor]
	Database database.Store
	Log      slog.Logger
}

func (a *AuditAPI) ReportConnection(ctx context.Context, req *agentproto.ReportConnectionRequest) (*emptypb.Empty, error) {
	// We will use connection ID as request ID, typically this is the
	// SSH session ID as reported by the agent.
	connectionID, err := uuid.FromBytes(req.GetConnection().GetId())
	if err != nil {
		return nil, xerrors.Errorf("connection id from bytes: %w", err)
	}

	action, err := AgentProtoConnectionActionToAuditAction(req.GetConnection().GetAction())
	if err != nil {
		return nil, err
	}
	connectionType, err := AgentProtoConnectionTypeToAgentConnectionType(req.GetConnection().GetType())
	if err != nil {
		return nil, err
	}

	// Fetch contextual data for this audit event.
	workspaceAgent, err := a.AgentFn(ctx)
	if err != nil {
		return nil, xerrors.Errorf("get agent: %w", err)
	}
	workspace, err := a.Database.GetWorkspaceByAgentID(ctx, workspaceAgent.ID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace by agent id: %w", err)
	}
	build, err := a.Database.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspace.ID)
	if err != nil {
		return nil, xerrors.Errorf("get latest workspace build by workspace id: %w", err)
	}

	// We pass the below information to the Auditor so that it
	// can form a friendly string for the user to view in the UI.
	type additionalFields struct {
		audit.AdditionalFields

		ConnectionType agentsdk.ConnectionType `json:"connection_type"`
		Reason         string                  `json:"reason,omitempty"`
	}
	resourceInfo := additionalFields{
		AdditionalFields: audit.AdditionalFields{
			WorkspaceID:    workspace.ID,
			WorkspaceName:  workspace.Name,
			WorkspaceOwner: workspace.OwnerUsername,
			BuildNumber:    strconv.FormatInt(int64(build.BuildNumber), 10),
			BuildReason:    database.BuildReason(string(build.Reason)),
		},
		ConnectionType: connectionType,
		Reason:         req.GetConnection().GetReason(),
	}

	riBytes, err := json.Marshal(resourceInfo)
	if err != nil {
		a.Log.Error(ctx, "marshal resource info for agent connection failed", slog.Error(err))
		riBytes = []byte("{}")
	}

	audit.BackgroundAudit(ctx, &audit.BackgroundAuditParams[database.WorkspaceAgent]{
		Audit:            *a.Auditor.Load(),
		Log:              a.Log,
		Time:             req.GetConnection().GetTimestamp().AsTime(),
		OrganizationID:   workspace.OrganizationID,
		RequestID:        connectionID,
		Action:           action,
		New:              workspaceAgent,
		Old:              workspaceAgent,
		IP:               req.GetConnection().GetIp(),
		Status:           int(req.GetConnection().GetStatusCode()),
		AdditionalFields: riBytes,

		// It's not possible to tell which user connected. Once we have
		// the capability, this may be reported by the agent.
		UserID: uuid.Nil,
	})

	return &emptypb.Empty{}, nil
}

func AgentProtoConnectionActionToAuditAction(action agentproto.Connection_Action) (database.AuditAction, error) {
	switch action {
	case agentproto.Connection_CONNECT:
		return database.AuditActionConnect, nil
	case agentproto.Connection_DISCONNECT:
		return database.AuditActionDisconnect, nil
	default:
		return "", xerrors.Errorf("unknown agent connection action %q", action)
	}
}

func AgentProtoConnectionTypeToAgentConnectionType(typ agentproto.Connection_Type) (agentsdk.ConnectionType, error) {
	switch typ {
	case agentproto.Connection_SSH:
		return agentsdk.ConnectionTypeSSH, nil
	case agentproto.Connection_VSCODE:
		return agentsdk.ConnectionTypeVSCode, nil
	case agentproto.Connection_JETBRAINS:
		return agentsdk.ConnectionTypeJetBrains, nil
	case agentproto.Connection_RECONNECTING_PTY:
		return agentsdk.ConnectionTypeReconnectingPTY, nil
	default:
		return "", xerrors.Errorf("unknown agent connection type %q", typ)
	}
}
