package tailnet

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/tailnet/proto"
)

// Coordinator exchanges nodes with agents to establish connections.
// ┌──────────────────┐   ┌────────────────────┐   ┌───────────────────┐   ┌──────────────────┐
// │tailnet.Coordinate├──►│tailnet.AcceptClient│◄─►│tailnet.AcceptAgent│◄──┤tailnet.Coordinate│
// └──────────────────┘   └────────────────────┘   └───────────────────┘   └──────────────────┘
// Coordinators have different guarantees for HA support.
type Coordinator interface {
	CoordinatorV1
	CoordinatorV2
}

type CoordinatorV1 interface {
	// ServeHTTPDebug serves a debug webpage that shows the internal state of
	// the coordinator.
	ServeHTTPDebug(w http.ResponseWriter, r *http.Request)
	// Node returns an in-memory node by ID.
	Node(id uuid.UUID) *Node
	// ServeClient accepts a WebSocket connection that wants to connect to an agent
	// with the specified ID.
	ServeClient(conn net.Conn, id uuid.UUID, agent uuid.UUID) error
	// ServeAgent accepts a WebSocket connection to an agent that listens to
	// incoming connections and publishes node updates.
	// Name is just used for debug information. It can be left blank.
	ServeAgent(conn net.Conn, id uuid.UUID, name string) error
	// Close closes the coordinator.
	Close() error

	ServeMultiAgent(id uuid.UUID) MultiAgentConn
}

// CoordinatorV2 is the interface for interacting with the coordinator via the 2.0 tailnet API.
type CoordinatorV2 interface {
	// ServeHTTPDebug serves a debug webpage that shows the internal state of
	// the coordinator.
	ServeHTTPDebug(w http.ResponseWriter, r *http.Request)
	// Node returns a node by peer ID, if known to the coordinator.  Returns nil if unknown.
	Node(id uuid.UUID) *Node
	Close() error
	Coordinate(ctx context.Context, id uuid.UUID, name string, a TunnelAuth) (chan<- *proto.CoordinateRequest, <-chan *proto.CoordinateResponse)
}

// Node represents a node in the network.
type Node struct {
	// ID is used to identify the connection.
	ID tailcfg.NodeID `json:"id"`
	// AsOf is the time the node was created.
	AsOf time.Time `json:"as_of"`
	// Key is the Wireguard public key of the node.
	Key key.NodePublic `json:"key"`
	// DiscoKey is used for discovery messages over DERP to establish
	// peer-to-peer connections.
	DiscoKey key.DiscoPublic `json:"disco"`
	// PreferredDERP is the DERP server that peered connections should meet at
	// to establish.
	PreferredDERP int `json:"preferred_derp"`
	// DERPLatency is the latency in seconds to each DERP server.
	DERPLatency map[string]float64 `json:"derp_latency"`
	// DERPForcedWebsocket contains a mapping of DERP regions to
	// error messages that caused the connection to be forced to
	// use WebSockets. We don't use WebSockets by default because
	// they are less performant.
	DERPForcedWebsocket map[int]string `json:"derp_forced_websockets"`
	// Addresses are the IP address ranges this connection exposes.
	Addresses []netip.Prefix `json:"addresses"`
	// AllowedIPs specify what addresses can dial the connection. We allow all
	// by default.
	AllowedIPs []netip.Prefix `json:"allowed_ips"`
	// Endpoints are ip:port combinations that can be used to establish
	// peer-to-peer connections.
	Endpoints []string `json:"endpoints"`
}

// Coordinatee is something that can be coordinated over the Coordinate protocol.  Usually this is a
// Conn.
type Coordinatee interface {
	UpdatePeers([]*proto.CoordinateResponse_PeerUpdate) error
	SetAllPeersLost()
	SetNodeCallback(func(*Node))
}

type Coordination interface {
	io.Closer
	Error() <-chan error
}

type remoteCoordination struct {
	sync.Mutex
	closed       bool
	errChan      chan error
	coordinatee  Coordinatee
	logger       slog.Logger
	protocol     proto.DRPCTailnet_CoordinateClient
	respLoopDone chan struct{}
}

