package tailnet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/util/slice"
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

	ServeMultiAgent(id uuid.UUID) MultiAgentConn
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

func (c *coordinator) ServeMultiAgent(id uuid.UUID) MultiAgentConn {
	m := (&MultiAgent{
		ID:                id,
		AgentIsLegacyFunc: c.core.agentIsLegacy,
		OnSubscribe:       c.core.clientSubscribeToAgent,
		OnUnsubscribe:     c.core.clientUnsubscribeFromAgent,
		OnNodeUpdate:      c.core.clientNodeUpdate,
		OnRemove:          func(enq Queue) { c.core.clientDisconnected(enq.UniqueID()) },
	}).Init()
	c.core.addClient(id, m)
	return m
}

func (c *core) addClient(id uuid.UUID, ma Queue) {
	c.mutex.Lock()
	c.clients[id] = ma
	c.clientsToAgents[id] = map[uuid.UUID]Queue{}
	c.mutex.Unlock()
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
	agentSockets map[uuid.UUID]Queue
	// agentToConnectionSockets maps agent IDs to connection IDs of conns that
	// are subscribed to updates for that agent.
	agentToConnectionSockets map[uuid.UUID]map[uuid.UUID]Queue

	// clients holds a map of all clients connected to the coordinator. This is
	// necessary because a client may not be subscribed into any agents.
	clients map[uuid.UUID]Queue
	// clientsToAgents is an index of clients to all of their subscribed agents.
	clientsToAgents map[uuid.UUID]map[uuid.UUID]Queue

	// agentNameCache holds a cache of agent names. If one of them disappears,
	// it's helpful to have a name cached for debugging.
	agentNameCache *lru.Cache[uuid.UUID, string]

	// legacyAgents holda a mapping of all agents detected as legacy, meaning
	// they only listen on codersdk.WorkspaceAgentIP. They aren't compatible
	// with the new ServerTailnet, so they must be connected through
	// wsconncache.
	legacyAgents map[uuid.UUID]struct{}
}

type QueueKind int

const (
	QueueKindClient QueueKind = 1 + iota
	QueueKindAgent
)

type Queue interface {
	UniqueID() uuid.UUID
	Kind() QueueKind
	Enqueue(n []*Node) error
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
	nameCache, err := lru.New[uuid.UUID, string](512)
	if err != nil {
		panic("make lru cache: " + err.Error())
	}

	return &core{
		logger:                   logger,
		closed:                   false,
		nodes:                    map[uuid.UUID]*Node{},
		agentSockets:             map[uuid.UUID]Queue{},
		agentToConnectionSockets: map[uuid.UUID]map[uuid.UUID]Queue{},
		agentNameCache:           nameCache,
		legacyAgents:             map[uuid.UUID]struct{}{},
		clients:                  map[uuid.UUID]Queue{},
		clientsToAgents:          map[uuid.UUID]map[uuid.UUID]Queue{},
	}
}

