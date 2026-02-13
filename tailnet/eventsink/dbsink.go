package eventsink

import (
	"context"
	"database/sql"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	gProto "google.golang.org/protobuf/proto"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
)

type Sink struct {
	ctx    context.Context
	db     database.Store
	logger slog.Logger
}

var eventSinkSubject = rbac.Subject{
	ID: uuid.Nil.String(),
	Roles: rbac.Roles([]rbac.Role{
		{
			Identifier:  rbac.RoleIdentifier{Name: "eventsink"},
			DisplayName: "Event Sink",
			Site: rbac.Permissions(map[string][]policy.Action{
				rbac.ResourceTailnetCoordinator.Type: {policy.WildcardSymbol},
			}),
			User:    []rbac.Permission{},
			ByOrgID: map[string]rbac.OrgPermissions{},
		},
	}),
	Scope: rbac.ScopeAll,
}.WithCachedASTValue()

func NewEventSink(ctx context.Context, db database.Store, logger slog.Logger) tailnet.EventSink {
	return &Sink{
		ctx:    dbauthz.As(ctx, eventSinkSubject),
		db:     db,
		logger: logger.Named("events"),
	}
}

func (s *Sink) AddedTunnel(src, dst uuid.UUID, srcNode *proto.Node) {
	// This is very janky, the Coordinator is calling this while holding a lock, so we can't block
	// here.
	go func() {
		err := s.db.InsertTailnetPeeringEvent(s.ctx, database.InsertTailnetPeeringEventParams{
			PeeringID:  tailnet.PeeringIDFromUUIDs(src, dst),
			EventType:  database.TailnetPeeringEventTypeAddedTunnel,
			SrcPeerID:  uuid.NullUUID{UUID: src, Valid: true},
			DstPeerID:  uuid.NullUUID{UUID: dst, Valid: true},
			Node:       nil,
			OccurredAt: dbtime.Now(),
		})
		if err != nil {
			s.logger.Error(s.ctx, "failed to added tunnel event", slog.Error(err))
		}

		// Also create a connection_log entry for the tunnel so that
		// workspace session history tracks system/tunnel connections.
		s.logTunnelConnection(src, dst, srcNode, database.ConnectionStatusConnected)
	}()
}

func (s *Sink) RemovedTunnel(src, dst uuid.UUID) {
	// This is very janky, the Coordinator is calling this while holding a lock, so we can't block
	// here.
	go func() {
		err := s.db.InsertTailnetPeeringEvent(s.ctx, database.InsertTailnetPeeringEventParams{
			PeeringID:  tailnet.PeeringIDFromUUIDs(src, dst),
			EventType:  database.TailnetPeeringEventTypeRemovedTunnel,
			SrcPeerID:  uuid.NullUUID{UUID: src, Valid: true},
			DstPeerID:  uuid.NullUUID{UUID: dst, Valid: true},
			Node:       nil,
			OccurredAt: dbtime.Now(),
		})
		if err != nil {
			s.logger.Error(s.ctx, "failed to insert removed tunnel event", slog.Error(err))
		}

		// Log the disconnect in connection_log.
		s.logTunnelConnection(src, dst, nil, database.ConnectionStatusDisconnected)
	}()
}

func (s *Sink) SentPeerUpdate(recipient uuid.UUID, update *proto.CoordinateResponse_PeerUpdate) {
	// This is very janky, the Coordinator is calling this while holding a lock, so we can't block
	// here.
	go func() {
		peerID, err := uuid.FromBytes(update.Id)
		if err != nil {
			s.logger.Error(s.ctx, "failed to parse peer ID", slog.Error(err))
			return
		}
		var eventType string
		switch update.Kind {
		case proto.CoordinateResponse_PeerUpdate_NODE:
			eventType = database.TailnetPeeringEventTypePeerUpdateNode
		case proto.CoordinateResponse_PeerUpdate_DISCONNECTED:
			eventType = database.TailnetPeeringEventTypePeerUpdateDisconnected
		case proto.CoordinateResponse_PeerUpdate_LOST:
			eventType = database.TailnetPeeringEventTypePeerUpdateLost
		case proto.CoordinateResponse_PeerUpdate_READY_FOR_HANDSHAKE:
			eventType = database.TailnetPeeringEventTypePeerUpdateReadyForHandshake
		default:
			s.logger.Error(s.ctx, "unknown peer update kind", slog.F("kind", update.Kind))
			return
		}
		nodeBytes, err := gProto.Marshal(update.Node)
		if err != nil {
			s.logger.Error(s.ctx, "failed to marshal node", slog.Error(err))
			return
		}
		err = s.db.InsertTailnetPeeringEvent(s.ctx, database.InsertTailnetPeeringEventParams{
			PeeringID:  tailnet.PeeringIDFromUUIDs(recipient, peerID),
			EventType:  eventType,
			SrcPeerID:  uuid.NullUUID{UUID: peerID, Valid: true},
			DstPeerID:  uuid.NullUUID{UUID: recipient, Valid: true},
			Node:       nodeBytes,
			OccurredAt: dbtime.Now(),
		})
		if err != nil {
			s.logger.Error(s.ctx, "failed to insert peer update event", slog.Error(err))
		}
	}()
}