func (c *remoteCoordination) Close() (retErr error) {
	c.Lock()
	defer c.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	defer func() {
		protoErr := c.protocol.Close()
		<-c.respLoopDone
		if retErr == nil {
			retErr = protoErr
		}
	}()
	err := c.protocol.Send(&proto.CoordinateRequest{Disconnect: &proto.CoordinateRequest_Disconnect{}})
	if err != nil && !xerrors.Is(err, io.EOF) {
		// Coordinator RPC hangs up when it gets disconnect, so EOF is expected.
		return xerrors.Errorf("send disconnect: %w", err)
	}
	c.logger.Debug(context.Background(), "sent disconnect")
	return nil
}

func (c *remoteCoordination) Error() <-chan error {
	return c.errChan
}

func (c *remoteCoordination) sendErr(err error) {
	select {
	case c.errChan <- err:
	default:
	}
}

func (c *remoteCoordination) respLoop() {
	defer func() {
		c.coordinatee.SetAllPeersLost()
		close(c.respLoopDone)
	}()
	for {
		resp, err := c.protocol.Recv()
		if err != nil {
			c.sendErr(xerrors.Errorf("read: %w", err))
			return
		}
		err = c.coordinatee.UpdatePeers(resp.GetPeerUpdates())
		if err != nil {
			c.sendErr(xerrors.Errorf("update peers: %w", err))
			return
		}
	}
}

// NewRemoteCoordination uses the provided protocol to coordinate the provided coordinatee (usually a
// Conn).  If the tunnelTarget is not uuid.Nil, then we add a tunnel to the peer (i.e. we are acting as
// a client---agents should NOT set this!).
func NewRemoteCoordination(logger slog.Logger,
	protocol proto.DRPCTailnet_CoordinateClient, coordinatee Coordinatee,
	tunnelTarget uuid.UUID,
) Coordination {
	c := &remoteCoordination{
		errChan:      make(chan error, 1),
		coordinatee:  coordinatee,
		logger:       logger,
		protocol:     protocol,
		respLoopDone: make(chan struct{}),
	}
	if tunnelTarget != uuid.Nil {
		c.Lock()
		err := c.protocol.Send(&proto.CoordinateRequest{AddTunnel: &proto.CoordinateRequest_Tunnel{Id: tunnelTarget[:]}})
		c.Unlock()
		if err != nil {
			c.sendErr(err)
		}
	}

	coordinatee.SetNodeCallback(func(node *Node) {
		pn, err := NodeToProto(node)
		if err != nil {
			c.logger.Critical(context.Background(), "failed to convert node", slog.Error(err))
			c.sendErr(err)
			return
		}
		c.Lock()
		defer c.Unlock()
		if c.closed {
			c.logger.Debug(context.Background(), "ignored node update because coordination is closed")
			return
		}
		err = c.protocol.Send(&proto.CoordinateRequest{UpdateSelf: &proto.CoordinateRequest_UpdateSelf{Node: pn}})
		if err != nil {
			c.sendErr(xerrors.Errorf("write: %w", err))
		}
	})
	go c.respLoop()
	return c
}

type inMemoryCoordination struct {
	sync.Mutex
	ctx          context.Context
	errChan      chan error
	closed       bool
	closedCh     chan struct{}
	respLoopDone chan struct{}
	coordinatee  Coordinatee
	logger       slog.Logger
	resps        <-chan *proto.CoordinateResponse
	reqs         chan<- *proto.CoordinateRequest
}

func (c *inMemoryCoordination) sendErr(err error) {
	select {
	case c.errChan <- err:
	default:
	}
}

func (c *inMemoryCoordination) Error() <-chan error {
	return c.errChan
}

