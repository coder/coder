package tailnet

import (
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
}

// Node represents a node in the network.
type Node struct {
	// ID is used to identify the connection.
	ID tailcfg.NodeID `json:"id"`
	// AsOf is the time the node was created.
	AsOf time.Time `json:"as_of"`
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

// NewCoordinator constructs a new in-memory connection coordinator. This
// coordinator is incompatible with multiple Coder replicas as all node data is
// in-memory.
func NewCoordinator() Coordinator {
	cache, err := lru.New[uuid.UUID, string](512)
	if err != nil {
		panic("make lru cache: " + err.Error())
	}

	return &coordinator{
		closed:                   false,
		nodes:                    map[uuid.UUID]*Node{},
		agentSockets:             map[uuid.UUID]*trackedConn{},
		agentToConnectionSockets: map[uuid.UUID]map[uuid.UUID]*trackedConn{},
		agentNameCache:           cache,
	}
}

// coordinator exchanges nodes with agents to establish connections entirely in-memory.
// The Enterprise implementation provides this for high-availability.
// ┌──────────────────┐   ┌────────────────────┐   ┌───────────────────┐   ┌──────────────────┐
// │tailnet.Coordinate├──►│tailnet.AcceptClient│◄─►│tailnet.AcceptAgent│◄──┤tailnet.Coordinate│
// └──────────────────┘   └────────────────────┘   └───────────────────┘   └──────────────────┘
// This coordinator is incompatible with multiple Coder
// replicas as all node data is in-memory.
type coordinator struct {
	mutex  sync.RWMutex
	closed bool

	// nodes maps agent and connection IDs their respective node.
	nodes map[uuid.UUID]*Node
	// agentSockets maps agent IDs to their open websocket.
	agentSockets map[uuid.UUID]*trackedConn
	// agentToConnectionSockets maps agent IDs to connection IDs of conns that
	// are subscribed to updates for that agent.
	agentToConnectionSockets map[uuid.UUID]map[uuid.UUID]*trackedConn

	// agentNameCache holds a cache of agent names. If one of them disappears,
	// it's helpful to have a name cached for debugging.
	agentNameCache *lru.Cache[uuid.UUID, string]
}

type trackedConn struct {
	net.Conn

	// id is an ephemeral UUID used to uniquely identify the owner of the
	// connection.
	id uuid.UUID

	name       string
	start      int64
	lastWrite  int64
	overwrites int64
}

func (t *trackedConn) Write(b []byte) (n int, err error) {
	atomic.StoreInt64(&t.lastWrite, time.Now().Unix())
	return t.Conn.Write(b)
}

// Node returns an in-memory node by ID.
// If the node does not exist, nil is returned.
func (c *coordinator) Node(id uuid.UUID) *Node {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.nodes[id]
}

func (c *coordinator) NodeCount() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return len(c.nodes)
}

func (c *coordinator) AgentCount() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return len(c.agentSockets)
}

