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
	"sync"
	"time"

	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	agpl "github.com/coder/coder/tailnet"
)

// NewCoordinator creates a new high availability coordinator
// that uses PostgreSQL pubsub to exchange handshakes.
func NewCoordinator(logger slog.Logger, pubsub database.Pubsub) (agpl.Coordinator, error) {
	ctx, cancelFunc := context.WithCancel(context.Background())

	nameCache, err := lru.New[uuid.UUID, string](512)
	if err != nil {
		panic("make lru cache: " + err.Error())
	}

	coord := &haCoordinator{
		id:                       uuid.New(),
		log:                      logger,
		pubsub:                   pubsub,
		closeFunc:                cancelFunc,
		close:                    make(chan struct{}),
		nodes:                    map[uuid.UUID]*agpl.Node{},
		agentSockets:             map[uuid.UUID]*agpl.TrackedConn{},
		agentToConnectionSockets: map[uuid.UUID]map[uuid.UUID]*agpl.TrackedConn{},
		agentNameCache:           nameCache,
	}

	if err := coord.runPubsub(ctx); err != nil {
		return nil, xerrors.Errorf("run coordinator pubsub: %w", err)
	}

	return coord, nil
}

type haCoordinator struct {
	id        uuid.UUID
	log       slog.Logger
	mutex     sync.RWMutex
	pubsub    database.Pubsub
	close     chan struct{}
	closeFunc context.CancelFunc

	// nodes maps agent and connection IDs their respective node.
	nodes map[uuid.UUID]*agpl.Node
	// agentSockets maps agent IDs to their open websocket.
	agentSockets map[uuid.UUID]*agpl.TrackedConn
	// agentToConnectionSockets maps agent IDs to connection IDs of conns that
	// are subscribed to updates for that agent.
	agentToConnectionSockets map[uuid.UUID]map[uuid.UUID]*agpl.TrackedConn

	// agentNameCache holds a cache of agent names. If one of them disappears,
	// it's helpful to have a name cached for debugging.
	agentNameCache *lru.Cache[uuid.UUID, string]
}

// Node returns an in-memory node by ID.
func (c *haCoordinator) Node(id uuid.UUID) *agpl.Node {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	node := c.nodes[id]
	return node
}

// ServeClient accepts a WebSocket connection that wants to connect to an agent
// with the specified ID.
func (c *haCoordinator) ServeClient(conn net.Conn, id uuid.UUID, agent uuid.UUID) error {
	c.mutex.Lock()
	// When a new connection is requested, we update it with the latest
	// node of the agent. This allows the connection to establish.
	node, ok := c.nodes[agent]
	c.mutex.Unlock()
	if ok {
		data, err := json.Marshal([]*agpl.Node{node})
		if err != nil {
			return xerrors.Errorf("marshal node: %w", err)
		}
		_, err = conn.Write(data)
		if err != nil {
			return xerrors.Errorf("write nodes: %w", err)
		}
	} else {
		err := c.publishClientHello(agent)
		if err != nil {
			return xerrors.Errorf("publish client hello: %w", err)
		}
	}

	c.mutex.Lock()
	connectionSockets, ok := c.agentToConnectionSockets[agent]
	if !ok {
		connectionSockets = map[uuid.UUID]*agpl.TrackedConn{}
		c.agentToConnectionSockets[agent] = connectionSockets
	}

	now := time.Now().Unix()
	// Insert this connection into a map so the agent
	// can publish node updates.
	connectionSockets[id] = &agpl.TrackedConn{
		Conn:      conn,
		Start:     now,
		LastWrite: now,
	}
	c.mutex.Unlock()

	defer func() {
		c.mutex.Lock()
		defer c.mutex.Unlock()
		// Clean all traces of this connection from the map.
		delete(c.nodes, id)
		connectionSockets, ok := c.agentToConnectionSockets[agent]
		if !ok {
			return
		}
		delete(connectionSockets, id)
		if len(connectionSockets) != 0 {
			return
		}
		delete(c.agentToConnectionSockets, agent)
	}()

	decoder := json.NewDecoder(conn)
	// Indefinitely handle messages from the client websocket.
	for {
		err := c.handleNextClientMessage(id, agent, decoder)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
				return nil
			}
			return xerrors.Errorf("handle next client message: %w", err)
		}
	}
}

