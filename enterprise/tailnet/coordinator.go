package tailnet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/codersdk"
	agpl "github.com/coder/coder/v2/tailnet"
)

// NewCoordinator creates a new high availability coordinator
// that uses PostgreSQL pubsub to exchange handshakes.
func NewCoordinator(logger slog.Logger, ps pubsub.Pubsub) (agpl.Coordinator, error) {
	ctx, cancelFunc := context.WithCancel(context.Background())

	nameCache, err := lru.New[uuid.UUID, string](512)
	if err != nil {
		panic("make lru cache: " + err.Error())
	}

	coord := &haCoordinator{
		id:                       uuid.New(),
		log:                      logger,
		pubsub:                   ps,
		closeFunc:                cancelFunc,
		close:                    make(chan struct{}),
		nodes:                    map[uuid.UUID]*agpl.Node{},
		agentSockets:             map[uuid.UUID]agpl.Queue{},
		agentToConnectionSockets: map[uuid.UUID]map[uuid.UUID]agpl.Queue{},
		agentNameCache:           nameCache,
		clients:                  map[uuid.UUID]agpl.Queue{},
		clientsToAgents:          map[uuid.UUID]map[uuid.UUID]agpl.Queue{},
		legacyAgents:             map[uuid.UUID]struct{}{},
	}

	if err := coord.runPubsub(ctx); err != nil {
		return nil, xerrors.Errorf("run coordinator pubsub: %w", err)
	}

	return coord, nil
}

func (c *haCoordinator) ServeMultiAgent(id uuid.UUID) agpl.MultiAgentConn {
	m := (&agpl.MultiAgent{
		ID:                id,
		AgentIsLegacyFunc: c.agentIsLegacy,
		OnSubscribe:       c.clientSubscribeToAgent,
		OnNodeUpdate:      c.clientNodeUpdate,
		OnRemove:          func(enq agpl.Queue) { c.clientDisconnected(enq.UniqueID()) },
	}).Init()
	c.addClient(id, m)
	return m
}

func (c *haCoordinator) addClient(id uuid.UUID, q agpl.Queue) {
	c.mutex.Lock()
	c.clients[id] = q
	c.clientsToAgents[id] = map[uuid.UUID]agpl.Queue{}
	c.mutex.Unlock()
}

func (c *haCoordinator) clientSubscribeToAgent(enq agpl.Queue, agentID uuid.UUID) (*agpl.Node, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.initOrSetAgentConnectionSocketLocked(agentID, enq)

	node := c.nodes[enq.UniqueID()]
	if node != nil {
		err := c.sendNodeToAgentLocked(agentID, node)
		if err != nil {
			return nil, xerrors.Errorf("handle client update: %w", err)
		}
	}

	agentNode, ok := c.nodes[agentID]
	// If we have the node locally, give it back to the multiagent.
	if ok {
		return agentNode, nil
	}

	// If we don't have the node locally, notify other coordinators.
	err := c.publishClientHello(agentID)
	if err != nil {
		return nil, xerrors.Errorf("publish client hello: %w", err)
	}

	// nolint:nilnil
	return nil, nil
}

type haCoordinator struct {
	id        uuid.UUID
	log       slog.Logger
	mutex     sync.RWMutex
	pubsub    pubsub.Pubsub
	close     chan struct{}
	closeFunc context.CancelFunc

	// nodes maps agent and connection IDs their respective node.
	nodes map[uuid.UUID]*agpl.Node
	// agentSockets maps agent IDs to their open websocket.
	agentSockets map[uuid.UUID]agpl.Queue
	// agentToConnectionSockets maps agent IDs to connection IDs of conns that
	// are subscribed to updates for that agent.
	agentToConnectionSockets map[uuid.UUID]map[uuid.UUID]agpl.Queue

	// clients holds a map of all clients connected to the coordinator. This is
	// necessary because a client may not be subscribed into any agents.
	clients map[uuid.UUID]agpl.Queue
	// clientsToAgents is an index of clients to all of their subscribed agents.
	clientsToAgents map[uuid.UUID]map[uuid.UUID]agpl.Queue

	// agentNameCache holds a cache of agent names. If one of them disappears,
	// it's helpful to have a name cached for debugging.
	agentNameCache *lru.Cache[uuid.UUID, string]

	// legacyAgents holda a mapping of all agents detected as legacy, meaning
	// they only listen on codersdk.WorkspaceAgentIP. They aren't compatible
	// with the new ServerTailnet, so they must be connected through
	// wsconncache.
	legacyAgents map[uuid.UUID]struct{}
}