var ErrWouldBlock = xerrors.New("would block")

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
func (c *coordinator) ServeClient(conn net.Conn, id, agentID uuid.UUID) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := c.core.clientLogger(id, agentID)
	logger.Debug(ctx, "coordinating client")

	tc := NewTrackedConn(ctx, cancel, conn, id, logger, id.String(), 0, QueueKindClient)
	defer tc.Close()

	c.core.addClient(id, tc)
	defer c.core.clientDisconnected(id)

	agentNode, err := c.core.clientSubscribeToAgent(tc, agentID)
	if err != nil {
		return xerrors.Errorf("subscribe agent: %w", err)
	}

	if agentNode != nil {
		err := tc.Enqueue([]*Node{agentNode})
		if err != nil {
			logger.Debug(ctx, "enqueue initial node", slog.Error(err))
		}
	}

	// On this goroutine, we read updates from the client and publish them.  We start a second goroutine
	// to write updates back to the client.
	go tc.SendUpdates()

	decoder := json.NewDecoder(conn)
	for {
		err := c.handleNextClientMessage(id, decoder)
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

func (c *core) initOrSetAgentConnectionSocketLocked(agentID uuid.UUID, enq Queue) {
	connectionSockets, ok := c.agentToConnectionSockets[agentID]
	if !ok {
		connectionSockets = map[uuid.UUID]Queue{}
		c.agentToConnectionSockets[agentID] = connectionSockets
	}
	connectionSockets[enq.UniqueID()] = enq

	c.clientsToAgents[enq.UniqueID()][agentID] = c.agentSockets[agentID]
}

func (c *core) clientDisconnected(id uuid.UUID) {
	logger := c.clientLogger(id, uuid.Nil)
	c.mutex.Lock()
	defer c.mutex.Unlock()
	// Clean all traces of this connection from the map.
	delete(c.nodes, id)
	logger.Debug(context.Background(), "deleted client node")

	for agentID := range c.clientsToAgents[id] {
		connectionSockets, ok := c.agentToConnectionSockets[agentID]
		if !ok {
			continue
		}
		delete(connectionSockets, id)
		logger.Debug(context.Background(), "deleted client connectionSocket from map", slog.F("agent_id", agentID))

		if len(connectionSockets) == 0 {
			delete(c.agentToConnectionSockets, agentID)
			logger.Debug(context.Background(), "deleted last client connectionSocket from map", slog.F("agent_id", agentID))
		}
	}

	delete(c.clients, id)
	delete(c.clientsToAgents, id)
	logger.Debug(context.Background(), "deleted client agents")
}

func (c *coordinator) handleNextClientMessage(id uuid.UUID, decoder *json.Decoder) error {
	logger := c.core.clientLogger(id, uuid.Nil)

	var node Node
	err := decoder.Decode(&node)
	if err != nil {
		return xerrors.Errorf("read json: %w", err)
	}

	logger.Debug(context.Background(), "got client node update", slog.F("node", node))
	return c.core.clientNodeUpdate(id, &node)
}

func (c *core) clientNodeUpdate(id uuid.UUID, node *Node) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Update the node of this client in our in-memory map. If an agent entirely
	// shuts down and reconnects, it needs to be aware of all clients attempting
	// to establish connections.
	c.nodes[id] = node

	return c.clientNodeUpdateLocked(id, node)
}

func (c *core) clientNodeUpdateLocked(id uuid.UUID, node *Node) error {
	logger := c.clientLogger(id, uuid.Nil)

	agents := []uuid.UUID{}
	for agentID, agentSocket := range c.clientsToAgents[id] {
		if agentSocket == nil {
			logger.Debug(context.Background(), "enqueue node to agent; socket is nil", slog.F("agent_id", agentID))
			continue
		}

		err := agentSocket.Enqueue([]*Node{node})
		if err != nil {
			logger.Debug(context.Background(), "unable to Enqueue node to agent", slog.Error(err), slog.F("agent_id", agentID))
			continue
		}
		agents = append(agents, agentID)
	}

	logger.Debug(context.Background(), "enqueued node to agents", slog.F("agent_ids", agents))
	return nil
}

func (c *core) clientSubscribeToAgent(enq Queue, agentID uuid.UUID) (*Node, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	logger := c.clientLogger(enq.UniqueID(), agentID)

	c.initOrSetAgentConnectionSocketLocked(agentID, enq)

	node, ok := c.nodes[enq.UniqueID()]
	if ok {
		// If we have the client node, send it to the agent. If not, it will be
		// sent async.
		agentSocket, ok := c.agentSockets[agentID]
		if !ok {
			logger.Debug(context.Background(), "subscribe to agent; socket is nil")
		} else {
			err := agentSocket.Enqueue([]*Node{node})
			if err != nil {
				return nil, xerrors.Errorf("enqueue client to agent: %w", err)
			}
		}
	} else {
		logger.Debug(context.Background(), "multiagent node doesn't exist")
	}

	agentNode, ok := c.nodes[agentID]
	if !ok {
		// This is ok, once the agent connects the node will be sent over.
		logger.Debug(context.Background(), "agent node doesn't exist", slog.F("agent_id", agentID))
	}

	// Send the subscribed agent back to the multi agent.
	return agentNode, nil
}

func (c *core) clientUnsubscribeFromAgent(enq Queue, agentID uuid.UUID) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.clientsToAgents[enq.UniqueID()], agentID)
	delete(c.agentToConnectionSockets[agentID], enq.UniqueID())

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
	if idConn, ok := c.agentSockets[id]; ok && idConn.UniqueID() == unique {
		delete(c.agentSockets, id)
		delete(c.nodes, id)
		logger.Debug(context.Background(), "deleted agent socket and node")
	}
	for clientID := range c.agentToConnectionSockets[id] {
		c.clientsToAgents[clientID][id] = nil
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
		overwrites = oldAgentSocket.Overwrites() + 1
		_ = oldAgentSocket.Close()
	}
	tc := NewTrackedConn(ctx, cancel, conn, unique, logger, name, overwrites, QueueKindAgent)
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
	for clientID := range c.agentToConnectionSockets[id] {
		c.clientsToAgents[clientID][id] = tc
	}

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

