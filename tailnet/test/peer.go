package test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"golang.org/x/xerrors"
	"tailscale.com/types/key"

	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
)

type PeerStatus struct {
	preferredDERP     int32
	status            proto.CoordinateResponse_PeerUpdate_Kind
	readyForHandshake bool
}

type PeerOption func(*Peer)

func WithID(id uuid.UUID) PeerOption {
	return func(p *Peer) {
		p.ID = id
	}
}

func WithAuth(auth tailnet.CoordinateeAuth) PeerOption {
	return func(p *Peer) {
		p.auth = auth
	}
}

type Peer struct {
	ctx         context.Context
	cancel      context.CancelFunc
	t           testing.TB
	ID          uuid.UUID
	auth        tailnet.CoordinateeAuth
	name        string
	nodeKey     key.NodePublic
	discoKey    key.DiscoPublic
	resps       <-chan *proto.CoordinateResponse
	reqs        chan<- *proto.CoordinateRequest
	peers       map[uuid.UUID]PeerStatus
	peerUpdates map[uuid.UUID][]*proto.CoordinateResponse_PeerUpdate
}

func NewPeer(ctx context.Context, t testing.TB, coord tailnet.CoordinatorV2, name string, opts ...PeerOption) *Peer {
	p := &Peer{
		t:           t,
		name:        name,
		peers:       make(map[uuid.UUID]PeerStatus),
		peerUpdates: make(map[uuid.UUID][]*proto.CoordinateResponse_PeerUpdate),
		ID:          uuid.New(),
		// SingleTailnetCoordinateeAuth allows connections to arbitrary peers
		auth: tailnet.SingleTailnetCoordinateeAuth{},
		// required for converting to and from protobuf, so we always include them
		nodeKey:  key.NewNode().Public(),
		discoKey: key.NewDisco().Public(),
	}
	p.ctx, p.cancel = context.WithCancel(ctx)
	for _, opt := range opts {
		opt(p)
	}

	p.reqs, p.resps = coord.Coordinate(p.ctx, p.ID, name, p.auth)
	return p
}

// NewAgent is a wrapper around NewPeer, creating a peer with Agent auth tied to its ID
func NewAgent(ctx context.Context, t testing.TB, coord tailnet.CoordinatorV2, name string) *Peer {
	id := uuid.New()
	return NewPeer(ctx, t, coord, name, WithID(id), WithAuth(tailnet.AgentCoordinateeAuth{ID: id}))
}

// NewClient is a wrapper around NewPeer, creating a peer with Client auth tied to the provided agentID
func NewClient(ctx context.Context, t testing.TB, coord tailnet.CoordinatorV2, name string, agentID uuid.UUID) *Peer {
	p := NewPeer(ctx, t, coord, name, WithAuth(tailnet.ClientCoordinateeAuth{AgentID: agentID}))
	p.AddTunnel(agentID)
	return p
}