// Node returns an in-memory node by ID.
func (c *haCoordinator) Node(id uuid.UUID) *agpl.Node {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	node := c.nodes[id]
	return node
}

func (c *haCoordinator) clientLogger(id, agent uuid.UUID) slog.Logger {
	return c.log.With(slog.F("client_id", id), slog.F("agent_id", agent))
}

func (c *haCoordinator) agentLogger(agent uuid.UUID) slog.Logger {
	return c.log.With(slog.F("agent_id", agent))
}

// ServeClient accepts a WebSocket connection that wants to connect to an agent
// with the specified ID.
func (c *haCoordinator) ServeClient(conn net.Conn, id, agentID uuid.UUID) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := c.clientLogger(id, agentID)

	tc := agpl.NewTrackedConn(ctx, cancel, conn, id, logger, id.String(), 0, agpl.QueueKindClient)
	defer tc.Close()

	c.addClient(id, tc)
	defer c.clientDisconnected(id)

	agentNode, err := c.clientSubscribeToAgent(tc, agentID)
	if err != nil {
		return xerrors.Errorf("subscribe agent: %w", err)
	}

	if agentNode != nil {
		err := tc.Enqueue([]*agpl.Node{agentNode})
		if err != nil {
			logger.Debug(ctx, "enqueue initial node", slog.Error(err))
		}
	}

	go tc.SendUpdates()

	decoder := json.NewDecoder(conn)
	// Indefinitely handle messages from the client websocket.
	for {
		err := c.handleNextClientMessage(id, decoder)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
				return nil
			}
			return xerrors.Errorf("handle next client message: %w", err)
		}
	}
}

func (c *haCoordinator) initOrSetAgentConnectionSocketLocked(agentID uuid.UUID, enq agpl.Queue) {
	connectionSockets, ok := c.agentToConnectionSockets[agentID]
	if !ok {
		connectionSockets = map[uuid.UUID]agpl.Queue{}
		c.agentToConnectionSockets[agentID] = connectionSockets
	}
	connectionSockets[enq.UniqueID()] = enq
	c.clientsToAgents[enq.UniqueID()][agentID] = c.agentSockets[agentID]
}

func (c *haCoordinator) clientDisconnected(id uuid.UUID) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for agentID := range c.clientsToAgents[id] {
		// Clean all traces of this connection from the map.
		delete(c.nodes, id)
		connectionSockets, ok := c.agentToConnectionSockets[agentID]
		if !ok {
			return
		}
		delete(connectionSockets, id)
		if len(connectionSockets) != 0 {
			return
		}
		delete(c.agentToConnectionSockets, agentID)
	}

	delete(c.clients, id)
	delete(c.clientsToAgents, id)
}

func (c *haCoordinator) handleNextClientMessage(id uuid.UUID, decoder *json.Decoder) error {
	var node agpl.Node
	err := decoder.Decode(&node)
	if err != nil {
		return xerrors.Errorf("read json: %w", err)
	}

	return c.clientNodeUpdate(id, &node)
}

func (c *haCoordinator) clientNodeUpdate(id uuid.UUID, node *agpl.Node) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	// Update the node of this client in our in-memory map. If an agent entirely
	// shuts down and reconnects, it needs to be aware of all clients attempting
	// to establish connections.
	c.nodes[id] = node

	for agentID, agentSocket := range c.clientsToAgents[id] {
		if agentSocket == nil {
			// If we don't own the agent locally, send it over pubsub to a node that
			// owns the agent.
			err := c.publishNodesToAgent(agentID, []*agpl.Node{node})
			if err != nil {
				c.log.Error(context.Background(), "publish node to agent", slog.Error(err), slog.F("agent_id", agentID))
			}
		} else {
			// Write the new node from this client to the actively connected agent.
			err := agentSocket.Enqueue([]*agpl.Node{node})
			if err != nil {
				c.log.Error(context.Background(), "enqueue node to agent", slog.Error(err), slog.F("agent_id", agentID))
			}
		}
	}

	return nil
}