// NewInMemoryCoordination connects a Coordinatee (usually Conn) to an in memory Coordinator, for testing
// or local clients.  Set ClientID to uuid.Nil for an agent.
func NewInMemoryCoordination(
	ctx context.Context, logger slog.Logger,
	clientID, agentID uuid.UUID,
	coordinator Coordinator, coordinatee Coordinatee,
) Coordination {
	thisID := agentID
	logger = logger.With(slog.F("agent_id", agentID))
	var auth TunnelAuth = AgentTunnelAuth{}
	if clientID != uuid.Nil {
		// this is a client connection
		auth = ClientTunnelAuth{AgentID: agentID}
		logger = logger.With(slog.F("client_id", clientID))
		thisID = clientID
	}
	c := &inMemoryCoordination{
		ctx:          ctx,
		errChan:      make(chan error, 1),
		coordinatee:  coordinatee,
		logger:       logger,
		closedCh:     make(chan struct{}),
		respLoopDone: make(chan struct{}),
	}

	// use the background context since we will depend exclusively on closing the req channel to
	// tell the coordinator we are done.
	c.reqs, c.resps = coordinator.Coordinate(context.Background(),
		thisID, fmt.Sprintf("inmemory%s", thisID),
		auth,
	)
	go c.respLoop()
	if agentID != uuid.Nil {
		select {
		case <-ctx.Done():
			c.logger.Warn(ctx, "context expired before we could add tunnel", slog.Error(ctx.Err()))
			return c
		case c.reqs <- &proto.CoordinateRequest{AddTunnel: &proto.CoordinateRequest_Tunnel{Id: agentID[:]}}:
			// OK!
		}
	}
	coordinatee.SetNodeCallback(func(n *Node) {
		pn, err := NodeToProto(n)
		if err != nil {
			c.logger.Critical(ctx, "failed to convert node", slog.Error(err))
			c.sendErr(err)
			return
		}
		c.Lock()
		defer c.Unlock()
		if c.closed {
			return
		}
		select {
		case <-ctx.Done():
			c.logger.Info(ctx, "context expired before sending node update")
			return
		case c.reqs <- &proto.CoordinateRequest{UpdateSelf: &proto.CoordinateRequest_UpdateSelf{Node: pn}}:
			c.logger.Debug(ctx, "sent node in-memory to coordinator")
		}
	})
	return c
}

func (c *inMemoryCoordination) respLoop() {
	defer func() {
		c.coordinatee.SetAllPeersLost()
		close(c.respLoopDone)
	}()
	for {
		select {
		case <-c.closedCh:
			c.logger.Debug(context.Background(), "in-memory coordination closed")
			return
		case resp, ok := <-c.resps:
			if !ok {
				c.logger.Debug(context.Background(), "in-memory response channel closed")
				return
			}
			c.logger.Debug(context.Background(), "got in-memory response from coordinator", slog.F("resp", resp))
			err := c.coordinatee.UpdatePeers(resp.GetPeerUpdates())
			if err != nil {
				c.sendErr(xerrors.Errorf("failed to update peers: %w", err))
				return
			}
		}
	}
}

func (c *inMemoryCoordination) Close() error {
	c.Lock()
	defer c.Unlock()
	c.logger.Debug(context.Background(), "closing in-memory coordination")
	if c.closed {
		return nil
	}
	defer close(c.reqs)
	c.closed = true
	close(c.closedCh)
	<-c.respLoopDone
	select {
	case <-c.ctx.Done():
		return xerrors.Errorf("failed to gracefully disconnect: %w", c.ctx.Err())
	case c.reqs <- &proto.CoordinateRequest{Disconnect: &proto.CoordinateRequest_Disconnect{}}:
		c.logger.Debug(context.Background(), "sent graceful disconnect in-memory")
		return nil
	}
}

// ServeCoordinator matches the RW structure of a coordinator to exchange node messages.
func ServeCoordinator(conn net.Conn, updateNodes func(node []*Node) error) (func(node *Node), <-chan error) {
	errChan := make(chan error, 1)
	sendErr := func(err error) {
		select {
		case errChan <- err:
		default:
		}
	}
	go func() {
		decoder := json.NewDecoder(conn)
		for {
			var nodes []*Node
			err := decoder.Decode(&nodes)
			if err != nil {
				sendErr(xerrors.Errorf("read: %w", err))
				return
			}
			err = updateNodes(nodes)
			if err != nil {
				sendErr(xerrors.Errorf("update nodes: %w", err))
			}
		}
	}()

	return func(node *Node) {
		data, err := json.Marshal(node)
		if err != nil {
			sendErr(xerrors.Errorf("marshal node: %w", err))
			return
		}
		_, err = conn.Write(data)
		if err != nil {
			sendErr(xerrors.Errorf("write: %w", err))
		}
	}, errChan
}

const LoggerName = "coord"

var (
	ErrClosed         = xerrors.New("coordinator is closed")
	ErrWouldBlock     = xerrors.New("would block")
	ErrAlreadyRemoved = xerrors.New("already removed")
)

// NewCoordinator constructs a new in-memory connection coordinator. This
// coordinator is incompatible with multiple Coder replicas as all node data is
// in-memory.
func NewCoordinator(logger slog.Logger) Coordinator {
	return &coordinator{
		core:       newCore(logger.Named(LoggerName)),
		closedChan: make(chan struct{}),
	}
}