// ServeClient accepts a WebSocket connection that wants to connect to an agent
// with the specified ID.
func (c *coordinator) ServeClient(conn net.Conn, id uuid.UUID, agent uuid.UUID) error {
	c.mutex.Lock()
	if c.closed {
		c.mutex.Unlock()
		return xerrors.New("coordinator is closed")
	}

	// When a new connection is requested, we update it with the latest
	// node of the agent. This allows the connection to establish.
	node, ok := c.nodes[agent]
	c.mutex.Unlock()
	if ok {
		data, err := json.Marshal([]*Node{node})
		if err != nil {
			return xerrors.Errorf("marshal node: %w", err)
		}
		_, err = conn.Write(data)
		if err != nil {
			return xerrors.Errorf("write nodes: %w", err)
		}
	}
	c.mutex.Lock()
	connectionSockets, ok := c.agentToConnectionSockets[agent]
	if !ok {
		connectionSockets = map[uuid.UUID]*trackedConn{}
		c.agentToConnectionSockets[agent] = connectionSockets
	}

	now := time.Now().Unix()
	// Insert this connection into a map so the agent
	// can publish node updates.
	connectionSockets[id] = &trackedConn{
		Conn:      conn,
		start:     now,
		lastWrite: now,
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

func (c *coordinator) handleNextClientMessage(id, agent uuid.UUID, decoder *json.Decoder) error {
	var node Node
	err := decoder.Decode(&node)
	if err != nil {
		return xerrors.Errorf("read json: %w", err)
	}

	c.mutex.Lock()
	// Update the node of this client in our in-memory map. If an agent entirely
	// shuts down and reconnects, it needs to be aware of all clients attempting
	// to establish connections.
	c.nodes[id] = &node

	agentSocket, ok := c.agentSockets[agent]
	if !ok {
		c.mutex.Unlock()
		return nil
	}
	c.mutex.Unlock()

	// Write the new node from this client to the actively connected agent.
	data, err := json.Marshal([]*Node{&node})
	if err != nil {
		return xerrors.Errorf("marshal nodes: %w", err)
	}

	_, err = agentSocket.Write(data)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, context.Canceled) {
			return nil
		}
		return xerrors.Errorf("write json: %w", err)
	}

	return nil
}

// ServeAgent accepts a WebSocket connection to an agent that
// listens to incoming connections and publishes node updates.
func (c *coordinator) ServeAgent(conn net.Conn, id uuid.UUID, name string) error {
	c.mutex.Lock()
	if c.closed {
		c.mutex.Unlock()
		return xerrors.New("coordinator is closed")
	}

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
		c.mutex.Unlock()
		data, err := json.Marshal(nodes)
		if err != nil {
			return xerrors.Errorf("marshal json: %w", err)
		}
		_, err = conn.Write(data)
		if err != nil {
			return xerrors.Errorf("write nodes: %w", err)
		}
		c.mutex.Lock()
	}

	// This uniquely identifies a connection that belongs to this goroutine.
	unique := uuid.New()
	now := time.Now().Unix()
	overwrites := int64(0)

	// If an old agent socket is connected, we close it to avoid any leaks. This
	// shouldn't ever occur because we expect one agent to be running, but it's
	// possible for a race condition to happen when an agent is disconnected and
	// attempts to reconnect before the server realizes the old connection is
	// dead.
	oldAgentSocket, ok := c.agentSockets[id]
	if ok {
		overwrites = oldAgentSocket.overwrites + 1
		_ = oldAgentSocket.Close()
	}
	c.agentSockets[id] = &trackedConn{
		id:   unique,
		Conn: conn,

		name:       name,
		start:      now,
		lastWrite:  now,
		overwrites: overwrites,
	}

	c.mutex.Unlock()
	defer func() {
		c.mutex.Lock()
		defer c.mutex.Unlock()

		// Only delete the connection if it's ours. It could have been
		// overwritten.
		if idConn, ok := c.agentSockets[id]; ok && idConn.id == unique {
			delete(c.agentSockets, id)
			delete(c.nodes, id)
		}
	}()

	decoder := json.NewDecoder(conn)
	for {
		err := c.handleNextAgentMessage(id, decoder)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, context.Canceled) {
				return nil
			}
			return xerrors.Errorf("handle next agent message: %w", err)
		}
	}
}

