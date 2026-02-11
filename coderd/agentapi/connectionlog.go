package agentapi

import (
	"context"
	"database/sql"
	"net/netip"
	"sync/atomic"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/emptypb"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/connectionlog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/tailnet"
)

type ConnLogAPI struct {
	AgentFn                  func(context.Context) (database.WorkspaceAgent, error)
	ConnectionLogger         *atomic.Pointer[connectionlog.ConnectionLogger]
	TailnetCoordinator       *atomic.Pointer[tailnet.Coordinator]
	Workspace                *CachedWorkspaceFields
	Database                 database.Store
	Log                      slog.Logger
	PublishWorkspaceUpdateFn func(context.Context, *database.WorkspaceAgent, wspubsub.WorkspaceEventKind) error
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

	// Inject RBAC object into context for dbauthz fast path, avoid having to
	// call GetWorkspaceByAgentID on every metadata update.
	rbacCtx := ctx
	var ws database.WorkspaceIdentity
	if dbws, ok := a.Workspace.AsWorkspaceIdentity(); ok {
		ws = dbws
		rbacCtx, err = dbauthz.WithWorkspaceRBAC(ctx, dbws.RBACObject())
		if err != nil {
			// Don't error level log here, will exit the function. We want to fall back to GetWorkspaceByAgentID.
			//nolint:gocritic
			a.Log.Debug(ctx, "Cached workspace was present but RBAC object was invalid", slog.F("err", err))
		}
	}

	// Fetch contextual data for this connection log event.
	workspaceAgent, err := a.AgentFn(rbacCtx)
	if err != nil {
		return nil, xerrors.Errorf("get agent: %w", err)
	}
	if ws.Equal(database.WorkspaceIdentity{}) {
		workspace, err := a.Database.GetWorkspaceByAgentID(ctx, workspaceAgent.ID)
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

	// At connect time, look up the tailnet peer to capture the
	// client hostname and description for session grouping later.
	var clientHostname, shortDescription sql.NullString
	if action == database.ConnectionStatusConnected && a.TailnetCoordinator != nil {
		if coord := a.TailnetCoordinator.Load(); coord != nil {
			for _, peer := range (*coord).TunnelPeers(workspaceAgent.ID) {
				if peer.Node != nil {
					// Match peer by checking if any of its addresses
					// match the connection IP.
					for _, addr := range peer.Node.Addresses {
						prefix, err := netip.ParsePrefix(addr)
						if err != nil {
							continue
						}
						if logIP.Valid && prefix.Addr().String() == logIP.IPNet.IP.String() {
							if peer.Node.Hostname != "" {
								clientHostname = sql.NullString{String: peer.Node.Hostname, Valid: true}
							}
							if peer.Node.ShortDescription != "" {
								shortDescription = sql.NullString{String: peer.Node.ShortDescription, Valid: true}
							}
							break
						}
					}
				}
			}
		}
	}

	reason := req.GetConnection().GetReason()
	connLogger := *a.ConnectionLogger.Load()
	err = connLogger.Upsert(ctx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		Time:             req.GetConnection().GetTimestamp().AsTime(),
		OrganizationID:   ws.OrganizationID,
		WorkspaceOwnerID: ws.OwnerID,
		WorkspaceID:      ws.ID,
		WorkspaceName:    ws.Name,
		AgentName:        workspaceAgent.Name,
		AgentID:          uuid.NullUUID{UUID: workspaceAgent.ID, Valid: true},
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
		SessionID: uuid.NullUUID{},
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
		UserAgent:        sql.NullString{},
		ClientHostname:   clientHostname,
		ShortDescription: shortDescription,
		SlugOrPort: sql.NullString{
			String: req.GetConnection().GetSlugOrPort(),
			Valid:  req.GetConnection().GetSlugOrPort() != "",
		},
	})
	if err != nil {
		return nil, xerrors.Errorf("export connection log: %w", err)
	}

	// At disconnect time, find or create a session for this connection.
	// This groups related connection logs into workspace sessions.
	if action == database.ConnectionStatusDisconnected {
		a.assignSessionForDisconnect(ctx, connectionID, ws, workspaceAgent, logIPRaw, req)
	}

	if a.PublishWorkspaceUpdateFn != nil {
		if err := a.PublishWorkspaceUpdateFn(ctx, &workspaceAgent, wspubsub.WorkspaceEventKindConnectionLogUpdate); err != nil {
			a.Log.Warn(ctx, "failed to publish connection log update", slog.Error(err))
		}
	}

	return &emptypb.Empty{}, nil
}

// assignSessionForDisconnect looks up the existing connection log for this
// connection ID and finds or creates a session to group it with.
func (a *ConnLogAPI) assignSessionForDisconnect(
	ctx context.Context,
	connectionID uuid.UUID,
	ws database.WorkspaceIdentity,
	workspaceAgent database.WorkspaceAgent,
	logIPRaw string,
	req *agentproto.ReportConnectionRequest,
) {
	//nolint:gocritic // The agent context doesn't have connection_log
	// permissions. Session creation is authorized by the workspace
	// access already validated in ReportConnection.
	ctx = dbauthz.AsConnectionLogger(ctx)

	existingLog, err := a.Database.GetConnectionLogByConnectionID(ctx, database.GetConnectionLogByConnectionIDParams{
		ConnectionID: uuid.NullUUID{UUID: connectionID, Valid: true},
		WorkspaceID:  ws.ID,
		AgentName:    workspaceAgent.Name,
	})
	if err != nil {
		a.Log.Warn(ctx, "failed to look up connection log for session assignment",
			slog.Error(err),
			slog.F("connection_id", connectionID),
		)
		return
	}

	_, err = a.Database.FindOrCreateSessionForDisconnect(ctx, database.FindOrCreateSessionForDisconnectParams{
		WorkspaceID:      ws.ID.String(),
		Ip:               logIPRaw,
		ClientHostname:   existingLog.ClientHostname,
		ShortDescription: existingLog.ShortDescription,
		ConnectTime:      existingLog.ConnectTime,
		DisconnectTime:   req.GetConnection().GetTimestamp().AsTime(),
		AgentID:          uuid.NullUUID{UUID: workspaceAgent.ID, Valid: true},
	})
	if err != nil {
		a.Log.Warn(ctx, "failed to find or create session for disconnect",
			slog.Error(err),
			slog.F("connection_id", connectionID),
		)
	}
}
