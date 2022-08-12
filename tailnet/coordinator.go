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

// Coordinate matches the RW structure of a coordinator to exchange node messages.
func Coordinate(ctx context.Context, socket *websocket.Conn, updateNodes func(node []*Node)) (func(node *Node), <-chan error) {
	errChan := make(chan error, 1)
	go func() {
		for {
			var nodes []*Node
			err := wsjson.Read(ctx, socket, &nodes)
			if err != nil {
				errChan <- xerrors.Errorf("read: %w", err)
				return
			}
			updateNodes(nodes)
		}
	}()

	return func(node *Node) {
		err := wsjson.Write(ctx, socket, node)
		if err != nil {
			errChan <- xerrors.Errorf("write: %w", err)
		}
	}, errChan
}

// NewCoordinator constructs a new in-memory connection coordinator.
func NewCoordinator() *Coordinator {
	return &Coordinator{
		agentNodes:         map[uuid.UUID]*Node{},
		agentClientNodes:   map[uuid.UUID]map[uuid.UUID]*Node{},
		agentSockets:       map[uuid.UUID]*websocket.Conn{},
		agentClientSockets: map[uuid.UUID]map[uuid.UUID]*websocket.Conn{},
	}
}

// Coordinator brokers connections over WebSockets.
type Coordinator struct {
	mutex sync.Mutex
	// Stores the most recent node an agent sent.
	agentNodes map[uuid.UUID]*Node
	// Stores the most recent node reported by each client.
	agentClientNodes map[uuid.UUID]map[uuid.UUID]*Node
	// Stores the active connection from an agent.
	agentSockets map[uuid.UUID]*websocket.Conn
	// Stores the active connection from a client to an agent.
	agentClientSockets map[uuid.UUID]map[uuid.UUID]*websocket.Conn
}

// Client represents a tailnet looking to peer with an agent.
func (c *Coordinator) Client(ctx context.Context, agentID uuid.UUID, socket *websocket.Conn) error {
	id := uuid.New()
	c.mutex.Lock()
	clients, ok := c.agentClientSockets[agentID]
	if !ok {
		clients = map[uuid.UUID]*websocket.Conn{}
		c.agentClientSockets[agentID] = clients
	}
	clients[id] = socket
	agentNode, ok := c.agentNodes[agentID]
	if ok {
		err := wsjson.Write(ctx, socket, []*Node{agentNode})
		if err != nil {
			c.mutex.Unlock()
			return xerrors.Errorf("write agent node: %w", err)
		}
	}

	c.mutex.Unlock()
	defer func() {
		c.mutex.Lock()
		defer c.mutex.Unlock()
		clients, ok := c.agentClientSockets[agentID]
		if !ok {
			return
		}
		delete(clients, id)
		nodes, ok := c.agentClientNodes[agentID]
		if !ok {
			return
		}
		delete(nodes, id)
	}()

	for {
		var node Node
		err := wsjson.Read(ctx, socket, &node)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		if err != nil {
			return xerrors.Errorf("read json: %w", err)
		}
		c.mutex.Lock()
		nodes, ok := c.agentClientNodes[agentID]
		if !ok {
			nodes = map[uuid.UUID]*Node{}
			c.agentClientNodes[agentID] = nodes
		}
		nodes[id] = &node

		agentSocket, ok := c.agentSockets[agentID]
		if !ok {
			// If the agent isn't connected yet, that's fine. It'll reconcile later.
			c.mutex.Unlock()
			continue
		}
		err = wsjson.Write(ctx, agentSocket, []*Node{&node})
		if err != nil {
			c.mutex.Unlock()
			return xerrors.Errorf("write node to agent: %w", err)
		}
		c.mutex.Unlock()
	}
}

func (c *Coordinator) Agent(ctx context.Context, agentID uuid.UUID, socket *websocket.Conn) error {
	c.mutex.Lock()
	agentSocket, ok := c.agentSockets[agentID]
	if ok {
		agentSocket.Close(websocket.StatusGoingAway, "another agent started with the same id")
	}
	c.agentSockets[agentID] = socket
	nodes, ok := c.agentClientNodes[agentID]
	if ok {
		err := wsjson.Write(ctx, socket, nodes)
		if err != nil {
			c.mutex.Unlock()
			return xerrors.Errorf("write nodes: %w", err)
		}
	}
	c.mutex.Unlock()
	defer func() {
		c.mutex.Lock()
		defer c.mutex.Unlock()
		delete(c.agentSockets, agentID)
	}()

	for {
		var node Node
		err := wsjson.Read(ctx, socket, &node)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		if err != nil {
			return xerrors.Errorf("read node: %w", err)
		}
		c.mutex.Lock()
		c.agentNodes[agentID] = &node

		clients, ok := c.agentClientSockets[agentID]
		if !ok {
			c.mutex.Unlock()
			continue
		}
		for _, client := range clients {
			err = wsjson.Write(ctx, client, []*Node{&node})
			if err != nil {
				c.mutex.Unlock()
				return xerrors.Errorf("write to client: %w", err)
			}
		}
		c.mutex.Unlock()
	}
}