func (c *coordinator) handleNextAgentMessage(id uuid.UUID, decoder *json.Decoder) error {
	var node Node
	err := decoder.Decode(&node)
	if err != nil {
		return xerrors.Errorf("read json: %w", err)
	}

	c.mutex.Lock()
	c.nodes[id] = &node
	connectionSockets, ok := c.agentToConnectionSockets[id]
	if !ok {
		c.mutex.Unlock()
		return nil
	}
	data, err := json.Marshal([]*Node{&node})
	if err != nil {
		c.mutex.Unlock()
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

	c.mutex.Unlock()
	wg.Wait()
	return nil
}

// Close closes all of the open connections in the coordinator and stops the
// coordinator from accepting new connections.
func (c *coordinator) Close() error {
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

func (c *coordinator) ServeHTTPDebug(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	now := time.Now()

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	fmt.Fprintln(w, "<h1>in-memory wireguard coordinator debug</h1>")

	type idConn struct {
		id   uuid.UUID
		conn *trackedConn
	}

	{
		fmt.Fprintf(w, "<h2 id=agents><a href=#agents>#</a> agents: total %d</h2>\n", len(c.agentSockets))
		fmt.Fprintln(w, "<ul>")
		agentSockets := make([]idConn, 0, len(c.agentSockets))

		for id, conn := range c.agentSockets {
			agentSockets = append(agentSockets, idConn{id, conn})
		}

		slices.SortFunc(agentSockets, func(a, b idConn) bool {
			return a.conn.name < b.conn.name
		})

		for _, agent := range agentSockets {
			fmt.Fprintf(w, "<li style=\"margin-top:4px\"><b>%s</b> (<code>%s</code>): created %v ago, write %v ago, overwrites %d </li>\n",
				agent.conn.name,
				agent.id.String(),
				now.Sub(time.Unix(agent.conn.start, 0)).Round(time.Second),
				now.Sub(time.Unix(agent.conn.lastWrite, 0)).Round(time.Second),
				agent.conn.overwrites,
			)

			if conns := c.agentToConnectionSockets[agent.id]; len(conns) > 0 {
				fmt.Fprintf(w, "<h3 style=\"margin:0px;font-size:16px;font-weight:400\">connections: total %d</h3>\n", len(conns))

				connSockets := make([]idConn, 0, len(conns))
				for id, conn := range conns {
					connSockets = append(connSockets, idConn{id, conn})
				}
				slices.SortFunc(connSockets, func(a, b idConn) bool {
					return a.id.String() < b.id.String()
				})

				fmt.Fprintln(w, "<ul>")
				for _, connSocket := range connSockets {
					fmt.Fprintf(w, "<li><b>%s</b> (<code>%s</code>): created %v ago, write %v ago </li>\n",
						connSocket.conn.name,
						connSocket.id.String(),
						now.Sub(time.Unix(connSocket.conn.start, 0)).Round(time.Second),
						now.Sub(time.Unix(connSocket.conn.lastWrite, 0)).Round(time.Second),
					)
				}
				fmt.Fprintln(w, "</ul>")
			}
		}

		fmt.Fprintln(w, "</ul>")
	}

	{
		type agentConns struct {
			id    uuid.UUID
			conns []idConn
		}

		missingAgents := []agentConns{}
		for agentID, conns := range c.agentToConnectionSockets {
			if len(conns) == 0 {
				continue
			}

			if _, ok := c.agentSockets[agentID]; !ok {
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

		fmt.Fprintf(w, "<h2 id=missing-agents><a href=#missing-agents>#</a> missing agents: total %d</h2>\n", len(missingAgents))
		fmt.Fprintln(w, "<ul>")

		for _, agentConns := range missingAgents {
			agentName, ok := c.agentNameCache.Get(agentConns.id)
			if !ok {
				agentName = "unknown"
			}

			fmt.Fprintf(w, "<li style=\"margin-top:4px\"><b>%s</b> (<code>%s</code>): created ? ago, write ? ago, overwrites ? </li>\n",
				agentName,
				agentConns.id.String(),
			)

			fmt.Fprintf(w, "<h3 style=\"margin:0px;font-size:16px;font-weight:400\">connections: total %d</h3>\n", len(agentConns.conns))
			fmt.Fprintln(w, "<ul>")
			for _, agentConn := range agentConns.conns {
				fmt.Fprintf(w, "<li><b>%s</b> (<code>%s</code>): created %v ago, write %v ago </li>\n",
					agentConn.conn.name,
					agentConn.id.String(),
					now.Sub(time.Unix(agentConn.conn.start, 0)).Round(time.Second),
					now.Sub(time.Unix(agentConn.conn.lastWrite, 0)).Round(time.Second),
				)
			}
			fmt.Fprintln(w, "</ul>")
		}
		fmt.Fprintln(w, "</ul>")
	}
}