// logTunnelConnection creates or updates a connection_log entry for a
// tunnel connection so that system/tunnel peers appear in workspace
// session history. It uses a separate auth context with connection_log
// write permissions since the default eventSinkSubject only covers
// tailnet coordinator resources.
func (s *Sink) logTunnelConnection(src, dst uuid.UUID, srcNode *proto.Node, status database.ConnectionStatus) {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	// Use system-restricted context for read-only lookups.
	// AsConnectionLogger only has connection_log permissions,
	// not workspace/agent read permissions.
	//nolint:gocritic // Read-only workspace metadata lookups need system scope.
	readCtx := dbauthz.AsSystemRestricted(ctx)

	// dst is the workspace agent ID. Look up agent and workspace info.
	agent, err := s.db.GetWorkspaceAgentByID(readCtx, dst)
	if err != nil {
		// Not all tunnel peers are workspace agents. Skip silently.
		return
	}

	resource, err := s.db.GetWorkspaceResourceByID(readCtx, agent.ResourceID)
	if err != nil {
		s.logger.Warn(ctx, "failed to get resource for tunnel connection log", slog.Error(err))
		return
	}

	build, err := s.db.GetWorkspaceBuildByJobID(readCtx, resource.JobID)
	if err != nil {
		s.logger.Warn(ctx, "failed to get build for tunnel connection log", slog.Error(err))
		return
	}

	workspace, err := s.db.GetWorkspaceByID(readCtx, build.WorkspaceID)
	if err != nil {
		s.logger.Warn(ctx, "failed to get workspace for tunnel connection log", slog.Error(err))
		return
	}

	// Deterministic connection_id from (src, dst) so connect and
	// disconnect upsert the same row.
	connectionID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("tunnel:"+src.String()+":"+dst.String()))

	// Extract IP and metadata from node if available.
	var ip pqtype.Inet
	var hostname, shortDesc sql.NullString
	if srcNode != nil {
		if len(srcNode.Addresses) > 0 {
			if prefix, err := netip.ParsePrefix(srcNode.Addresses[0]); err == nil {
				ipAddr := prefix.Addr()
				ip = database.ParseIP(ipAddr.String())
			}
		}
		if srcNode.Hostname != "" {
			hostname = sql.NullString{String: srcNode.Hostname, Valid: true}
		}
		if srcNode.ShortDescription != "" {
			shortDesc = sql.NullString{String: srcNode.ShortDescription, Valid: true}
		}
	}

	// Use connection logger context for the actual write.
	//nolint:gocritic // Connection-log writes require connection logger scope.
	connLogCtx := dbauthz.AsConnectionLogger(ctx)

	now := dbtime.Now()
	_, err = s.db.UpsertConnectionLog(connLogCtx, database.UpsertConnectionLogParams{
		ID:               uuid.New(),
		OrganizationID:   workspace.OrganizationID,
		WorkspaceOwnerID: workspace.OwnerID,
		WorkspaceID:      workspace.ID,
		WorkspaceName:    workspace.Name,
		AgentName:        agent.Name,
		AgentID:          uuid.NullUUID{UUID: agent.ID, Valid: true},
		Type:             database.ConnectionTypeSystem,
		ConnectionID:     uuid.NullUUID{UUID: connectionID, Valid: true},
		ConnectionStatus: status,
		Ip:               ip,
		ClientHostname:   hostname,
		ShortDescription: shortDesc,
		Time:             now,
		// No user info available for tunnel connections.
		UserID:           uuid.NullUUID{},
		UserAgent:        sql.NullString{},
		Code:             sql.NullInt32{},
		SlugOrPort:       sql.NullString{},
		DisconnectReason: sql.NullString{},
		SessionID:        uuid.NullUUID{},
	})
	if err != nil {
		s.logger.Warn(ctx, "failed to log tunnel connection",
			slog.Error(err), slog.F("src_id", src), slog.F("dst_id", dst))
	}
}
