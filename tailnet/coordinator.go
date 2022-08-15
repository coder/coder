package tailnet

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// ServeCoordinator matches the RW structure of a coordinator to exchange node messages.
func ServeCoordinator(ctx context.Context, socket *websocket.Conn, updateNodes func(node []*Node) error) (func(node *Node), <-chan error) {
	errChan := make(chan error, 1)
	go func() {
		for {
			var nodes []*Node
			err := wsjson.Read(ctx, socket, &nodes)
			if err != nil {
				errChan <- xerrors.Errorf("read: %w", err)
				return
			}
			err = updateNodes(nodes)
			if err != nil {
				errChan <- xerrors.Errorf("update nodes: %w", err)
			}
		}
	}()

	return func(node *Node) {
		err := wsjson.Write(ctx, socket, node)
		if errors.Is(err, context.Canceled) || errors.As(err, &websocket.CloseError{}) {
			errChan <- nil
			return
		}
		if err != nil {
			errChan <- xerrors.Errorf("write: %w", err)
		}
	}, errChan
}

// NewCoordinator constructs a new in-memory connection coordinator.
func NewCoordinator() *Coordinator {
	return &Coordinator{
		nodes:                    map[uuid.UUID]*Node{},
		agentSockets:             map[uuid.UUID]*websocket.Conn{},
		agentToConnectionSockets: map[uuid.UUID]map[uuid.UUID]*websocket.Conn{},
	}
}

// Coordinator exchanges nodes with agents to establish connections.
// ┌──────────────────┐   ┌────────────────────┐   ┌───────────────────┐   ┌──────────────────┐
// │tailnet.Coordinate├──►│tailnet.AcceptClient│◄─►│tailnet.AcceptAgent│◄──┤tailnet.Coordinate│
// └──────────────────┘   └────────────────────┘   └───────────────────┘   └──────────────────┘
// This coordinator is incompatible with multiple Coder
// replicas as all node data is in-memory.
type Coordinator struct {
	mutex sync.Mutex

	// Maps agent and connection IDs to a node.
	nodes map[uuid.UUID]*Node
	// Maps agent ID to an open socket.
	agentSockets map[uuid.UUID]*websocket.Conn
	// Maps agent ID to connection ID for sending
	// new node data as it comes in!
	agentToConnectionSockets map[uuid.UUID]map[uuid.UUID]*websocket.Conn
}

// Node returns an in-memory node by ID.
func (c *Coordinator) Node(id uuid.UUID) *Node {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	node := c.nodes[id]
	return node
}

// ServeClient accepts a WebSocket connection that wants to
// connect to an agent with the specified ID.
func (c *Coordinator) ServeClient(ctx context.Context, socket *websocket.Conn, id uuid.UUID, agent uuid.UUID) error {
	c.mutex.Lock()
	// When a new connection is requested, we update it with the latest
	// node of the agent. This allows the connection to establish.
	node, ok := c.nodes[agent]
	if ok {
		err := wsjson.Write(ctx, socket, []*Node{node})
		if err != nil {
			c.mutex.Unlock()
			return xerrors.Errorf("write nodes: %w", err)
		}
	}
	connectionSockets, ok := c.agentToConnectionSockets[agent]
	if !ok {
		connectionSockets = map[uuid.UUID]*websocket.Conn{}
		c.agentToConnectionSockets[agent] = connectionSockets
	}
	// Insert this connection into a map so the agent
	// can publish node updates.
	connectionSockets[id] = socket
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

	for {
		var node Node
		err := wsjson.Read(ctx, socket, &node)
		if errors.Is(err, context.Canceled) || errors.As(err, &websocket.CloseError{}) {
			return nil
		}
		if err != nil {
			return xerrors.Errorf("read json: %w", err)
		}
		c.mutex.Lock()
		// Update the node of this client in our in-memory map.
		// If an agent entirely shuts down and reconnects, it
		// needs to be aware of all clients attempting to
		// establish connections.
		c.nodes[id] = &node
		agentSocket, ok := c.agentSockets[agent]
		if !ok {
			c.mutex.Unlock()
			continue
		}
		// Write the new node from this client to the actively
		// connected agent.
		err = wsjson.Write(ctx, agentSocket, []*Node{&node})
		if errors.Is(err, context.Canceled) {
			c.mutex.Unlock()
			return nil
		}
		if err != nil {
			c.mutex.Unlock()
			return xerrors.Errorf("write json: %w", err)
		}
		c.mutex.Unlock()
	}
}

// ServeAgent accepts a WebSocket connection to an agent that
// listens to incoming connections and publishes node updates.
func (c *Coordinator) ServeAgent(ctx context.Context, socket *websocket.Conn, id uuid.UUID) error {
	c.mutex.Lock()
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
		err := wsjson.Write(ctx, socket, nodes)
		if err != nil {
			c.mutex.Unlock()
			return xerrors.Errorf("write nodes: %w", err)
		}
	}

	// If an old agent socket is connected, we close it
	// to avoid any leaks. This shouldn't ever occur because
	// we expect one agent to be running.
	oldAgentSocket, ok := c.agentSockets[id]
	if ok {
		_ = oldAgentSocket.Close(websocket.StatusNormalClosure, "another agent connected with the same id")
	}
	c.agentSockets[id] = socket
	c.mutex.Unlock()
	defer func() {
		c.mutex.Lock()
		defer c.mutex.Unlock()
		delete(c.agentSockets, id)
		delete(c.nodes, id)
	}()

	for {
		var node Node
		err := wsjson.Read(ctx, socket, &node)
		if errors.Is(err, context.Canceled) || errors.As(err, &websocket.CloseError{}) {
			return nil
		}
		if err != nil {
			return xerrors.Errorf("read json: %w", err)
		}
		c.mutex.Lock()
		c.nodes[id] = &node
		connectionSockets, ok := c.agentToConnectionSockets[id]
		if !ok {
			c.mutex.Unlock()
			continue
		}
		// Publish the new node to every listening socket.
		var wg sync.WaitGroup
		wg.Add(len(connectionSockets))
		for _, connectionSocket := range connectionSockets {
			connectionSocket := connectionSocket
			go func() {
				_ = wsjson.Write(ctx, connectionSocket, []*Node{&node})
				wg.Done()
			}()
		}
		wg.Wait()
		c.mutex.Unlock()
	}
}