// This is copied from codersdk because importing it here would cause an import
// cycle. This is just temporary until wsconncache is phased out.
var legacyAgentIP = netip.MustParseAddr("fd7a:115c:a1e0:49d6:b259:b7ac:b1b2:48f4")

// This is temporary until we no longer need to detect for agent backwards
// compatibility.
// See: https://github.com/coder/coder/issues/8218
func (c *core) agentIsLegacy(agentID uuid.UUID) bool {
	c.mutex.RLock()
	_, ok := c.legacyAgents[agentID]
	c.mutex.RUnlock()
	return ok
}

func (c *core) agentNodeUpdate(id uuid.UUID, node *Node) error {
	logger := c.agentLogger(id)
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.nodes[id] = node

	// Keep a cache of all legacy agents.
	if len(node.Addresses) > 0 && node.Addresses[0].Addr() == legacyAgentIP {
		c.legacyAgents[id] = struct{}{}
	}

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
			_ = socket.CoordinatorClose()
			wg.Done()
		}()
	}

	wg.Add(len(c.clients))
	for _, client := range c.clients {
		client := client
		go func() {
			_ = client.CoordinatorClose()
			wg.Done()
		}()
	}

	c.mutex.Unlock()

	wg.Wait()
	return nil
}

func (c *coordinator) ServeHTTPDebug(w http.ResponseWriter, r *http.Request) {
	c.core.serveHTTPDebug(w, r)
}

func (c *core) serveHTTPDebug(w http.ResponseWriter, r *http.Request) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	CoordinatorHTTPDebug(
		HTTPDebugFromLocal(false, c.agentSockets, c.agentToConnectionSockets, c.nodes, c.agentNameCache),
	)(w, r)
}

func HTTPDebugFromLocal(
	ha bool,
	agentSocketsMap map[uuid.UUID]Queue,
	agentToConnectionSocketsMap map[uuid.UUID]map[uuid.UUID]Queue,
	nodesMap map[uuid.UUID]*Node,
	agentNameCache *lru.Cache[uuid.UUID, string],
) HTMLDebug {
	now := time.Now()
	data := HTMLDebug{HA: ha}
	for id, conn := range agentSocketsMap {
		start, lastWrite := conn.Stats()
		agent := &HTMLAgent{
			Name:         conn.Name(),
			ID:           id,
			CreatedAge:   now.Sub(time.Unix(start, 0)).Round(time.Second),
			LastWriteAge: now.Sub(time.Unix(lastWrite, 0)).Round(time.Second),
			Overwrites:   int(conn.Overwrites()),
		}

		for id, conn := range agentToConnectionSocketsMap[id] {
			start, lastWrite := conn.Stats()
			agent.Connections = append(agent.Connections, &HTMLClient{
				Name:         conn.Name(),
				ID:           id,
				CreatedAge:   now.Sub(time.Unix(start, 0)).Round(time.Second),
				LastWriteAge: now.Sub(time.Unix(lastWrite, 0)).Round(time.Second),
			})
		}
		slices.SortFunc(agent.Connections, func(a, b *HTMLClient) int {
			return slice.Ascending(a.Name, b.Name)
		})

		data.Agents = append(data.Agents, agent)
	}
	slices.SortFunc(data.Agents, func(a, b *HTMLAgent) int {
		return slice.Ascending(a.Name, b.Name)
	})

	for agentID, conns := range agentToConnectionSocketsMap {
		if len(conns) == 0 {
			continue
		}

		if _, ok := agentSocketsMap[agentID]; ok {
			continue
		}

		agentName, ok := agentNameCache.Get(agentID)
		if !ok {
			agentName = "unknown"
		}
		agent := &HTMLAgent{
			Name: agentName,
			ID:   agentID,
		}
		for id, conn := range conns {
			start, lastWrite := conn.Stats()
			agent.Connections = append(agent.Connections, &HTMLClient{
				Name:         conn.Name(),
				ID:           id,
				CreatedAge:   now.Sub(time.Unix(start, 0)).Round(time.Second),
				LastWriteAge: now.Sub(time.Unix(lastWrite, 0)).Round(time.Second),
			})
		}
		slices.SortFunc(agent.Connections, func(a, b *HTMLClient) int {
			return slice.Ascending(a.Name, b.Name)
		})

		data.MissingAgents = append(data.MissingAgents, agent)
	}
	slices.SortFunc(data.MissingAgents, func(a, b *HTMLAgent) int {
		return slice.Ascending(a.Name, b.Name)
	})

	for id, node := range nodesMap {
		name, _ := agentNameCache.Get(id)
		data.Nodes = append(data.Nodes, &HTMLNode{
			ID:   id,
			Name: name,
			Node: node,
		})
	}
	slices.SortFunc(data.Nodes, func(a, b *HTMLNode) int {
		return slice.Ascending(a.Name+a.ID.String(), b.Name+b.ID.String())
	})

	return data
}

