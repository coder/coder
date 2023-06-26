package tailnet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"cdr.dev/slog"

	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
)

// Coordinator exchanges nodes with agents to establish connections.
// ┌──────────────────┐   ┌────────────────────┐   ┌───────────────────┐   ┌──────────────────┐
// │tailnet.Coordinate├──►│tailnet.AcceptClient│◄─►│tailnet.AcceptAgent│◄──┤tailnet.Coordinate│
// └──────────────────┘   └────────────────────┘   └───────────────────┘   └──────────────────┘
// Coordinators have different guarantees for HA support.
type Coordinator interface {
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

	SubscribeAgent(agentID uuid.UUID, cb func(agentID uuid.UUID, node *Node)) func()
	BroadcastToAgents(agents []uuid.UUID, node *Node) error
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

// NewCoordinator constructs a new in-memory connection coordinator. This
// coordinator is incompatible with multiple Coder replicas as all node data is
// in-memory.
func NewCoordinator(logger slog.Logger) Coordinator {
	return &coordinator{
		core: newCore(logger),
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
}

// core is an in-memory structure of Node and TrackedConn mappings.  Its methods may be called from multiple goroutines;
// it is protected by a mutex to ensure data stay consistent.
type core struct {
	logger slog.Logger
	mutex  sync.RWMutex
	closed bool

	// nodes maps agent and connection IDs their respective node.
	nodes map[uuid.UUID]*Node
	// agentSockets maps agent IDs to their open websocket.
	agentSockets map[uuid.UUID]*TrackedConn
	// agentToConnectionSockets maps agent IDs to connection IDs of conns that
	// are subscribed to updates for that agent.
	agentToConnectionSockets map[uuid.UUID]map[uuid.UUID]*TrackedConn

	// agentNameCache holds a cache of agent names. If one of them disappears,
	// it's helpful to have a name cached for debugging.
	agentNameCache *lru.Cache[uuid.UUID, string]

	agentCallbacks map[uuid.UUID]map[uuid.UUID]func(uuid.UUID, *Node)
}

func newCore(logger slog.Logger) *core {
	nameCache, err := lru.New[uuid.UUID, string](512)
	if err != nil {
		panic("make lru cache: " + err.Error())
	}

	return &core{
		logger:                   logger,
		closed:                   false,
		nodes:                    make(map[uuid.UUID]*Node),
		agentSockets:             map[uuid.UUID]*TrackedConn{},
		agentToConnectionSockets: map[uuid.UUID]map[uuid.UUID]*TrackedConn{},
		agentNameCache:           nameCache,
		agentCallbacks:           map[uuid.UUID]map[uuid.UUID]func(uuid.UUID, *Node){},
	}
}

var ErrWouldBlock = xerrors.New("would block")

type TrackedConn struct {
	ctx      context.Context
	cancel   func()
	conn     net.Conn
	updates  chan []*Node
	logger   slog.Logger
	lastData []byte

	// ID is an ephemeral UUID used to uniquely identify the owner of the
	// connection.
	ID uuid.UUID

	Name       string
	Start      int64
	LastWrite  int64
	Overwrites int64
}

func (t *TrackedConn) Enqueue(n []*Node) (err error) {
	atomic.StoreInt64(&t.LastWrite, time.Now().Unix())
	select {
	case t.updates <- n:
		return nil
	default:
		return ErrWouldBlock
	}
}

// Close the connection and cancel the context for reading node updates from the queue
func (t *TrackedConn) Close() error {
	t.cancel()
	return t.conn.Close()
}

// WriteTimeout is the amount of time we wait to write a node update to a connection before we declare it hung.
// It is exported so that tests can use it.
const WriteTimeout = time.Second * 5

// SendUpdates reads node updates and writes them to the connection.  Ends when writes hit an error or context is
// canceled.
func (t *TrackedConn) SendUpdates() {
	for {
		select {
		case <-t.ctx.Done():
			t.logger.Debug(t.ctx, "done sending updates")
			return
		case nodes := <-t.updates:
			data, err := json.Marshal(nodes)
			if err != nil {
				t.logger.Error(t.ctx, "unable to marshal nodes update", slog.Error(err), slog.F("nodes", nodes))
				return
			}
			if bytes.Equal(t.lastData, data) {
				t.logger.Debug(t.ctx, "skipping duplicate update", slog.F("nodes", nodes))
				continue
			}

			// Set a deadline so that hung connections don't put back pressure on the system.
			// Node updates are tiny, so even the dinkiest connection can handle them if it's not hung.
			err = t.conn.SetWriteDeadline(time.Now().Add(WriteTimeout))
			if err != nil {
				// often, this is just because the connection is closed/broken, so only log at debug.
				t.logger.Debug(t.ctx, "unable to set write deadline", slog.Error(err))
				_ = t.Close()
				return
			}
			_, err = t.conn.Write(data)
			if err != nil {
				// often, this is just because the connection is closed/broken, so only log at debug.
				t.logger.Debug(t.ctx, "could not write nodes to connection", slog.Error(err), slog.F("nodes", nodes))
				_ = t.Close()
				return
			}
			t.logger.Debug(t.ctx, "wrote nodes", slog.F("nodes", nodes))

			// nhooyr.io/websocket has a bugged implementation of deadlines on a websocket net.Conn.  What they are
			// *supposed* to do is set a deadline for any subsequent writes to complete, otherwise the call to Write()
			// fails.  What nhooyr.io/websocket does is set a timer, after which it expires the websocket write context.
			// If this timer fires, then the next write will fail *even if we set a new write deadline*.  So, after
			// our successful write, it is important that we reset the deadline before it fires.
			err = t.conn.SetWriteDeadline(time.Time{})
			if err != nil {
				// often, this is just because the connection is closed/broken, so only log at debug.
				t.logger.Debug(t.ctx, "unable to extend write deadline", slog.Error(err))
				_ = t.Close()
				return
			}
			t.lastData = data
		}
	}
}

func NewTrackedConn(ctx context.Context, cancel func(), conn net.Conn, id uuid.UUID, logger slog.Logger, overwrites int64) *TrackedConn {
	// buffer updates so they don't block, since we hold the
	// coordinator mutex while queuing.  Node updates don't
	// come quickly, so 512 should be plenty for all but
	// the most pathological cases.
	updates := make(chan []*Node, 512)
	now := time.Now().Unix()
	return &TrackedConn{
		ctx:        ctx,
		conn:       conn,
		cancel:     cancel,
		updates:    updates,
		logger:     logger,
		ID:         id,
		Start:      now,
		LastWrite:  now,
		Overwrites: overwrites,
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
	return c.nodes[id]
}

func (c *coordinator) NodeCount() int {
	return c.core.nodeCount()
}

func (c *core) nodeCount() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return len(c.nodes)
}

func (c *coordinator) AgentCount() int {
	return c.core.agentCount()
}

func (c *core) agentCount() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return len(c.agentSockets)
}

// ServeClient accepts a WebSocket connection that wants to connect to an agent
// with the specified ID.
func (c *coordinator) ServeClient(conn net.Conn, id uuid.UUID, agent uuid.UUID) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := c.core.clientLogger(id, agent)
	logger.Debug(ctx, "coordinating client")
	tc, err := c.core.initAndTrackClient(ctx, cancel, conn, id, agent)
	if err != nil {
		return err
	}
	defer c.core.clientDisconnected(id, agent)

	// On this goroutine, we read updates from the client and publish them.  We start a second goroutine
	// to write updates back to the client.
	go tc.SendUpdates()

	decoder := json.NewDecoder(conn)
	for {
		err := c.handleNextClientMessage(id, agent, decoder)
		if err != nil {
			logger.Debug(ctx, "unable to read client update, connection may be closed", slog.Error(err))
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, context.Canceled) {
				return nil
			}
			return xerrors.Errorf("handle next client message: %w", err)
		}
	}
}

