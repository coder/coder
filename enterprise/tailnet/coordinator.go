package tailnet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/coderd/database"
	agpl "github.com/coder/coder/tailnet"
)

func NewHACoordinator(logger slog.Logger, pubsub database.Pubsub) (agpl.Coordinator, error) {
	coord := &haCoordinator{
		id:                       uuid.New(),
		log:                      logger,
		pubsub:                   pubsub,
		close:                    make(chan struct{}),
		nodes:                    map[uuid.UUID]*agpl.Node{},
		agentSockets:             map[uuid.UUID]net.Conn{},
		agentToConnectionSockets: map[uuid.UUID]map[uuid.UUID]net.Conn{},
	}

	if err := coord.runPubsub(); err != nil {
		return nil, xerrors.Errorf("run coordinator pubsub: %w", err)
	}

	return coord, nil
}

type haCoordinator struct {
	id     uuid.UUID
	log    slog.Logger
	mutex  sync.RWMutex
	pubsub database.Pubsub
	close  chan struct{}

	// nodes maps agent and connection IDs their respective node.
	nodes map[uuid.UUID]*agpl.Node
	// agentSockets maps agent IDs to their open websocket.
	agentSockets map[uuid.UUID]net.Conn
	// agentToConnectionSockets maps agent IDs to connection IDs of conns that
	// are subscribed to updates for that agent.
	agentToConnectionSockets map[uuid.UUID]map[uuid.UUID]net.Conn
}

