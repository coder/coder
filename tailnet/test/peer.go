package test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
)

type PeerStatus struct {
	preferredDERP int32
	status        proto.CoordinateResponse_PeerUpdate_Kind
}

type Peer struct {
	ctx    context.Context
	cancel context.CancelFunc
	t      testing.TB
	ID     uuid.UUID
	name   string
	resps  <-chan *proto.CoordinateResponse
	reqs   chan<- *proto.CoordinateRequest
	peers  map[uuid.UUID]PeerStatus
}

func NewPeer(ctx context.Context, t testing.TB, coord tailnet.CoordinatorV2, name string, id ...uuid.UUID) *Peer {
	p := &Peer{t: t, name: name, peers: make(map[uuid.UUID]PeerStatus)}
	p.ctx, p.cancel = context.WithCancel(ctx)
	if len(id) > 1 {
		t.Fatal("too many")
	}
	if len(id) == 1 {
		p.ID = id[0]
	} else {
		p.ID = uuid.New()
	}
	// SingleTailnetTunnelAuth allows connections to arbitrary peers
	p.reqs, p.resps = coord.Coordinate(p.ctx, p.ID, name, tailnet.SingleTailnetTunnelAuth{})
	return p
}

func (p *Peer) AddTunnel(other uuid.UUID) {
	p.t.Helper()
	req := &proto.CoordinateRequest{AddTunnel: &proto.CoordinateRequest_Tunnel{Id: tailnet.UUIDToByteSlice(other)}}
	select {
	case <-p.ctx.Done():
		p.t.Errorf("timeout adding tunnel for %s", p.name)
		return
	case p.reqs <- req:
		return
	}
}

func (p *Peer) UpdateDERP(derp int32) {
	p.t.Helper()
	req := &proto.CoordinateRequest{UpdateSelf: &proto.CoordinateRequest_UpdateSelf{Node: &proto.Node{PreferredDerp: derp}}}
	select {
	case <-p.ctx.Done():
		p.t.Errorf("timeout updating node for %s", p.name)
		return
	case p.reqs <- req:
		return
	}
}

func (p *Peer) Disconnect() {
	p.t.Helper()
	req := &proto.CoordinateRequest{Disconnect: &proto.CoordinateRequest_Disconnect{}}
	select {
	case <-p.ctx.Done():
		p.t.Errorf("timeout updating node for %s", p.name)
		return
	case p.reqs <- req:
		return
	}
}

func (p *Peer) AssertEventuallyHasDERP(other uuid.UUID, derp int32) {
	p.t.Helper()
	for {
		o, ok := p.peers[other]
		if ok && o.preferredDERP == derp {
			return
		}
		if err := p.handleOneResp(); err != nil {
			assert.NoError(p.t, err)
			return
		}
	}
}

func (p *Peer) AssertEventuallyDisconnected(other uuid.UUID) {
	p.t.Helper()
	for {
		_, ok := p.peers[other]
		if !ok {
			return
		}
		if err := p.handleOneResp(); err != nil {
			assert.NoError(p.t, err)
			return
		}
	}
}

func (p *Peer) AssertEventuallyLost(other uuid.UUID) {
	p.t.Helper()
	for {
		o := p.peers[other]
		if o.status == proto.CoordinateResponse_PeerUpdate_LOST {
			return
		}
		if err := p.handleOneResp(); err != nil {
			assert.NoError(p.t, err)
			return
		}
	}
}

func (p *Peer) AssertEventuallyResponsesClosed() {
	p.t.Helper()
	for {
		err := p.handleOneResp()
		if xerrors.Is(err, responsesClosed) {
			return
		}
		if !assert.NoError(p.t, err) {
			return
		}
	}
}

var responsesClosed = xerrors.New("responses closed")

func (p *Peer) handleOneResp() error {
	select {
	case <-p.ctx.Done():
		return p.ctx.Err()
	case resp, ok := <-p.resps:
		if !ok {
			return responsesClosed
		}
		for _, update := range resp.PeerUpdates {
			id, err := uuid.FromBytes(update.Id)
			if err != nil {
				return err
			}
			switch update.Kind {
			case proto.CoordinateResponse_PeerUpdate_NODE, proto.CoordinateResponse_PeerUpdate_LOST:
				p.peers[id] = PeerStatus{
					preferredDERP: update.GetNode().GetPreferredDerp(),
					status:        update.Kind,
				}
			case proto.CoordinateResponse_PeerUpdate_DISCONNECTED:
				delete(p.peers, id)
			default:
				return xerrors.Errorf("unhandled update kind %s", update.Kind)
			}
		}
	}
	return nil
}

func (p *Peer) Close(ctx context.Context) {
	p.t.Helper()
	p.cancel()
	for {
		select {
		case <-ctx.Done():
			p.t.Errorf("timeout waiting for responses to close for %s", p.name)
			return
		case _, ok := <-p.resps:
			if ok {
				continue
			}
			return
		}
	}
}