// coordinator exchanges nodes with agents to establish connections entirely in-memory.
// The Enterprise implementation provides this for high-availability.
// ┌──────────────────┐   ┌────────────────────┐   ┌───────────────────┐   ┌──────────────────┐
// │tailnet.Coordinate├──►│tailnet.AcceptClient│◄─►│tailnet.AcceptAgent│◄──┤tailnet.Coordinate│
// └──────────────────┘   └────────────────────┘   └───────────────────┘   └──────────────────┘
// This coordinator is incompatible with multiple Coder replicas as all node
// data is in-memory.
type coordinator struct {
	core *core

	mu         sync.Mutex
	closed     bool
	wg         sync.WaitGroup
	closedChan chan struct{}
}

func (c *coordinator) Coordinate(
	ctx context.Context, id uuid.UUID, name string, a TunnelAuth,
) (
	chan<- *proto.CoordinateRequest, <-chan *proto.CoordinateResponse,
) {
	logger := c.core.logger.With(
		slog.F("peer_id", id),
		slog.F("peer_name", name),
	)
	reqs := make(chan *proto.CoordinateRequest, RequestBufferSize)
	resps := make(chan *proto.CoordinateResponse, ResponseBufferSize)

	p := &peer{
		logger: logger,
		id:     id,
		name:   name,
		resps:  resps,
		reqs:   reqs,
		auth:   a,
		sent:   make(map[uuid.UUID]*proto.Node),
	}
	err := c.core.initPeer(p)
	if err != nil {
		if xerrors.Is(err, ErrClosed) {
			logger.Debug(ctx, "coordinate failed: Coordinator is closed")
		} else {
			logger.Critical(ctx, "coordinate failed: %s", err.Error())
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		// don't start the readLoop if we are closed.
		return reqs, resps
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		p.reqLoop(ctx, logger, c.core.handleRequest)
		err := c.core.lostPeer(p)
		if xerrors.Is(err, ErrClosed) || xerrors.Is(err, ErrAlreadyRemoved) {
			return
		}
		if err != nil {
			logger.Error(context.Background(), "failed to process lost peer", slog.Error(err))
		}
	}()
	return reqs, resps
}

func (c *coordinator) ServeMultiAgent(id uuid.UUID) MultiAgentConn {
	return ServeMultiAgent(c, c.core.logger, id)
}

func ServeMultiAgent(c CoordinatorV2, logger slog.Logger, id uuid.UUID) MultiAgentConn {
	logger = logger.With(slog.F("client_id", id)).Named("multiagent")
	ctx, cancel := context.WithCancel(context.Background())
	reqs, resps := c.Coordinate(ctx, id, id.String(), SingleTailnetTunnelAuth{})
	m := (&MultiAgent{
		ID: id,
		OnSubscribe: func(enq Queue, agent uuid.UUID) error {
			err := SendCtx(ctx, reqs, &proto.CoordinateRequest{AddTunnel: &proto.CoordinateRequest_Tunnel{Id: UUIDToByteSlice(agent)}})
			return err
		},
		OnUnsubscribe: func(enq Queue, agent uuid.UUID) error {
			err := SendCtx(ctx, reqs, &proto.CoordinateRequest{RemoveTunnel: &proto.CoordinateRequest_Tunnel{Id: UUIDToByteSlice(agent)}})
			return err
		},
		OnNodeUpdate: func(id uuid.UUID, node *proto.Node) error {
			return SendCtx(ctx, reqs, &proto.CoordinateRequest{UpdateSelf: &proto.CoordinateRequest_UpdateSelf{
				Node: node,
			}})
		},
		OnRemove: func(_ Queue) {
			_ = SendCtx(ctx, reqs, &proto.CoordinateRequest{Disconnect: &proto.CoordinateRequest_Disconnect{}})
			cancel()
		},
	}).Init()

	go v1RespLoop(ctx, cancel, logger, m, resps)
	return m
}

// core is an in-memory structure of Node and TrackedConn mappings.  Its methods may be called from multiple goroutines;
// it is protected by a mutex to ensure data stay consistent.
type core struct {
	logger slog.Logger
	mutex  sync.RWMutex
	closed bool

	peers   map[uuid.UUID]*peer
	tunnels *tunnelStore
}

type QueueKind int

const (
	QueueKindClient QueueKind = 1 + iota
	QueueKindAgent
)

type Queue interface {
	UniqueID() uuid.UUID
	Kind() QueueKind
	Enqueue(resp *proto.CoordinateResponse) error
	Name() string
	Stats() (start, lastWrite int64)
	Overwrites() int64
	// CoordinatorClose is used by the coordinator when closing a Queue. It
	// should skip removing itself from the coordinator.
	CoordinatorClose() error
	Done() <-chan struct{}
	Close() error
}

func newCore(logger slog.Logger) *core {
	return &core{
		logger:  logger,
		closed:  false,
		peers:   make(map[uuid.UUID]*peer),
		tunnels: newTunnelStore(),
	}
}

// Node returns an in-memory node by ID.
// If the node does not exist, nil is returned.
func (c *coordinator) Node(id uuid.UUID) *Node {
	return c.core.node(id)
}

func (c *core) node(id uuid.UUID) *Node {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	p := c.peers[id]
	if p == nil || p.node == nil {
		return nil
	}
	v1Node, err := ProtoToNode(p.node)
	if err != nil {
		c.logger.Critical(context.Background(),
			"failed to convert node", slog.Error(err), slog.F("node", p.node))
		return nil
	}
	return v1Node
}

// ServeClient accepts a WebSocket connection that wants to connect to an agent
// with the specified ID.
func (c *coordinator) ServeClient(conn net.Conn, id, agentID uuid.UUID) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return ServeClientV1(ctx, c.core.logger, c, conn, id, agentID)
}