// Node returns an in-memory node by ID.
func (c *haCoordinator) Node(id uuid.UUID) *agpl.Node {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
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
	if ok {
		data, err := json.Marshal([]*agpl.Node{node})
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

	// Insert this connection into a map so the agent can publish node updates.
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
	// Indefinitely handle messages from the client websocket.
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

func (c *haCoordinator) handleNextClientMessage(id, agent uuid.UUID, decoder *json.Decoder) error {
	var node agpl.Node
	err := decoder.Decode(&node)
	if err != nil {
		return xerrors.Errorf("read json: %w", err)
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Update the node of this client in our in-memory map. If an agent entirely
	// shuts down and reconnects, it needs to be aware of all clients attempting
	// to establish connections.
	c.nodes[id] = &node

	// Write the new node from this client to the actively connected agent.
	err = c.writeNodeToAgent(agent, &node)
	if err != nil {
		return xerrors.Errorf("write node to agent: %w", err)
	}

	return nil
}

func (c *haCoordinator) writeNodeToAgent(agent uuid.UUID, node *agpl.Node) error {
	agentSocket, ok := c.agentSockets[agent]
	if !ok {
		// If we don't own the agent locally, send it over pubsub to a node that
		// owns the agent.
		err := c.publishNodeToAgent(agent, node)
		if err != nil {
			return xerrors.Errorf("publish node to agent")
		}
		return nil
	}

	// Write the new node from this client to the actively
	// connected agent.
	data, err := json.Marshal([]*agpl.Node{node})
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

// ServeAgent accepts a WebSocket connection to an agent that listens to
// incoming connections and publishes node updates.
func (c *haCoordinator) ServeAgent(conn net.Conn, id uuid.UUID) error {
	c.mutex.Lock()
	sockets, ok := c.agentToConnectionSockets[id]
	if ok {
		// Publish all nodes that want to connect to the
		// desired agent ID.
		nodes := make([]*agpl.Node, 0, len(sockets))
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
		err := c.hangleAgentUpdate(id, decoder, false)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return xerrors.Errorf("handle next agent message: %w", err)
		}
	}
}

func (c *haCoordinator) hangleAgentUpdate(id uuid.UUID, decoder *json.Decoder, fromPubsub bool) error {
	var node agpl.Node
	err := decoder.Decode(&node)
	if err != nil {
		return xerrors.Errorf("read json: %w", err)
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.nodes[id] = &node

	// Don't send the agent back over pubsub if that's where we received it from!
	if !fromPubsub {
		err = c.publishAgentToNodes(id, &node)
		if err != nil {
			return xerrors.Errorf("publish agent to nodes: %w", err)
		}
	}

	connectionSockets, ok := c.agentToConnectionSockets[id]
	if !ok {
		return nil
	}

	data, err := json.Marshal([]*agpl.Node{&node})
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

func (c *haCoordinator) Close() error {
	close(c.close)
	return nil
}

func (c *haCoordinator) publishNodeToAgent(recipient uuid.UUID, node *agpl.Node) error {
	msg, err := c.formatCallMeMaybe(recipient, node)
	if err != nil {
		return xerrors.Errorf("format publish message: %w", err)
	}

	fmt.Println("publishing callmemaybe", c.id.String())
	err = c.pubsub.Publish("wireguard_peers", msg)
	if err != nil {
		return xerrors.Errorf("publish message: %w", err)
	}

	return nil
}

func (c *haCoordinator) publishAgentToNodes(id uuid.UUID, node *agpl.Node) error {
	msg, err := c.formatAgentUpdate(id, node)
	if err != nil {
		return xerrors.Errorf("format publish message: %w", err)
	}

	fmt.Println("publishing agentupdate", c.id.String())
	err = c.pubsub.Publish("wireguard_peers", msg)
	if err != nil {
		return xerrors.Errorf("publish message: %w", err)
	}

	return nil
}

func (c *haCoordinator) runPubsub() error {
	cancelSub, err := c.pubsub.Subscribe("wireguard_peers", func(ctx context.Context, message []byte) {
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

			fmt.Println("got callmemaybe", agentUUID.String())
			c.mutex.Lock()
			defer c.mutex.Unlock()

			fmt.Println("process callmemaybe", agentUUID.String())
			agentSocket, ok := c.agentSockets[agentUUID]
			if !ok {
				fmt.Println("no socket")
				return
			}

			// We get a single node over pubsub, so turn into an array.
			_, err = agentSocket.Write(bytes.Join([][]byte{[]byte("["), nodeJSON, []byte("]")}, []byte{}))
			if err != nil {
				if errors.Is(err, io.EOF) {
					return
				}
				c.log.Error(ctx, "send callmemaybe to agent", slog.Error(err))
				return
			}
			fmt.Println("success callmemaybe", agentUUID.String())

		case "agentupdate":
			agentUUID, err := uuid.ParseBytes(agentID)
			if err != nil {
				c.log.Error(ctx, "invalid agent id", slog.F("id", string(agentID)))
			}

			decoder := json.NewDecoder(bytes.NewReader(nodeJSON))
			err = c.hangleAgentUpdate(agentUUID, decoder, true)
			if err != nil {
				c.log.Error(ctx, "handle agent update", slog.Error(err))
			}

		default:
			c.log.Error(ctx, "unknown peer event", slog.F("name", string(eventType)))
		}
	})
	if err != nil {
		return xerrors.Errorf("subscribe wireguard peers")
	}

	go func() {
		defer cancelSub()
		<-c.close
	}()

	return nil
}

// format: <coordinator id>|callmemaybe|<recipient id>|<node json>
func (c *haCoordinator) formatCallMeMaybe(recipient uuid.UUID, node *agpl.Node) ([]byte, error) {
	buf := bytes.Buffer{}

	buf.WriteString(c.id.String() + "|")
	buf.WriteString("callmemaybe|")
	buf.WriteString(recipient.String() + "|")
	err := json.NewEncoder(&buf).Encode(node)
	if err != nil {
		return nil, xerrors.Errorf("encode node: %w", err)
	}

	return buf.Bytes(), nil
}

// format: <coordinator id>|agentupdate|<node id>|<node json>
func (c *haCoordinator) formatAgentUpdate(id uuid.UUID, node *agpl.Node) ([]byte, error) {
	buf := bytes.Buffer{}

	buf.WriteString(c.id.String() + "|")
	buf.WriteString("agentupdate|")
	buf.WriteString(id.String() + "|")
	err := json.NewEncoder(&buf).Encode(node)
	if err != nil {
		return nil, xerrors.Errorf("encode node: %w", err)
	}

	return buf.Bytes(), nil
}