func (c *core) clientLogger(id, agent uuid.UUID) slog.Logger {
	return c.logger.With(slog.F("client_id", id), slog.F("agent_id", agent))
}

// initAndTrackClient creates a TrackedConn for the client, and sends any initial Node updates if we have any.  It is
// one function that does two things because it is critical that we hold the mutex for both things, lest we miss some
// updates.
func (c *core) initAndTrackClient(
	ctx context.Context, cancel func(), conn net.Conn, id, agent uuid.UUID,
) (
	*TrackedConn, error,
) {
	logger := c.clientLogger(id, agent)
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.closed {
		return nil, xerrors.New("coordinator is closed")
	}
	tc := NewTrackedConn(ctx, cancel, conn, id, logger, 0)

	// When a new connection is requested, we update it with the latest
	// node of the agent. This allows the connection to establish.
	node, ok := c.nodes[agent]
	if ok {
		err := tc.Enqueue([]*Node{node})
		// this should never error since we're still the only goroutine that
		// knows about the TrackedConn.  If we hit an error something really
		// wrong is happening
		if err != nil {
			logger.Critical(ctx, "unable to queue initial node", slog.Error(err))
			return nil, err
		}
	}

	// Insert this connection into a map so the agent
	// can publish node updates.
	connectionSockets, ok := c.agentToConnectionSockets[agent]
	if !ok {
		connectionSockets = map[uuid.UUID]*TrackedConn{}
		c.agentToConnectionSockets[agent] = connectionSockets
	}
	connectionSockets[id] = tc
	logger.Debug(ctx, "added tracked connection")
	return tc, nil
}