func (c *haCoordinator) sendNodeToAgentLocked(agentID uuid.UUID, node *agpl.Node) error {
	agentSocket, ok := c.agentSockets[agentID]
	if !ok {
		// If we don't own the agent locally, send it over pubsub to a node that
		// owns the agent.
		err := c.publishNodesToAgent(agentID, []*agpl.Node{node})
		if err != nil {
			return xerrors.Errorf("publish node to agent")
		}
		return nil
	}
	err := agentSocket.Enqueue([]*agpl.Node{node})
	if err != nil {
		return xerrors.Errorf("enqueue node: %w", err)
	}
	return nil
}

// ServeAgent accepts a WebSocket connection to an agent that listens to
// incoming connections and publishes node updates.
func (c *haCoordinator) ServeAgent(conn net.Conn, id uuid.UUID, name string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := c.agentLogger(id)
	c.agentNameCache.Add(id, name)

	c.mutex.Lock()
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
	// This uniquely identifies a connection that belongs to this goroutine.
	unique := uuid.New()
	tc := agpl.NewTrackedConn(ctx, cancel, conn, unique, logger, name, overwrites, agpl.QueueKindAgent)

	// Publish all nodes on this instance that want to connect to this agent.
	nodes := c.nodesSubscribedToAgent(id)
	if len(nodes) > 0 {
		err := tc.Enqueue(nodes)
		if err != nil {
			c.mutex.Unlock()
			return xerrors.Errorf("enqueue nodes: %w", err)
		}
	}
	c.agentSockets[id] = tc
	for clientID := range c.agentToConnectionSockets[id] {
		c.clientsToAgents[clientID][id] = tc
	}
	c.mutex.Unlock()
	go tc.SendUpdates()

	// Tell clients on other instances to send a callmemaybe to us.
	err := c.publishAgentHello(id)
	if err != nil {
		return xerrors.Errorf("publish agent hello: %w", err)
	}

	defer func() {
		c.mutex.Lock()
		defer c.mutex.Unlock()

		// Only delete the connection if it's ours. It could have been
		// overwritten.
		if idConn, ok := c.agentSockets[id]; ok && idConn.UniqueID() == unique {
			delete(c.agentSockets, id)
			delete(c.nodes, id)
		}
		for clientID := range c.agentToConnectionSockets[id] {
			c.clientsToAgents[clientID][id] = nil
		}
	}()

	decoder := json.NewDecoder(conn)
	for {
		node, err := c.handleAgentUpdate(id, decoder)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, context.Canceled) {
				return nil
			}
			return xerrors.Errorf("handle next agent message: %w", err)
		}

		err = c.publishAgentToNodes(id, node)
		if err != nil {
			return xerrors.Errorf("publish agent to nodes: %w", err)
		}
	}
}

func (c *haCoordinator) nodesSubscribedToAgent(agentID uuid.UUID) []*agpl.Node {
	sockets, ok := c.agentToConnectionSockets[agentID]
	if !ok {
		return nil
	}

	nodes := make([]*agpl.Node, 0, len(sockets))
	for targetID := range sockets {
		node, ok := c.nodes[targetID]
		if !ok {
			continue
		}
		nodes = append(nodes, node)
	}

	return nodes
}

func (c *haCoordinator) handleClientHello(id uuid.UUID) error {
	c.mutex.Lock()
	node, ok := c.nodes[id]
	c.mutex.Unlock()
	if !ok {
		return nil
	}
	return c.publishAgentToNodes(id, node)
}

func (c *haCoordinator) agentIsLegacy(agentID uuid.UUID) bool {
	c.mutex.RLock()
	_, ok := c.legacyAgents[agentID]
	c.mutex.RUnlock()
	return ok
}