func (p *Peer) ConnectToCoordinator(ctx context.Context, c tailnet.CoordinatorV2) {
	p.t.Helper()
	p.reqs, p.resps = c.Coordinate(ctx, p.ID, p.name, p.auth)
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

func (p *Peer) RemoveTunnel(other uuid.UUID) {
	p.t.Helper()
	req := &proto.CoordinateRequest{RemoveTunnel: &proto.CoordinateRequest_Tunnel{Id: tailnet.UUIDToByteSlice(other)}}
	select {
	case <-p.ctx.Done():
		p.t.Errorf("timeout removing tunnel for %s", p.name)
		return
	case p.reqs <- req:
		return
	}
}

func (p *Peer) UpdateDERP(derp int32) {
	p.t.Helper()
	node := &proto.Node{PreferredDerp: derp}
	p.UpdateNode(node)
}

func (p *Peer) UpdateNode(node *proto.Node) {
	p.t.Helper()
	nk, err := p.nodeKey.MarshalBinary()
	assert.NoError(p.t, err)
	node.Key = nk
	dk, err := p.discoKey.MarshalText()
	assert.NoError(p.t, err)
	node.Disco = string(dk)
	req := &proto.CoordinateRequest{UpdateSelf: &proto.CoordinateRequest_UpdateSelf{Node: node}}
	select {
	case <-p.ctx.Done():
		p.t.Errorf("timeout updating node for %s", p.name)
		return
	case p.reqs <- req:
		return
	}
}

func (p *Peer) ReadyForHandshake(peer uuid.UUID) {
	p.t.Helper()

	req := &proto.CoordinateRequest{ReadyForHandshake: []*proto.CoordinateRequest_ReadyForHandshake{{
		Id: peer[:],
	}}}
	select {
	case <-p.ctx.Done():
		p.t.Errorf("timeout sending ready for handshake for %s", p.name)
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
		if err := p.readOneResp(); err != nil {
			assert.NoError(p.t, err)
			return
		}
	}
}

func (p *Peer) AssertNeverHasDERPs(ctx context.Context, other uuid.UUID, expected ...int32) {
	p.t.Helper()
	for {
		select {
		case <-ctx.Done():
			return
		case resp, ok := <-p.resps:
			if !ok {
				p.t.Errorf("response channel closed")
			}
			if !assert.NoError(p.t, p.handleResp(resp)) {
				return
			}
			derp, ok := p.peers[other]
			if !ok {
				continue
			}
			if !assert.NotContains(p.t, expected, derp) {
				return
			}
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
		if err := p.readOneResp(); err != nil {
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
		if err := p.readOneResp(); err != nil {
			assert.NoError(p.t, err)
			return
		}
	}
}

func (p *Peer) AssertEventuallyResponsesClosed(expectedError string) {
	gotErr := false
	p.t.Helper()
	for {
		err := p.readOneResp()
		if xerrors.Is(err, errResponsesClosed) {
			if !gotErr && expectedError != "" {
				p.t.Errorf("responses closed without error '%s'", expectedError)
			}
			return
		}
		if err != nil && expectedError != "" && err.Error() == expectedError {
			gotErr = true
			continue
		}
		if !assert.NoError(p.t, err) {
			return
		}
	}
}

func (p *Peer) AssertNotClosed(d time.Duration) {
	p.t.Helper()
	// nolint: gocritic // erroneously thinks we're hardcoding non testutil constants here
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			// success!
			return
		case <-p.ctx.Done():
			p.t.Error("main ctx timeout before elapsed time")
			return
		case resp, ok := <-p.resps:
			if !ok {
				p.t.Error("response channel closed")
				return
			}
			err := p.handleResp(resp)
			if !assert.NoError(p.t, err) {
				return
			}
		}
	}
}

func (p *Peer) AssertEventuallyReadyForHandshake(other uuid.UUID) {
	p.t.Helper()
	for {
		o := p.peers[other]
		if o.readyForHandshake {
			return
		}

		err := p.readOneResp()
		if xerrors.Is(err, errResponsesClosed) {
			return
		}
	}
}

func (p *Peer) AssertEventuallyGetsError(match string) {
	p.t.Helper()
	for {
		err := p.readOneResp()
		if xerrors.Is(err, errResponsesClosed) {
			p.t.Error("closed before target error")
			return
		}

		if err != nil && assert.ErrorContains(p.t, err, match) {
			return
		}
	}
}

// AssertNeverUpdateKind asserts that we have not received
// any updates on the provided peer for the provided kind.
func (p *Peer) AssertNeverUpdateKind(peer uuid.UUID, kind proto.CoordinateResponse_PeerUpdate_Kind) {
	p.t.Helper()

	updates, ok := p.peerUpdates[peer]
	assert.True(p.t, ok, "expected updates for peer %s", peer)

	for _, update := range updates {
		assert.NotEqual(p.t, kind, update.Kind, update)
	}
}

var errResponsesClosed = xerrors.New("responses closed")

func (p *Peer) readOneResp() error {
	select {
	case <-p.ctx.Done():
		return p.ctx.Err()
	case resp, ok := <-p.resps:
		if !ok {
			return errResponsesClosed
		}
		err := p.handleResp(resp)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Peer) handleResp(resp *proto.CoordinateResponse) error {
	if resp.Error != "" {
		return xerrors.New(resp.Error)
	}
	for _, update := range resp.PeerUpdates {
		id, err := uuid.FromBytes(update.Id)
		if err != nil {
			return err
		}
		p.peerUpdates[id] = append(p.peerUpdates[id], update)

		switch update.Kind {
		case proto.CoordinateResponse_PeerUpdate_NODE, proto.CoordinateResponse_PeerUpdate_LOST:
			peer := p.peers[id]
			peer.preferredDERP = update.GetNode().GetPreferredDerp()
			peer.status = update.Kind
			p.peers[id] = peer
		case proto.CoordinateResponse_PeerUpdate_DISCONNECTED:
			delete(p.peers, id)
		case proto.CoordinateResponse_PeerUpdate_READY_FOR_HANDSHAKE:
			peer := p.peers[id]
			peer.readyForHandshake = true
			p.peers[id] = peer
		default:
			return xerrors.Errorf("unhandled update kind %s", update.Kind)
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

func (p *Peer) UngracefulDisconnect(ctx context.Context) {
	p.t.Helper()
	close(p.reqs)
	p.Close(ctx)
}

type FakeSubjectKey struct{}

type FakeCoordinateeAuth struct {
	Chan chan struct{}
}

func (f FakeCoordinateeAuth) Authorize(ctx context.Context, _ *proto.CoordinateRequest) error {
	_, ok := ctx.Value(FakeSubjectKey{}).(struct{})
	if !ok {
		return xerrors.New("unauthorized")
	}
	f.Chan <- struct{}{}
	return nil
}

var _ tailnet.CoordinateeAuth = (*FakeCoordinateeAuth)(nil)