func (c *core) clientDisconnected(id, agent uuid.UUID) {
	logger := c.clientLogger(id, agent)
	c.mutex.Lock()
	defer c.mutex.Unlock()
	// Clean all traces of this connection from the map.
	delete(c.nodes, id)
	logger.Debug(context.Background(), "deleted client node")
	connectionSockets, ok := c.agentToConnectionSockets[agent]
	if !ok {
		return
	}
	delete(connectionSockets, id)
	logger.Debug(context.Background(), "deleted client connectionSocket from map")
	if len(connectionSockets) != 0 {
		return
	}
	delete(c.agentToConnectionSockets, agent)
	logger.Debug(context.Background(), "deleted last client connectionSocket from map")
}

func (c *coordinator) handleNextClientMessage(id, agent uuid.UUID, decoder *json.Decoder) error {
	logger := c.core.clientLogger(id, agent)
	var node Node
	err := decoder.Decode(&node)
	if err != nil {
		return xerrors.Errorf("read json: %w", err)
	}
	logger.Debug(context.Background(), "got client node update", slog.F("node", node))
	return c.core.clientNodeUpdate(id, agent, &node)
}

func (c *core) clientNodeUpdate(id, agent uuid.UUID, node *Node) error {
	logger := c.clientLogger(id, agent)
	c.mutex.Lock()
	defer c.mutex.Unlock()
	// Update the node of this client in our in-memory map. If an agent entirely
	// shuts down and reconnects, it needs to be aware of all clients attempting
	// to establish connections.
	c.nodes[id] = node

	agentSocket, ok := c.agentSockets[agent]
	if !ok {
		logger.Debug(context.Background(), "no agent socket, unable to send node")
		return nil
	}

	err := agentSocket.Enqueue([]*Node{node})
	if err != nil {
		return xerrors.Errorf("Enqueue node: %w", err)
	}
	logger.Debug(context.Background(), "enqueued node to agent")
	return nil
}

func (c *core) agentLogger(id uuid.UUID) slog.Logger {
	return c.logger.With(slog.F("agent_id", id))
}