func (c *haCoordinator) handleAgentUpdate(id uuid.UUID, decoder *json.Decoder) (*agpl.Node, error) {
	var node agpl.Node
	err := decoder.Decode(&node)
	if err != nil {
		return nil, xerrors.Errorf("read json: %w", err)
	}

	c.mutex.Lock()
	// Keep a cache of all legacy agents.
	if len(node.Addresses) > 0 && node.Addresses[0].Addr() == codersdk.WorkspaceAgentIP {
		c.legacyAgents[id] = struct{}{}
	}

	oldNode := c.nodes[id]
	if oldNode != nil {
		if oldNode.AsOf.After(node.AsOf) {
			c.mutex.Unlock()
			return oldNode, nil
		}
	}
	c.nodes[id] = &node
	connectionSockets, ok := c.agentToConnectionSockets[id]
	if !ok {
		c.mutex.Unlock()
		return &node, nil
	}

	// Publish the new node to every listening socket.
	for _, connectionSocket := range connectionSockets {
		_ = connectionSocket.Enqueue([]*agpl.Node{&node})
	}

	c.mutex.Unlock()

	return &node, nil
}

// Close closes all of the open connections in the coordinator and stops the
// coordinator from accepting new connections.
func (c *haCoordinator) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	select {
	case <-c.close:
		return nil
	default:
	}
	close(c.close)
	c.closeFunc()

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

	wg.Wait()
	return nil
}

func (c *haCoordinator) publishNodesToAgent(recipient uuid.UUID, nodes []*agpl.Node) error {
	msg, err := c.formatCallMeMaybe(recipient, nodes)
	if err != nil {
		return xerrors.Errorf("format publish message: %w", err)
	}

	err = c.pubsub.Publish("wireguard_peers", msg)
	if err != nil {
		return xerrors.Errorf("publish message: %w", err)
	}

	return nil
}

func (c *haCoordinator) publishAgentHello(id uuid.UUID) error {
	msg, err := c.formatAgentHello(id)
	if err != nil {
		return xerrors.Errorf("format publish message: %w", err)
	}

	err = c.pubsub.Publish("wireguard_peers", msg)
	if err != nil {
		return xerrors.Errorf("publish message: %w", err)
	}

	return nil
}

func (c *haCoordinator) publishClientHello(id uuid.UUID) error {
	msg, err := c.formatClientHello(id)
	if err != nil {
		return xerrors.Errorf("format client hello: %w", err)
	}
	err = c.pubsub.Publish("wireguard_peers", msg)
	if err != nil {
		return xerrors.Errorf("publish client hello: %w", err)
	}
	return nil
}

func (c *haCoordinator) publishAgentToNodes(id uuid.UUID, node *agpl.Node) error {
	msg, err := c.formatAgentUpdate(id, node)
	if err != nil {
		return xerrors.Errorf("format publish message: %w", err)
	}

	err = c.pubsub.Publish("wireguard_peers", msg)
	if err != nil {
		return xerrors.Errorf("publish message: %w", err)
	}

	return nil
}

func (c *haCoordinator) runPubsub(ctx context.Context) error {
	messageQueue := make(chan []byte, 64)
	cancelSub, err := c.pubsub.Subscribe("wireguard_peers", func(ctx context.Context, message []byte) {
		select {
		case messageQueue <- message:
		case <-ctx.Done():
			return
		}
	})
	if err != nil {
		return xerrors.Errorf("subscribe wireguard peers")
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case message := <-messageQueue:
				c.handlePubsubMessage(ctx, message)
			}
		}
	}()

	go func() {
		defer cancelSub()
		<-c.close
	}()

	return nil
}