// ServeClientV1 adapts a v1 Client to a v2 Coordinator
func ServeClientV1(ctx context.Context, logger slog.Logger, c CoordinatorV2, conn net.Conn, id uuid.UUID, agent uuid.UUID) error {
	logger = logger.With(slog.F("client_id", id), slog.F("agent_id", agent))
	defer func() {
		err := conn.Close()
		if err != nil {
			logger.Debug(ctx, "closing client connection", slog.Error(err))
		}
	}()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	reqs, resps := c.Coordinate(ctx, id, id.String(), ClientTunnelAuth{AgentID: agent})
	err := SendCtx(ctx, reqs, &proto.CoordinateRequest{
		AddTunnel: &proto.CoordinateRequest_Tunnel{Id: UUIDToByteSlice(agent)},
	})
	if err != nil {
		// can only be a context error, no need to log here.
		return err
	}

	tc := NewTrackedConn(ctx, cancel, conn, id, logger, id.String(), 0, QueueKindClient)
	go tc.SendUpdates()
	go v1RespLoop(ctx, cancel, logger, tc, resps)
	go v1ReqLoop(ctx, cancel, logger, conn, reqs)
	<-ctx.Done()
	return nil
}

func (c *core) handleRequest(p *peer, req *proto.CoordinateRequest) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.closed {
		return ErrClosed
	}
	pr, ok := c.peers[p.id]
	if !ok || pr != p {
		return ErrAlreadyRemoved
	}
	if req.UpdateSelf != nil {
		err := c.nodeUpdateLocked(p, req.UpdateSelf.Node)
		if xerrors.Is(err, ErrAlreadyRemoved) || xerrors.Is(err, ErrClosed) {
			return nil
		}
		if err != nil {
			return xerrors.Errorf("node update failed: %w", err)
		}
	}
	if req.AddTunnel != nil {
		dstID, err := uuid.FromBytes(req.AddTunnel.Id)
		if err != nil {
			// this shouldn't happen unless there is a client error.  Close the connection so the client
			// doesn't just happily continue thinking everything is fine.
			return xerrors.Errorf("unable to convert bytes to UUID: %w", err)
		}
		err = c.addTunnelLocked(p, dstID)
		if xerrors.Is(err, ErrAlreadyRemoved) || xerrors.Is(err, ErrClosed) {
			return nil
		}
		if err != nil {
			return xerrors.Errorf("add tunnel failed: %w", err)
		}
	}
	if req.RemoveTunnel != nil {
		dstID, err := uuid.FromBytes(req.RemoveTunnel.Id)
		if err != nil {
			// this shouldn't happen unless there is a client error.  Close the connection so the client
			// doesn't just happily continue thinking everything is fine.
			return xerrors.Errorf("unable to convert bytes to UUID: %w", err)
		}
		err = c.removeTunnelLocked(p, dstID)
		if xerrors.Is(err, ErrAlreadyRemoved) || xerrors.Is(err, ErrClosed) {
			return nil
		}
		if err != nil {
			return xerrors.Errorf("remove tunnel failed: %w", err)
		}
	}
	if req.Disconnect != nil {
		c.removePeerLocked(p.id, proto.CoordinateResponse_PeerUpdate_DISCONNECTED, "graceful disconnect")
	}
	return nil
}

