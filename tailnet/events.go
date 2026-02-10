package tailnet

import (
	"context"
	"crypto/sha256"
	"net/netip"
	"slices"

	"github.com/google/uuid"
	gProto "google.golang.org/protobuf/proto"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/tailnet/proto"
)

type EventSink interface {
	AddedTunnel(src, dst uuid.UUID)
	RemovedTunnel(src, dst uuid.UUID)
	SentPeerUpdate(recipient uuid.UUID, update *proto.CoordinateResponse_PeerUpdate)
}

type eventSink struct {
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

func NewEventSink(ctx context.Context, db database.Store, logger slog.Logger) EventSink {
	return &eventSink{
		ctx:    dbauthz.As(ctx, eventSinkSubject),
		db:     db,
		logger: logger.Named("events"),
	}
}

func (s *eventSink) AddedTunnel(src, dst uuid.UUID) {
	err := s.db.InsertTailnetPeeringEvent(s.ctx, database.InsertTailnetPeeringEventParams{
		PeeringID:  PeeringIDFromUUIDs(src, dst),
		EventType:  database.TailnetPeeringEventTypeAddedTunnel,
		SrcPeerID:  uuid.NullUUID{UUID: src, Valid: true},
		DstPeerID:  uuid.NullUUID{UUID: dst, Valid: true},
		Node:       nil,
		OccurredAt: dbtime.Now(),
	})
	if err != nil {
		s.logger.Error(s.ctx, "failed to added tunnel event", slog.Error(err))
	}
}

func (s *eventSink) RemovedTunnel(src, dst uuid.UUID) {
	err := s.db.InsertTailnetPeeringEvent(s.ctx, database.InsertTailnetPeeringEventParams{
		PeeringID:  PeeringIDFromUUIDs(src, dst),
		EventType:  database.TailnetPeeringEventTypeRemovedTunnel,
		SrcPeerID:  uuid.NullUUID{UUID: src, Valid: true},
		DstPeerID:  uuid.NullUUID{UUID: dst, Valid: true},
		Node:       nil,
		OccurredAt: dbtime.Now(),
	})
	if err != nil {
		s.logger.Error(s.ctx, "failed to insert removed tunnel event", slog.Error(err))
	}
}

func (s *eventSink) SentPeerUpdate(recipient uuid.UUID, update *proto.CoordinateResponse_PeerUpdate) {
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
		PeeringID:  PeeringIDFromUUIDs(recipient, peerID),
		EventType:  eventType,
		SrcPeerID:  uuid.NullUUID{UUID: peerID, Valid: true},
		DstPeerID:  uuid.NullUUID{UUID: recipient, Valid: true},
		Node:       nodeBytes,
		OccurredAt: dbtime.Now(),
	})
	if err != nil {
		s.logger.Error(s.ctx, "failed to insert peer update event", slog.Error(err))
	}
}

func PeeringIDFromUUIDs(a, b uuid.UUID) []byte {
	// it's a little roundabout to construct the addrs, then convert back to slices, but I want to
	// make sure we're calling into PeeringIDFromAddrs, so that there is only one place the sorting
	// and hashing is done.
	aa := CoderServicePrefix.AddrFromUUID(a)
	ba := CoderServicePrefix.AddrFromUUID(b)
	return PeeringIDFromAddrs(aa, ba)
}

func PeeringIDFromAddrs(a, b netip.Addr) []byte {
	as := a.AsSlice()[6:]
	bs := b.AsSlice()[6:]
	h := sha256.New()
	if cmp := slices.Compare(as, bs); cmp < 0 {
		if _, err := h.Write(as); err != nil {
			panic(err)
		}
		if _, err := h.Write(bs); err != nil {
			panic(err)
		}
	} else {
		if _, err := h.Write(bs); err != nil {
			panic(err)
		}
		if _, err := h.Write(as); err != nil {
			panic(err)
		}
	}
	return h.Sum(nil)
}