func CoordinatorHTTPDebug(data HTMLDebug) func(w http.ResponseWriter, _ *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		tmpl, err := template.New("coordinator_debug").Funcs(template.FuncMap{
			"marshal": func(v any) template.JS {
				a, err := json.MarshalIndent(v, "", "  ")
				if err != nil {
					//nolint:gosec
					return template.JS(fmt.Sprintf(`{"err": %q}`, err))
				}
				//nolint:gosec
				return template.JS(a)
			},
		}).Parse(coordinatorDebugTmpl)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		err = tmpl.Execute(w, data)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			return
		}
	}
}

type HTMLDebug struct {
	HA            bool
	Agents        []*HTMLAgent
	MissingAgents []*HTMLAgent
	Nodes         []*HTMLNode
}

type HTMLAgent struct {
	Name         string
	ID           uuid.UUID
	CreatedAge   time.Duration
	LastWriteAge time.Duration
	Overwrites   int
	Connections  []*HTMLClient
}

type HTMLClient struct {
	Name         string
	ID           uuid.UUID
	CreatedAge   time.Duration
	LastWriteAge time.Duration
}

type HTMLNode struct {
	ID   uuid.UUID
	Name string
	Node any
}

var coordinatorDebugTmpl = `
<!DOCTYPE html>
<html>
	<head>
		<meta charset="UTF-8">
	</head>
	<body>
	{{- if .HA }}
		<h1>high-availability wireguard coordinator debug</h1>
		<h4 style="margin-top:-25px">warning: this only provides info from the node that served the request, if there are multiple replicas this data may be incomplete</h4>
	{{- else }}
		<h1>in-memory wireguard coordinator debug</h1>
	{{- end }}

		<h2 id=agents> <a href=#agents>#</a> agents: total {{ len .Agents }} </h2>
		<ul>
		{{- range .Agents }}
			<li style="margin-top:4px">
				<b>{{ .Name }}</b> (<code>{{ .ID }}</code>): created {{ .CreatedAge }} ago, write {{ .LastWriteAge }} ago, overwrites {{ .Overwrites }}
				<h3 style="margin:0px;font-size:16px;font-weight:400"> connections: total {{ len .Connections}} </h3>
				<ul>
				{{- range .Connections }}
					<li><b>{{ .Name }}</b> (<code>{{ .ID }}</code>): created {{ .CreatedAge }} ago, write {{ .LastWriteAge }} ago </li>
				{{- end }}
				</ul>
			</li>
		{{- end }}
		</ul>

		<h2 id=missing-agents><a href=#missing-agents>#</a> missing agents: total {{ len .MissingAgents }}</h2>
		<ul>
		{{- range .MissingAgents}}
			<li style="margin-top:4px"><b>{{ .Name }}</b> (<code>{{ .ID }}</code>): created ? ago, write ? ago, overwrites ? </li>
			<h3 style="margin:0px;font-size:16px;font-weight:400"> connections: total {{ len .Connections }} </h3>
			<ul>
			{{- range .Connections }}
				<li><b>{{ .Name }}</b> (<code>{{ .ID }}</code>): created {{ .CreatedAge }} ago, write {{ .LastWriteAge }} ago </li>
			{{- end }}
			</ul>
		{{- end }}
		</ul>

		<h2 id=nodes><a href=#nodes>#</a> nodes: total {{ len .Nodes }}</h2>
		<ul>
		{{- range .Nodes }}
			<li style="margin-top:4px"><b>{{ .Name }}</b> (<code>{{ .ID }}</code>):
				<span style="white-space: pre;"><code>{{ marshal .Node }}</code></span>
			</li>
		{{- end }}
		</ul>
	</body>
</html>
`
