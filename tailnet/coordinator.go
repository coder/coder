package tailnet

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/google/uuid"
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
	// Node returns an in-memory node by ID.
	Node(id uuid.UUID) *Node
	// ServeClient accepts a WebSocket connection that wants to connect to an agent
	// with the specified ID.
	ServeClient(conn net.Conn, id uuid.UUID, agent uuid.UUID) error
	// ServeAgent accepts a WebSocket connection to an agent that listens to
	// incoming connections and publishes node updates.
	ServeAgent(conn net.Conn, id uuid.UUID) error
	// Close closes the coordinator.
	Close() error
}

// Node represents a node in the network.
type Node struct {
	ID            tailcfg.NodeID     `json:"id"`
	Key           key.NodePublic     `json:"key"`
	DiscoKey      key.DiscoPublic    `json:"disco"`
	PreferredDERP int                `json:"preferred_derp"`
	DERPLatency   map[string]float64 `json:"derp_latency"`
	Addresses     []netip.Prefix     `json:"addresses"`
	AllowedIPs    []netip.Prefix     `json:"allowed_ips"`
	Endpoints     []string           `json:"endpoints"`
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

// NewMemoryCoordinator constructs a new in-memory connection coordinator. This
// coordinator is incompatible with multiple Coder replicas as all node data is
// in-memory.
func NewMemoryCoordinator() Coordinator {
	return &memoryCoordinator{
		nodes:                    map[uuid.UUID]*Node{},
		agentSockets:             map[uuid.UUID]net.Conn{},
		agentToConnectionSockets: map[uuid.UUID]map[uuid.UUID]net.Conn{},
	}
}

// MemoryCoordinator exchanges nodes with agents to establish connections.
// ┌──────────────────┐   ┌────────────────────┐   ┌───────────────────┐   ┌──────────────────┐
// │tailnet.Coordinate├──►│tailnet.AcceptClient│◄─►│tailnet.AcceptAgent│◄──┤tailnet.Coordinate│
// └──────────────────┘   └────────────────────┘   └───────────────────┘   └──────────────────┘
// This coordinator is incompatible with multiple Coder
// replicas as all node data is in-memory.
type memoryCoordinator struct {
	mutex sync.Mutex

	// nodes maps agent and connection IDs their respective node.
	nodes map[uuid.UUID]*Node
	// agentSockets maps agent IDs to their open websocket.
	agentSockets map[uuid.UUID]net.Conn
	// agentToConnectionSockets maps agent IDs to connection IDs of conns that
	// are subscribed to updates for that agent.
	agentToConnectionSockets map[uuid.UUID]map[uuid.UUID]net.Conn
}

// Node returns an in-memory node by ID.
func (c *memoryCoordinator) Node(id uuid.UUID) *Node {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	node := c.nodes[id]
	return node
}

// ServeClient accepts a WebSocket connection that wants to connect to an agent
// with the specified ID.
func (c *memoryCoordinator) ServeClient(conn net.Conn, id uuid.UUID, agent uuid.UUID) error {
	c.mutex.Lock()
	// When a new connection is requested, we update it with the latest
	// node of the agent. This allows the connection to establish.
	node, ok := c.nodes[agent]
	if ok {
		data, err := json.Marshal([]*Node{node})
		if err != nil {
			c.mutex.Unlock()
			return xerrors.Errorf("marshal node: %w", err)
		}
		_, err = conn.Write(data)
		if err != nil {
			c.mutex.Unlock()
			return xerrors.Errorf("write nodes: %w", err)
		}
	}
	connectionSockets, ok := c.agentToConnectionSockets[agent]
	if !ok {
		connectionSockets = map[uuid.UUID]net.Conn{}
		c.agentToConnectionSockets[agent] = connectionSockets
	}
	// Insert this connection into a map so the agent
	// can publish node updates.
	connectionSockets[id] = conn
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
	for {
		err := c.handleNextClientMessage(id, agent, decoder)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return xerrors.Errorf("handle next client message: %w", err)
		}
	}
}

func (c *memoryCoordinator) handleNextClientMessage(id, agent uuid.UUID, decoder *json.Decoder) error {
	var node Node
	err := decoder.Decode(&node)
	if err != nil {
		return xerrors.Errorf("read json: %w", err)
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Update the node of this client in our in-memory map. If an agent
	// entirely shuts down and reconnects, it needs to be aware of all clients
	// attempting to establish connections.
	c.nodes[id] = &node

	// Write the new node from this client to the actively
	// connected agent.
	err = c.writeNodeToAgent(agent, &node)
	if err != nil {
		return xerrors.Errorf("write node to agent: %w", err)
	}

	return nil
}

func (c *memoryCoordinator) writeNodeToAgent(agent uuid.UUID, node *Node) error {
	agentSocket, ok := c.agentSockets[agent]
	if !ok {
		return nil
	}

	// Write the new node from this client to the actively
	// connected agent.
	data, err := json.Marshal([]*Node{node})
	if err != nil {
		return xerrors.Errorf("marshal nodes: %w", err)
	}

	_, err = agentSocket.Write(data)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return xerrors.Errorf("write json: %w", err)
	}
	return nil
}

// ServeAgent accepts a WebSocket connection to an agent that
// listens to incoming connections and publishes node updates.
func (c *memoryCoordinator) ServeAgent(conn net.Conn, id uuid.UUID) error {
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
		data, err := json.Marshal(nodes)
		if err != nil {
			c.mutex.Unlock()
			return xerrors.Errorf("marshal json: %w", err)
		}
		_, err = conn.Write(data)
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
		_ = oldAgentSocket.Close()
	}
	c.agentSockets[id] = conn
	c.mutex.Unlock()
	defer func() {
		c.mutex.Lock()
		defer c.mutex.Unlock()
		delete(c.agentSockets, id)
		delete(c.nodes, id)
	}()

	decoder := json.NewDecoder(conn)
	for {
		err := c.handleNextAgentMessage(id, decoder)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return xerrors.Errorf("handle next agent message: %w", err)
		}
	}
}

func (c *memoryCoordinator) handleNextAgentMessage(id uuid.UUID, decoder *json.Decoder) error {
	var node Node
	err := decoder.Decode(&node)
	if err != nil {
		return xerrors.Errorf("read json: %w", err)
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.nodes[id] = &node
	connectionSockets, ok := c.agentToConnectionSockets[id]
	if !ok {
		return nil
	}

	data, err := json.Marshal([]*Node{&node})
	if err != nil {
		return xerrors.Errorf("marshal nodes: %w", err)
	}

	// Publish the new node to every listening socket.
	var wg sync.WaitGroup
	wg.Add(len(connectionSockets))
	for _, connectionSocket := range connectionSockets {
		connectionSocket := connectionSocket
		go func() {
			_ = connectionSocket.SetWriteDeadline(time.Now().Add(5 * time.Second))
			_, _ = connectionSocket.Write(data)
			wg.Done()
		}()
	}

	wg.Wait()
	return nil
}

func (*memoryCoordinator) Close() error { return nil }