// ServeAgent accepts a WebSocket connection to an agent that
// listens to incoming connections and publishes node updates.
func (c *coordinator) ServeAgent(conn net.Conn, id uuid.UUID, name string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := c.core.agentLogger(id)
	logger.Debug(context.Background(), "coordinating agent")
	// This uniquely identifies a connection that belongs to this goroutine.
	unique := uuid.New()
	tc, err := c.core.initAndTrackAgent(ctx, cancel, conn, id, unique, name)
	if err != nil {
		return err
	}

	// On this goroutine, we read updates from the agent and publish them.  We start a second goroutine
	// to write updates back to the agent.
	go tc.SendUpdates()

	defer c.core.agentDisconnected(id, unique)

	decoder := json.NewDecoder(conn)
	for {
		err := c.handleNextAgentMessage(id, decoder)
		if err != nil {
			logger.Debug(ctx, "unable to read agent update, connection may be closed", slog.Error(err))
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, context.Canceled) {
				return nil
			}
			return xerrors.Errorf("handle next agent message: %w", err)
		}
	}
}

func (c *core) agentDisconnected(id, unique uuid.UUID) {
	logger := c.agentLogger(id)
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Only delete the connection if it's ours. It could have been
	// overwritten.
	if idConn, ok := c.agentSockets[id]; ok && idConn.ID == unique {
		delete(c.agentSockets, id)
		delete(c.nodes, id)
		logger.Debug(context.Background(), "deleted agent socket and node")
	}
}

// initAndTrackAgent creates a TrackedConn for the agent, and sends any initial nodes updates if we have any.  It is
// one function that does two things because it is critical that we hold the mutex for both things, lest we miss some
// updates.
func (c *core) initAndTrackAgent(ctx context.Context, cancel func(), conn net.Conn, id, unique uuid.UUID, name string) (*TrackedConn, error) {
	logger := c.logger.With(slog.F("agent_id", id))
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.closed {
		return nil, xerrors.New("coordinator is closed")
	}

	overwrites := int64(0)
	// If an old agent socket is connected, we Close it to avoid any leaks. This
	// shouldn't ever occur because we expect one agent to be running, but it's
	// possible for a race condition to happen when an agent is disconnected and
	// attempts to reconnect before the server realizes the old connection is
	// dead.
	oldAgentSocket, ok := c.agentSockets[id]
	if ok {
		overwrites = oldAgentSocket.Overwrites + 1
		_ = oldAgentSocket.Close()
	}
	tc := NewTrackedConn(ctx, cancel, conn, unique, logger, overwrites)
	c.agentNameCache.Add(id, name)

	sockets, ok := c.agentToConnectionSockets[id]
	if ok {
		// Publish all nodes that want to connect to the
		// desired agent ID.
		nodes := make([]*Node, 0, len(sockets))
		for targetID := range sockets {
			node, ok := c.nodes[targetID]
			if !ok {
				continue
			}
			nodes = append(nodes, node)
		}
		err := tc.Enqueue(nodes)
		// this should never error since we're still the only goroutine that
		// knows about the TrackedConn.  If we hit an error something really
		// wrong is happening
		if err != nil {
			logger.Critical(ctx, "unable to queue initial nodes", slog.Error(err))
			return nil, err
		}
		logger.Debug(ctx, "wrote initial client(s) to agent", slog.F("nodes", nodes))
	}

	c.agentSockets[id] = tc
	logger.Debug(ctx, "added agent socket")
	return tc, nil
}

func (c *coordinator) handleNextAgentMessage(id uuid.UUID, decoder *json.Decoder) error {
	logger := c.core.agentLogger(id)
	var node Node
	err := decoder.Decode(&node)
	if err != nil {
		return xerrors.Errorf("read json: %w", err)
	}
	logger.Debug(context.Background(), "decoded agent node", slog.F("node", node))
	return c.core.agentNodeUpdate(id, &node)
}