func (c *core) nodeUpdateLocked(p *peer, node *proto.Node) error {
	c.logger.Debug(context.Background(), "processing node update",
		slog.F("peer_id", p.id),
		slog.F("node", node.String()))

	p.node = node
	c.updateTunnelPeersLocked(p.id, node, proto.CoordinateResponse_PeerUpdate_NODE, "node update")
	return nil
}

func (c *core) updateTunnelPeersLocked(id uuid.UUID, n *proto.Node, k proto.CoordinateResponse_PeerUpdate_Kind, reason string) {
	tp := c.tunnels.findTunnelPeers(id)
	c.logger.Debug(context.Background(), "got tunnel peers", slog.F("peer_id", id), slog.F("tunnel_peers", tp))
	for _, otherID := range tp {
		other, ok := c.peers[otherID]
		if !ok {
			continue
		}
		err := other.updateMappingLocked(id, n, k, reason)
		if err != nil {
			other.logger.Error(context.Background(), "failed to update mapping", slog.Error(err))
			c.removePeerLocked(other.id, proto.CoordinateResponse_PeerUpdate_DISCONNECTED, "failed update")
		}
	}
}

func (c *core) addTunnelLocked(src *peer, dstID uuid.UUID) error {
	if !src.auth.Authorize(dstID) {
		return xerrors.Errorf("src %s is not allowed to tunnel to %s", src.id, dstID)
	}
	c.tunnels.add(src.id, dstID)
	c.logger.Debug(context.Background(), "adding tunnel",
		slog.F("src_id", src.id),
		slog.F("dst_id", dstID))
	dst, ok := c.peers[dstID]
	if ok {
		if dst.node != nil {
			err := src.updateMappingLocked(dstID, dst.node, proto.CoordinateResponse_PeerUpdate_NODE, "add tunnel")
			if err != nil {
				src.logger.Error(context.Background(), "failed update of tunnel src", slog.Error(err))
				c.removePeerLocked(src.id, proto.CoordinateResponse_PeerUpdate_DISCONNECTED, "failed update")
				// if the source fails, then the tunnel is also removed and there is no reason to continue
				// processing.
				return err
			}
		}
		if src.node != nil {
			err := dst.updateMappingLocked(src.id, src.node, proto.CoordinateResponse_PeerUpdate_NODE, "add tunnel")
			if err != nil {
				dst.logger.Error(context.Background(), "failed update of tunnel dst", slog.Error(err))
				c.removePeerLocked(dst.id, proto.CoordinateResponse_PeerUpdate_DISCONNECTED, "failed update")
			}
		}
	}
	return nil
}

func (c *core) removeTunnelLocked(src *peer, dstID uuid.UUID) error {
	err := src.updateMappingLocked(dstID, nil, proto.CoordinateResponse_PeerUpdate_DISCONNECTED, "remove tunnel")
	if err != nil {
		src.logger.Error(context.Background(), "failed to update", slog.Error(err))
		c.removePeerLocked(src.id, proto.CoordinateResponse_PeerUpdate_DISCONNECTED, "failed update")
		// removing the peer also removes all other tunnels and notifies destinations, so it's safe to
		// return here.
		return err
	}
	dst, ok := c.peers[dstID]
	if ok {
		err = dst.updateMappingLocked(src.id, nil, proto.CoordinateResponse_PeerUpdate_DISCONNECTED, "remove tunnel")
		if err != nil {
			dst.logger.Error(context.Background(), "failed to update", slog.Error(err))
			c.removePeerLocked(dst.id, proto.CoordinateResponse_PeerUpdate_DISCONNECTED, "failed update")
			// don't return here because we still want to remove the tunnel, and an error at the
			// destination doesn't count as an error removing the tunnel at the source.
		}
	}
	c.tunnels.remove(src.id, dstID)
	return nil
}

