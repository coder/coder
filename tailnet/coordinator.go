package tailnet

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/google/uuid"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/tailnet/proto"
)

const (
	// ResponseBufferSize is the max number of responses to buffer per connection before we start
	// dropping updates
	ResponseBufferSize = 512
	// RequestBufferSize is the max number of requests to buffer per connection
	RequestBufferSize = 32
)

// Coordinator exchanges nodes with agents to establish connections.
// ┌──────────────────┐   ┌────────────────────┐   ┌───────────────────┐   ┌──────────────────┐
// │tailnet.Coordinate├──►│tailnet.AcceptClient│◄─►│tailnet.AcceptAgent│◄──┤tailnet.Coordinate│
// └──────────────────┘   └────────────────────┘   └───────────────────┘   └──────────────────┘
// Coordinators have different guarantees for HA support.
type Coordinator interface {
	CoordinatorV2
}

// CoordinatorV2 is the interface for interacting with the coordinator via the 2.0 tailnet API.
type CoordinatorV2 interface {
	// ServeHTTPDebug serves a debug webpage that shows the internal state of
	// the coordinator.
	ServeHTTPDebug(w http.ResponseWriter, r *http.Request)
	// Node returns a node by peer ID, if known to the coordinator.  Returns nil if unknown.
	Node(id uuid.UUID) *Node
	Close() error
	Coordinate(ctx context.Context, id uuid.UUID, name string, a CoordinateeAuth) (chan<- *proto.CoordinateRequest, <-chan *proto.CoordinateResponse)
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
	// SetTunnelDestination indicates to tailnet that the peer id is a
	// destination.
	SetTunnelDestination(id uuid.UUID)
}

const LoggerName = "coord"

var (
	ErrClosed         = errors.New("coordinator is closed")
	ErrWouldBlock     = errors.New("would block")
	ErrAlreadyRemoved = errors.New("already removed")
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
	ctx context.Context, id uuid.UUID, name string, a CoordinateeAuth,
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
		if errors.Is(err, ErrClosed) {
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
		if errors.Is(err, ErrClosed) || errors.Is(err, ErrAlreadyRemoved) {
			return
		}
		if err != nil {
			logger.Error(context.Background(), "failed to process lost peer", slog.Error(err))
		}
	}()
	return reqs, resps
}

// core is an in-memory structure of peer mappings.  Its methods may be called from multiple goroutines;
// it is protected by a mutex to ensure data stay consistent.
type core struct {
	logger slog.Logger
	mutex  sync.RWMutex
	closed bool

	peers   map[uuid.UUID]*peer
	tunnels *tunnelStore
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

func (c *core) handleRequest(ctx context.Context, p *peer, req *proto.CoordinateRequest) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.closed {
		return ErrClosed
	}
	pr, ok := c.peers[p.id]
	if !ok || pr != p {
		return ErrAlreadyRemoved
	}

	if err := pr.auth.Authorize(ctx, req); err != nil {
		return fmt.Errorf("authorize request: %w", err)
	}

	if req.UpdateSelf != nil {
		err := c.nodeUpdateLocked(p, req.UpdateSelf.Node)
		if errors.Is(err, ErrAlreadyRemoved) || errors.Is(err, ErrClosed) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("node update failed: %w", err)
		}
	}
	if req.AddTunnel != nil {
		dstID, err := uuid.FromBytes(req.AddTunnel.Id)
		if err != nil {
			// this shouldn't happen unless there is a client error.  Close the connection so the client
			// doesn't just happily continue thinking everything is fine.
			return fmt.Errorf("unable to convert bytes to UUID: %w", err)
		}
		err = c.addTunnelLocked(p, dstID)
		if errors.Is(err, ErrAlreadyRemoved) || errors.Is(err, ErrClosed) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("add tunnel failed: %w", err)
		}
	}
	if req.RemoveTunnel != nil {
		dstID, err := uuid.FromBytes(req.RemoveTunnel.Id)
		if err != nil {
			// this shouldn't happen unless there is a client error.  Close the connection so the client
			// doesn't just happily continue thinking everything is fine.
			return fmt.Errorf("unable to convert bytes to UUID: %w", err)
		}
		err = c.removeTunnelLocked(p, dstID)
		if errors.Is(err, ErrAlreadyRemoved) || errors.Is(err, ErrClosed) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("remove tunnel failed: %w", err)
		}
	}
	if req.Disconnect != nil {
		c.removePeerLocked(p.id, proto.CoordinateResponse_PeerUpdate_DISCONNECTED, "graceful disconnect")
	}
	if rfhs := req.ReadyForHandshake; rfhs != nil {
		err := c.handleReadyForHandshakeLocked(pr, rfhs)
		if err != nil {
			return fmt.Errorf("handle ack: %w", err)
		}
	}
	return nil
}

func (c *core) handleReadyForHandshakeLocked(src *peer, rfhs []*proto.CoordinateRequest_ReadyForHandshake) error {
	for _, rfh := range rfhs {
		dstID, err := uuid.FromBytes(rfh.Id)
		if err != nil {
			// this shouldn't happen unless there is a client error.  Close the connection so the client
			// doesn't just happily continue thinking everything is fine.
			return fmt.Errorf("unable to convert bytes to UUID: %w", err)
		}

		if !c.tunnels.tunnelExists(src.id, dstID) {
			// We intentionally do not return an error here, since it's
			// inherently racy. It's possible for a source to connect, then
			// subsequently disconnect before the agent has sent back the RFH.
			// Since this could potentially happen to a non-malicious agent, we
			// don't want to kill its connection.
			select {
			case src.resps <- &proto.CoordinateResponse{
				Error: fmt.Sprintf("you do not share a tunnel with %q", dstID.String()),
			}:
			default:
				return ErrWouldBlock
			}
			continue
		}

		dst, ok := c.peers[dstID]
		if ok {
			select {
			case dst.resps <- &proto.CoordinateResponse{
				PeerUpdates: []*proto.CoordinateResponse_PeerUpdate{{
					Id:   src.id[:],
					Kind: proto.CoordinateResponse_PeerUpdate_READY_FOR_HANDSHAKE,
				}},
			}:
			default:
				return ErrWouldBlock
			}
		}
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
		return fmt.Errorf("peer (%s) must be initialized with nil node", p.id)
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
