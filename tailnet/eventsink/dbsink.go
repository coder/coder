package eventsink

import (
	"context"

	"github.com/google/uuid"
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

func (s *Sink) AddedTunnel(src, dst uuid.UUID) {
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