func (c *core) initPeer(p *peer) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	p.logger.Debug(context.Background(), "initPeer")
	if c.closed {
		return ErrClosed
	}
	if p.node != nil {
		return xerrors.Errorf("peer (%s) must be initialized with nil node", p.id)
	}
	if old, ok := c.peers[p.id]; ok {
		// rare and interesting enough to log at Info, but it isn't an error per se
		old.logger.Info(context.Background(), "overwritten by new connection")
		close(old.resps)
		p.overwrites = old.overwrites + 1
	}
	now := time.Now()
	p.start = now
	p.lastWrite = now
	c.peers[p.id] = p

	tp := c.tunnels.findTunnelPeers(p.id)
	p.logger.Debug(context.Background(), "initial tunnel peers", slog.F("tunnel_peers", tp))
	var others []*peer
	for _, otherID := range tp {
		// ok to append nil here because the batch call below filters them out
		others = append(others, c.peers[otherID])
	}
	return p.batchUpdateMappingLocked(others, proto.CoordinateResponse_PeerUpdate_NODE, "init")
}

// removePeer removes and cleans up a lost peer.  It updates all peers it shares a tunnel with, deletes
// all tunnels from which the removed peer is the source.
func (c *core) lostPeer(p *peer) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.logger.Debug(context.Background(), "lostPeer", slog.F("peer_id", p.id))
	if c.closed {
		return ErrClosed
	}
	if existing, ok := c.peers[p.id]; !ok || existing != p {
		return ErrAlreadyRemoved
	}
	c.removePeerLocked(p.id, proto.CoordinateResponse_PeerUpdate_LOST, "lost")
	return nil
}

func (c *core) removePeerLocked(id uuid.UUID, kind proto.CoordinateResponse_PeerUpdate_Kind, reason string) {
	p, ok := c.peers[id]
	if !ok {
		c.logger.Critical(context.Background(), "removed non-existent peer %s", id)
		return
	}
	c.updateTunnelPeersLocked(id, nil, kind, reason)
	c.tunnels.removeAll(id)
	close(p.resps)
	delete(c.peers, id)
}

// ServeAgent accepts a WebSocket connection to an agent that
// listens to incoming connections and publishes node updates.
func (c *coordinator) ServeAgent(conn net.Conn, id uuid.UUID, name string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return ServeAgentV1(ctx, c.core.logger, c, conn, id, name)
}

func ServeAgentV1(ctx context.Context, logger slog.Logger, c CoordinatorV2, conn net.Conn, id uuid.UUID, name string) error {
	logger = logger.With(slog.F("agent_id", id), slog.F("name", name))
	defer func() {
		logger.Debug(ctx, "closing agent connection")
		err := conn.Close()
		logger.Debug(ctx, "closed agent connection", slog.Error(err))
	}()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	logger.Debug(ctx, "starting new agent connection")
	reqs, resps := c.Coordinate(ctx, id, name, AgentTunnelAuth{})
	tc := NewTrackedConn(ctx, cancel, conn, id, logger, name, 0, QueueKindAgent)
	go tc.SendUpdates()
	go v1RespLoop(ctx, cancel, logger, tc, resps)
	go v1ReqLoop(ctx, cancel, logger, conn, reqs)
	<-ctx.Done()
	logger.Debug(ctx, "ending agent connection")
	return nil
}

// Close closes all of the open connections in the coordinator and stops the
// coordinator from accepting new connections.
func (c *coordinator) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	close(c.closedChan)
	c.mu.Unlock()

	err := c.core.close()
	// wait for all request loops to complete
	c.wg.Wait()
	return err
}

func (c *core) close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	for id := range c.peers {
		// when closing, mark them as LOST so that we don't disrupt in-progress
		// connections.
		c.removePeerLocked(id, proto.CoordinateResponse_PeerUpdate_LOST, "coordinator close")
	}
	return nil
}

func (c *coordinator) ServeHTTPDebug(w http.ResponseWriter, r *http.Request) {
	c.core.serveHTTPDebug(w, r)
}

func (c *core) serveHTTPDebug(w http.ResponseWriter, _ *http.Request) {
	debug := c.getHTMLDebug()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err := debugTempl.Execute(w, debug)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
}

func (c *core) getHTMLDebug() HTMLDebug {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	debug := HTMLDebug{Tunnels: c.tunnels.htmlDebug()}
	for _, p := range c.peers {
		debug.Peers = append(debug.Peers, p.htmlDebug())
	}
	return debug
}

type HTMLDebug struct {
	Peers   []HTMLPeer
	Tunnels []HTMLTunnel
}

type HTMLPeer struct {
	ID           uuid.UUID
	Name         string
	CreatedAge   time.Duration
	LastWriteAge time.Duration
	Overwrites   int
	Node         string
}

type HTMLTunnel struct {
	Src, Dst uuid.UUID
}