func (c *core) agentNodeUpdate(id uuid.UUID, node *Node) error {
	logger := c.agentLogger(id)
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.nodes[id] = node
	connectionSockets, ok := c.agentToConnectionSockets[id]
	if !ok {
		logger.Debug(context.Background(), "no client sockets; unable to send node")
		return nil
	}

	// Publish the new node to every listening socket.
	for clientID, connectionSocket := range connectionSockets {
		err := connectionSocket.Enqueue([]*Node{node})
		if err == nil {
			logger.Debug(context.Background(), "enqueued agent node to client",
				slog.F("client_id", clientID))
		} else {
			// queue is backed up for some reason.  This is bad, but we don't want to drop
			// updates to other clients over it.  Log and move on.
			logger.Error(context.Background(), "failed to Enqueue",
				slog.F("client_id", clientID), slog.Error(err))
		}
	}

	wg := sync.WaitGroup{}
	cbs := c.agentCallbacks[id]
	wg.Add(len(cbs))
	for _, cb := range cbs {
		cb := cb
		go func() {
			cb(id, node)
			wg.Done()
		}()
	}

	wg.Wait()
	return nil
}

// Close closes all of the open connections in the coordinator and stops the
// coordinator from accepting new connections.
func (c *coordinator) Close() error {
	return c.core.close()
}

func (c *core) close() error {
	c.mutex.Lock()
	if c.closed {
		c.mutex.Unlock()
		return nil
	}
	c.closed = true

	wg := sync.WaitGroup{}

	wg.Add(len(c.agentSockets))
	for _, socket := range c.agentSockets {
		socket := socket
		go func() {
			_ = socket.Close()
			wg.Done()
		}()
	}

	for _, connMap := range c.agentToConnectionSockets {
		wg.Add(len(connMap))
		for _, socket := range connMap {
			socket := socket
			go func() {
				_ = socket.Close()
				wg.Done()
			}()
		}
	}

	c.mutex.Unlock()

	wg.Wait()
	return nil
}

func (c *coordinator) ServeHTTPDebug(w http.ResponseWriter, r *http.Request) {
	c.core.serveHTTPDebug(w, r)
}

func (c *core) serveHTTPDebug(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	_, _ = fmt.Fprintln(w, "<h1>in-memory wireguard coordinator debug</h1>")

	CoordinatorHTTPDebug(c.agentSockets, c.agentToConnectionSockets, c.agentNameCache)(w, r)
}

