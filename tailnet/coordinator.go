package tailnet

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
)

// Node represents a node in the network.
type Node struct {
	// ID is used to identify the connection.
	ID tailcfg.NodeID `json:"id"`
	// Key is the Wireguard public key of the node.
	Key key.NodePublic `json:"key"`
	// DiscoKey is used for discovery messages over DERP to establish peer-to-peer connections.
	DiscoKey key.DiscoPublic `json:"disco"`
	// PreferredDERP is the DERP server that peered connections
	// should meet at to establish.
	PreferredDERP int `json:"preferred_derp"`
	// DERPLatency is the latency in seconds to each DERP server.
	DERPLatency map[string]float64 `json:"derp_latency"`
	// Addresses are the IP address ranges this connection exposes.
	Addresses []netip.Prefix `json:"addresses"`
	// AllowedIPs specify what addresses can dial the connection.
	// We allow all by default.
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

// NewCoordinator constructs a new in-memory connection coordinator.
func NewCoordinator() *Coordinator {
	return &Coordinator{
		nodes:                    map[uuid.UUID]*Node{},
		agentSockets:             map[uuid.UUID]net.Conn{},
		agentToConnectionSockets: map[uuid.UUID]map[uuid.UUID]net.Conn{},
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
	agentSockets map[uuid.UUID]net.Conn
	// Maps agent ID to connection ID for sending
	// new node data as it comes in!
	agentToConnectionSockets map[uuid.UUID]map[uuid.UUID]net.Conn
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
func (c *Coordinator) ServeClient(conn net.Conn, id uuid.UUID, agent uuid.UUID) error {
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
		var node Node
		err := decoder.Decode(&node)
		if errors.Is(err, io.EOF) {
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
		c.mutex.Unlock()
		// Write the new node from this client to the actively
		// connected agent.
		data, err := json.Marshal([]*Node{&node})
		if err != nil {
			c.mutex.Unlock()
			return xerrors.Errorf("marshal nodes: %w", err)
		}
		_, err = agentSocket.Write(data)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return xerrors.Errorf("write json: %w", err)
		}
	}
}

// ServeAgent accepts a WebSocket connection to an agent that
// listens to incoming connections and publishes node updates.
func (c *Coordinator) ServeAgent(conn net.Conn, id uuid.UUID) error {
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
		var node Node
		err := decoder.Decode(&node)
		if errors.Is(err, io.EOF) {
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
				_, _ = connectionSocket.Write(data)
				wg.Done()
			}()
		}
		c.mutex.Unlock()
		wg.Wait()
	}
}