var coordinatorDebugTmpl = `
<!DOCTYPE html>
<html>
	<head>
		<meta charset="UTF-8">
		<style>
th, td {
  padding-top: 6px;
  padding-bottom: 6px;
  padding-left: 10px;
  padding-right: 10px;
  text-align: left;
}
tr {
  border-bottom: 1px solid #ddd;
}
		</style>
	</head>
	<body>
		<h1>in-memory wireguard coordinator debug</h1>

		<h2 id=peers> <a href=#peers>#</a> peers: total {{ len .Peers }} </h2>
		<table>
			<tr style="margin-top:4px">
				<th>Name</th>
				<th>ID</th>
				<th>Created Age</th>
				<th>Last Write Age</th>
				<th>Overwrites</th>
				<th>Node</th>
			</tr>
		{{- range .Peers }}
			<tr style="margin-top:4px">
				<td>{{ .Name }}</td>
				<td>{{ .ID }}</td>
				<td>{{ .CreatedAge }}</td>
				<td>{{ .LastWriteAge }} ago</td>
				<td>{{ .Overwrites }}</td>
				<td style="white-space: pre;"><code>{{ .Node }}</code></td>
			</tr>
		{{- end }}
		</table>

		<h2 id=tunnels><a href=#tunnels>#</a> tunnels: total {{ len .Tunnels }}</h2>
		<table>
			<tr style="margin-top:4px">
				<th>SrcID</th>
				<th>DstID</th>
			</tr>
		{{- range .Tunnels }}
			<tr style="margin-top:4px">
				<td>{{ .Src }}</td>
				<td>{{ .Dst }}</td>
			</tr>
		{{- end }}
		</table>
	</body>
</html>
`
var debugTempl = template.Must(template.New("coordinator_debug").Parse(coordinatorDebugTmpl))

func SendCtx[A any](ctx context.Context, c chan<- A, a A) (err error) {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case c <- a:
		return nil
	}
}

func RecvCtx[A any](ctx context.Context, c <-chan A) (a A, err error) {
	select {
	case <-ctx.Done():
		return a, ctx.Err()
	case a, ok := <-c:
		if ok {
			return a, nil
		}
		return a, io.EOF
	}
}

func v1ReqLoop(ctx context.Context, cancel context.CancelFunc, logger slog.Logger,
	conn net.Conn, reqs chan<- *proto.CoordinateRequest,
) {
	defer close(reqs)
	defer cancel()
	decoder := json.NewDecoder(conn)
	for {
		var node Node
		err := decoder.Decode(&node)
		if err != nil {
			if xerrors.Is(err, io.EOF) ||
				xerrors.Is(err, io.ErrClosedPipe) ||
				xerrors.Is(err, context.Canceled) ||
				xerrors.Is(err, context.DeadlineExceeded) ||
				websocket.CloseStatus(err) > 0 {
				logger.Debug(ctx, "v1ReqLoop exiting", slog.Error(err))
			} else {
				logger.Info(ctx, "v1ReqLoop failed to decode Node update", slog.Error(err))
			}
			return
		}
		logger.Debug(ctx, "v1ReqLoop got node update", slog.F("node", node))
		pn, err := NodeToProto(&node)
		if err != nil {
			logger.Critical(ctx, "v1ReqLoop failed to convert v1 node", slog.F("node", node), slog.Error(err))
			return
		}
		req := &proto.CoordinateRequest{UpdateSelf: &proto.CoordinateRequest_UpdateSelf{
			Node: pn,
		}}
		if err := SendCtx(ctx, reqs, req); err != nil {
			logger.Debug(ctx, "v1ReqLoop ctx expired", slog.Error(err))
			return
		}
	}
}

func v1RespLoop(ctx context.Context, cancel context.CancelFunc, logger slog.Logger, q Queue, resps <-chan *proto.CoordinateResponse) {
	defer func() {
		cErr := q.Close()
		if cErr != nil {
			logger.Info(ctx, "error closing response Queue", slog.Error(cErr))
		}
		cancel()
	}()
	for {
		resp, err := RecvCtx(ctx, resps)
		if err != nil {
			logger.Debug(ctx, "v1RespLoop done reading responses", slog.Error(err))
			return
		}
		logger.Debug(ctx, "v1RespLoop got response", slog.F("resp", resp))
		err = q.Enqueue(resp)
		if err != nil && !xerrors.Is(err, context.Canceled) {
			logger.Error(ctx, "v1RespLoop failed to enqueue v1 update", slog.Error(err))
		}
	}
}