func CoordinatorHTTPDebug(
	agentSocketsMap map[uuid.UUID]*TrackedConn,
	agentToConnectionSocketsMap map[uuid.UUID]map[uuid.UUID]*TrackedConn,
	agentNameCache *lru.Cache[uuid.UUID, string],
) func(w http.ResponseWriter, _ *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		now := time.Now()

		type idConn struct {
			id   uuid.UUID
			conn *TrackedConn
		}

		{
			_, _ = fmt.Fprintf(w, "<h2 id=agents><a href=#agents>#</a> agents: total %d</h2>\n", len(agentSocketsMap))
			_, _ = fmt.Fprintln(w, "<ul>")
			agentSockets := make([]idConn, 0, len(agentSocketsMap))

			for id, conn := range agentSocketsMap {
				agentSockets = append(agentSockets, idConn{id, conn})
			}

			slices.SortFunc(agentSockets, func(a, b idConn) bool {
				return a.conn.Name < b.conn.Name
			})

			for _, agent := range agentSockets {
				_, _ = fmt.Fprintf(w, "<li style=\"margin-top:4px\"><b>%s</b> (<code>%s</code>): created %v ago, write %v ago, overwrites %d </li>\n",
					agent.conn.Name,
					agent.id.String(),
					now.Sub(time.Unix(agent.conn.Start, 0)).Round(time.Second),
					now.Sub(time.Unix(agent.conn.LastWrite, 0)).Round(time.Second),
					agent.conn.Overwrites,
				)

				if conns := agentToConnectionSocketsMap[agent.id]; len(conns) > 0 {
					_, _ = fmt.Fprintf(w, "<h3 style=\"margin:0px;font-size:16px;font-weight:400\">connections: total %d</h3>\n", len(conns))

					connSockets := make([]idConn, 0, len(conns))
					for id, conn := range conns {
						connSockets = append(connSockets, idConn{id, conn})
					}
					slices.SortFunc(connSockets, func(a, b idConn) bool {
						return a.id.String() < b.id.String()
					})

					_, _ = fmt.Fprintln(w, "<ul>")
					for _, connSocket := range connSockets {
						_, _ = fmt.Fprintf(w, "<li><b>%s</b> (<code>%s</code>): created %v ago, write %v ago </li>\n",
							connSocket.conn.Name,
							connSocket.id.String(),
							now.Sub(time.Unix(connSocket.conn.Start, 0)).Round(time.Second),
							now.Sub(time.Unix(connSocket.conn.LastWrite, 0)).Round(time.Second),
						)
					}
					_, _ = fmt.Fprintln(w, "</ul>")
				}
			}

			_, _ = fmt.Fprintln(w, "</ul>")
		}

		{
			type agentConns struct {
				id    uuid.UUID
				conns []idConn
			}

			missingAgents := []agentConns{}
			for agentID, conns := range agentToConnectionSocketsMap {
				if len(conns) == 0 {
					continue
				}

				if _, ok := agentSocketsMap[agentID]; !ok {
					connsSlice := make([]idConn, 0, len(conns))
					for id, conn := range conns {
						connsSlice = append(connsSlice, idConn{id, conn})
					}
					slices.SortFunc(connsSlice, func(a, b idConn) bool {
						return a.id.String() < b.id.String()
					})

					missingAgents = append(missingAgents, agentConns{agentID, connsSlice})
				}
			}
			slices.SortFunc(missingAgents, func(a, b agentConns) bool {
				return a.id.String() < b.id.String()
			})

			_, _ = fmt.Fprintf(w, "<h2 id=missing-agents><a href=#missing-agents>#</a> missing agents: total %d</h2>\n", len(missingAgents))
			_, _ = fmt.Fprintln(w, "<ul>")

			for _, agentConns := range missingAgents {
				agentName, ok := agentNameCache.Get(agentConns.id)
				if !ok {
					agentName = "unknown"
				}

				_, _ = fmt.Fprintf(w, "<li style=\"margin-top:4px\"><b>%s</b> (<code>%s</code>): created ? ago, write ? ago, overwrites ? </li>\n",
					agentName,
					agentConns.id.String(),
				)

				_, _ = fmt.Fprintf(w, "<h3 style=\"margin:0px;font-size:16px;font-weight:400\">connections: total %d</h3>\n", len(agentConns.conns))
				_, _ = fmt.Fprintln(w, "<ul>")
				for _, agentConn := range agentConns.conns {
					_, _ = fmt.Fprintf(w, "<li><b>%s</b> (<code>%s</code>): created %v ago, write %v ago </li>\n",
						agentConn.conn.Name,
						agentConn.id.String(),
						now.Sub(time.Unix(agentConn.conn.Start, 0)).Round(time.Second),
						now.Sub(time.Unix(agentConn.conn.LastWrite, 0)).Round(time.Second),
					)
				}
				_, _ = fmt.Fprintln(w, "</ul>")
			}
			_, _ = fmt.Fprintln(w, "</ul>")
		}
	}
}

func (c *coordinator) SubscribeAgent(agentID uuid.UUID, cb func(agentID uuid.UUID, node *Node)) func() {
	c.core.mutex.Lock()
	defer c.core.mutex.Unlock()

	id := uuid.New()
	cbMap, ok := c.core.agentCallbacks[agentID]
	if !ok {
		cbMap = map[uuid.UUID]func(uuid.UUID, *Node){}
		c.core.agentCallbacks[agentID] = cbMap
	}

	cbMap[id] = cb

	return func() {
		c.core.mutex.Lock()
		defer c.core.mutex.Unlock()
		delete(cbMap, id)
	}
}

func (c *coordinator) BroadcastToAgents(agents []uuid.UUID, node *Node) error {
	ctx := context.Background()

	for _, id := range agents {
		c.core.mutex.Lock()
		agentSocket, ok := c.core.agentSockets[id]
		c.core.mutex.Unlock()
		if !ok {
			continue
		}

		// Write the new node from this client to the actively connected agent.
		err := agentSocket.Enqueue([]*Node{node})
		if err != nil {
			c.core.logger.Debug(ctx, "failed to write to agent", slog.Error(err))
		}
	}

	return nil
}