func (c *haCoordinator) handlePubsubMessage(ctx context.Context, message []byte) {
	sp := bytes.Split(message, []byte("|"))
	if len(sp) != 4 {
		c.log.Error(ctx, "invalid wireguard peer message", slog.F("msg", string(message)))
		return
	}

	var (
		coordinatorID = sp[0]
		eventType     = sp[1]
		agentID       = sp[2]
		nodeJSON      = sp[3]
	)

	sender, err := uuid.ParseBytes(coordinatorID)
	if err != nil {
		c.log.Error(ctx, "invalid sender id", slog.F("id", string(coordinatorID)), slog.F("msg", string(message)))
		return
	}

	// We sent this message!
	if sender == c.id {
		return
	}

	switch string(eventType) {
	case "callmemaybe":
		agentUUID, err := uuid.ParseBytes(agentID)
		if err != nil {
			c.log.Error(ctx, "invalid agent id", slog.F("id", string(agentID)))
			return
		}

		c.mutex.Lock()
		agentSocket, ok := c.agentSockets[agentUUID]
		c.mutex.Unlock()
		if !ok {
			return
		}

		// Socket takes a slice of Nodes, so we need to parse the JSON here.
		var nodes []*agpl.Node
		err = json.Unmarshal(nodeJSON, &nodes)
		if err != nil {
			c.log.Error(ctx, "invalid nodes JSON", slog.F("id", agentID), slog.Error(err), slog.F("node", string(nodeJSON)))
		}
		err = agentSocket.Enqueue(nodes)
		if err != nil {
			c.log.Error(ctx, "send callmemaybe to agent", slog.Error(err))
			return
		}
	case "clienthello":
		agentUUID, err := uuid.ParseBytes(agentID)
		if err != nil {
			c.log.Error(ctx, "invalid agent id", slog.F("id", string(agentID)))
			return
		}

		err = c.handleClientHello(agentUUID)
		if err != nil {
			c.log.Error(ctx, "handle agent request node", slog.Error(err))
			return
		}
	case "agenthello":
		agentUUID, err := uuid.ParseBytes(agentID)
		if err != nil {
			c.log.Error(ctx, "invalid agent id", slog.F("id", string(agentID)))
			return
		}

		c.mutex.RLock()
		nodes := c.nodesSubscribedToAgent(agentUUID)
		c.mutex.RUnlock()
		if len(nodes) > 0 {
			err := c.publishNodesToAgent(agentUUID, nodes)
			if err != nil {
				c.log.Error(ctx, "publish nodes to agent", slog.Error(err))
				return
			}
		}
	case "agentupdate":
		agentUUID, err := uuid.ParseBytes(agentID)
		if err != nil {
			c.log.Error(ctx, "invalid agent id", slog.F("id", string(agentID)))
			return
		}

		decoder := json.NewDecoder(bytes.NewReader(nodeJSON))
		_, err = c.handleAgentUpdate(agentUUID, decoder)
		if err != nil {
			c.log.Error(ctx, "handle agent update", slog.Error(err))
			return
		}
	default:
		c.log.Error(ctx, "unknown peer event", slog.F("name", string(eventType)))
	}
}

// format: <coordinator id>|callmemaybe|<recipient id>|<node json>
func (c *haCoordinator) formatCallMeMaybe(recipient uuid.UUID, nodes []*agpl.Node) ([]byte, error) {
	buf := bytes.Buffer{}

	_, _ = buf.WriteString(c.id.String() + "|")
	_, _ = buf.WriteString("callmemaybe|")
	_, _ = buf.WriteString(recipient.String() + "|")
	err := json.NewEncoder(&buf).Encode(nodes)
	if err != nil {
		return nil, xerrors.Errorf("encode node: %w", err)
	}

	return buf.Bytes(), nil
}

// format: <coordinator id>|agenthello|<node id>|
func (c *haCoordinator) formatAgentHello(id uuid.UUID) ([]byte, error) {
	buf := bytes.Buffer{}

	_, _ = buf.WriteString(c.id.String() + "|")
	_, _ = buf.WriteString("agenthello|")
	_, _ = buf.WriteString(id.String() + "|")

	return buf.Bytes(), nil
}

// format: <coordinator id>|clienthello|<agent id>|
func (c *haCoordinator) formatClientHello(id uuid.UUID) ([]byte, error) {
	buf := bytes.Buffer{}

	_, _ = buf.WriteString(c.id.String() + "|")
	_, _ = buf.WriteString("clienthello|")
	_, _ = buf.WriteString(id.String() + "|")

	return buf.Bytes(), nil
}

// format: <coordinator id>|agentupdate|<node id>|<node json>
func (c *haCoordinator) formatAgentUpdate(id uuid.UUID, node *agpl.Node) ([]byte, error) {
	buf := bytes.Buffer{}

	_, _ = buf.WriteString(c.id.String() + "|")
	_, _ = buf.WriteString("agentupdate|")
	_, _ = buf.WriteString(id.String() + "|")
	err := json.NewEncoder(&buf).Encode(node)
	if err != nil {
		return nil, xerrors.Errorf("encode node: %w", err)
	}

	return buf.Bytes(), nil
}

func (c *haCoordinator) ServeHTTPDebug(w http.ResponseWriter, r *http.Request) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	agpl.CoordinatorHTTPDebug(
		agpl.HTTPDebugFromLocal(true, c.agentSockets, c.agentToConnectionSockets, c.nodes, c.agentNameCache),
	)(w, r)
}