func (c *haCoordinator) handleNextClientMessage(id, agent uuid.UUID, decoder *json.Decoder) error {
	var node agpl.Node
	err := decoder.Decode(&node)
	if err != nil {
		return xerrors.Errorf("read json: %w", err)
	}

	c.mutex.Lock()
	// Update the node of this client in our in-memory map. If an agent entirely
	// shuts down and reconnects, it needs to be aware of all clients attempting
	// to establish connections.
	c.nodes[id] = &node
	// Write the new node from this client to the actively connected agent.
	agentSocket, ok := c.agentSockets[agent]
	c.mutex.Unlock()
	if !ok {
		// If we don't own the agent locally, send it over pubsub to a node that
		// owns the agent.
		err := c.publishNodesToAgent(agent, []*agpl.Node{&node})
		if err != nil {
			return xerrors.Errorf("publish node to agent")
		}
		return nil
	}

	// Write the new node from this client to the actively
	// connected agent.
	data, err := json.Marshal([]*agpl.Node{&node})
	if err != nil {
		return xerrors.Errorf("marshal nodes: %w", err)
	}

	_, err = agentSocket.Write(data)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
			return nil
		}
		return xerrors.Errorf("write json: %w", err)
	}

	return nil
}

// ServeAgent accepts a WebSocket connection to an agent that listens to
// incoming connections and publishes node updates.
func (c *haCoordinator) ServeAgent(conn net.Conn, id uuid.UUID, name string) error {
	c.agentNameCache.Add(id, name)

	// Tell clients on other instances to send a callmemaybe to us.
	err := c.publishAgentHello(id)
	if err != nil {
		return xerrors.Errorf("publish agent hello: %w", err)
	}

	// Publish all nodes on this instance that want to connect to this agent.
	nodes := c.nodesSubscribedToAgent(id)
	if len(nodes) > 0 {
		data, err := json.Marshal(nodes)
		if err != nil {
			return xerrors.Errorf("marshal json: %w", err)
		}
		_, err = conn.Write(data)
		if err != nil {
			return xerrors.Errorf("write nodes: %w", err)
		}
	}

	// This uniquely identifies a connection that belongs to this goroutine.
	unique := uuid.New()
	now := time.Now().Unix()
	overwrites := int64(0)

	// If an old agent socket is connected, we close it
	// to avoid any leaks. This shouldn't ever occur because
	// we expect one agent to be running.
	c.mutex.Lock()
	oldAgentSocket, ok := c.agentSockets[id]
	if ok {
		overwrites = oldAgentSocket.Overwrites + 1
		_ = oldAgentSocket.Close()
	}
	c.agentSockets[id] = &agpl.TrackedConn{
		ID:   unique,
		Conn: conn,

		Name:       name,
		Start:      now,
		LastWrite:  now,
		Overwrites: overwrites,
	}
	c.mutex.Unlock()

	defer func() {
		c.mutex.Lock()
		defer c.mutex.Unlock()

		// Only delete the connection if it's ours. It could have been
		// overwritten.
		if idConn, ok := c.agentSockets[id]; ok && idConn.ID == unique {
			delete(c.agentSockets, id)
			delete(c.nodes, id)
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
	c.mutex.Lock()
	defer c.mutex.Unlock()
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

func (c *haCoordinator) handleAgentUpdate(id uuid.UUID, decoder *json.Decoder) (*agpl.Node, error) {
	var node agpl.Node
	err := decoder.Decode(&node)
	if err != nil {
		return nil, xerrors.Errorf("read json: %w", err)
	}

	c.mutex.Lock()
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

	data, err := json.Marshal([]*agpl.Node{&node})
	if err != nil {
		c.mutex.Unlock()
		return nil, xerrors.Errorf("marshal nodes: %w", err)
	}

	// Publish the new node to every listening socket.
	var wg sync.WaitGroup
	wg.Add(len(connectionSockets))
	for _, connectionSocket := range connectionSockets {
		connectionSocket := connectionSocket
		go func() {
			defer wg.Done()
			_ = connectionSocket.SetWriteDeadline(time.Now().Add(5 * time.Second))
			_, _ = connectionSocket.Write(data)
		}()
	}
	c.mutex.Unlock()
	wg.Wait()
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
			var message []byte
			select {
			case <-ctx.Done():
				return
			case message = <-messageQueue:
			}
			c.handlePubsubMessage(ctx, message)
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
		if !ok {
			c.mutex.Unlock()
			return
		}
		c.mutex.Unlock()

		// We get a single node over pubsub, so turn into an array.
		_, err = agentSocket.Write(nodeJSON)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
				return
			}
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

		nodes := c.nodesSubscribedToAgent(agentUUID)
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	_, _ = fmt.Fprintln(w, "<h1>high-availability wireguard coordinator debug</h1>")
	_, _ = fmt.Fprintln(w, "<h4 style=\"margin-top:-25px\">warning: this only provides info from the node that served the request, if there are multiple replicas this data may be incomplete</h4>")

	agpl.CoordinatorHTTPDebug(c.agentSockets, c.agentToConnectionSockets, c.agentNameCache)(w, r)
}
