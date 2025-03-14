package tailnet
import (
	"fmt"
	"errors"
	"context"
	"time"
	"github.com/google/uuid"
	"cdr.dev/slog"
	"github.com/coder/coder/v2/tailnet/proto"
)
type peer struct {
	logger slog.Logger
	id     uuid.UUID
	node   *proto.Node
	resps  chan<- *proto.CoordinateResponse
	reqs   <-chan *proto.CoordinateRequest
	auth   CoordinateeAuth
	sent   map[uuid.UUID]*proto.Node
	name       string
	start      time.Time
	lastWrite  time.Time
	overwrites int
}
// updateMappingLocked updates the mapping for another peer linked to this one by a tunnel.  This method
// is NOT threadsafe and must be called while holding the core lock.
func (p *peer) updateMappingLocked(id uuid.UUID, n *proto.Node, k proto.CoordinateResponse_PeerUpdate_Kind, reason string) error {
	logger := p.logger.With(slog.F("from_id", id), slog.F("kind", k), slog.F("reason", reason))
	update, err := p.storeMappingLocked(id, n, k, reason)
	if errors.Is(err, noResp) {
		logger.Debug(context.Background(), "skipping update")
		return nil
	}
	if err != nil {
		return err
	}
	req := &proto.CoordinateResponse{PeerUpdates: []*proto.CoordinateResponse_PeerUpdate{update}}
	select {
	case p.resps <- req:
		p.lastWrite = time.Now()
		logger.Debug(context.Background(), "wrote peer update")
		return nil
	default:
		return ErrWouldBlock
	}
}
// batchUpdateMapping updates the mappings for a list of peers linked to this one by a tunnel. This
// method is NOT threadsafe and must be called while holding the core lock.
func (p *peer) batchUpdateMappingLocked(others []*peer, k proto.CoordinateResponse_PeerUpdate_Kind, reason string) error {
	req := &proto.CoordinateResponse{}
	for _, other := range others {
		if other == nil || other.node == nil {
			continue
		}
		update, err := p.storeMappingLocked(other.id, other.node, k, reason)
		if errors.Is(err, noResp) {
			continue
		}
		if err != nil {
			return err
		}
		req.PeerUpdates = append(req.PeerUpdates, update)
	}
	if len(req.PeerUpdates) == 0 {
		return nil
	}
	select {
	case p.resps <- req:
		p.lastWrite = time.Now()
		p.logger.Debug(context.Background(), "wrote batched update", slog.F("num_peer_updates", len(req.PeerUpdates)))
		return nil
	default:
		return ErrWouldBlock
	}
}
var noResp = errors.New("no response needed")
func (p *peer) storeMappingLocked(
	id uuid.UUID, n *proto.Node, k proto.CoordinateResponse_PeerUpdate_Kind, reason string,
) (
	*proto.CoordinateResponse_PeerUpdate, error,
) {
	p.logger.Debug(context.Background(), "got updated mapping",
		slog.F("from_id", id), slog.F("kind", k), slog.F("reason", reason))
	sn, ok := p.sent[id]
	switch {
	case !ok && (k == proto.CoordinateResponse_PeerUpdate_LOST || k == proto.CoordinateResponse_PeerUpdate_DISCONNECTED):
		// we don't need to send a lost/disconnect update if we've never sent an update about this peer
		return nil, noResp
	case !ok && k == proto.CoordinateResponse_PeerUpdate_NODE:
		p.sent[id] = n
	case ok && k == proto.CoordinateResponse_PeerUpdate_LOST:
		delete(p.sent, id)
	case ok && k == proto.CoordinateResponse_PeerUpdate_DISCONNECTED:
		delete(p.sent, id)
	case ok && k == proto.CoordinateResponse_PeerUpdate_NODE:
		eq, err := sn.Equal(n)
		if err != nil {
			p.logger.Critical(context.Background(), "failed to compare nodes", slog.F("old", sn), slog.F("new", n))
			return nil, fmt.Errorf("failed to compare nodes: %s", sn.String())
		}
		if eq {
			return nil, noResp
		}
		p.sent[id] = n
	}
	return &proto.CoordinateResponse_PeerUpdate{
		Id:     id[:],
		Kind:   k,
		Node:   n,
		Reason: reason,
	}, nil
}
func (p *peer) reqLoop(ctx context.Context, logger slog.Logger, handler func(context.Context, *peer, *proto.CoordinateRequest) error) {
	for {
		select {
		case <-ctx.Done():
			logger.Debug(ctx, "peerReadLoop context done")
			return
		case req, ok := <-p.reqs:
			if !ok {
				logger.Debug(ctx, "peerReadLoop channel closed")
				return
			}
			logger.Debug(ctx, "peerReadLoop got request")
			if err := handler(ctx, p, req); err != nil {
				if errors.Is(err, ErrAlreadyRemoved) || errors.Is(err, ErrClosed) {
					return
				}
				logger.Error(ctx, "peerReadLoop error handling request", slog.Error(err), slog.F("request", req))
				return
			}
		}
	}
}
func (p *peer) htmlDebug() HTMLPeer {
	node := "<nil>"
	if p.node != nil {
		node = p.node.String()
	}
	return HTMLPeer{
		ID:           p.id,
		Name:         p.name,
		CreatedAge:   time.Since(p.start),
		LastWriteAge: time.Since(p.lastWrite),
		Overwrites:   p.overwrites,
		Node:         node,
	}
}
