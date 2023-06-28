package tailnet

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"cdr.dev/slog"
)

type MultiAgentConn interface {
	UpdateSelf(node *Node) error
	SubscribeAgent(agentID uuid.UUID, node *Node) error
	UnsubscribeAgent(agentID uuid.UUID)
	NextUpdate(ctx context.Context) []AgentNode
	AgentIsLegacy(agentID uuid.UUID) bool
	Close() error
}

type MultiAgent struct {
	mu sync.RWMutex

	ID     uuid.UUID
	Logger slog.Logger

	AgentIsLegacyFunc func(agentID uuid.UUID) bool
	OnNodeUpdate      func(id uuid.UUID, agents []uuid.UUID, node *Node) error
	OnClose           func(id uuid.UUID)

	updates          chan AgentNode
	subscribedAgents map[uuid.UUID]struct{}
}

type AgentNode struct {
	AgentID uuid.UUID
	*Node
}

func (m *MultiAgent) Init() *MultiAgent {
	m.updates = make(chan AgentNode, 128)
	m.subscribedAgents = map[uuid.UUID]struct{}{}
	return m
}

func (m *MultiAgent) AgentIsLegacy(agentID uuid.UUID) bool {
	return m.AgentIsLegacyFunc(agentID)
}

func (m *MultiAgent) OnAgentUpdate(id uuid.UUID, node *Node) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, ok := m.subscribedAgents[id]; !ok {
		return
	}

	select {
	case m.updates <- AgentNode{AgentID: id, Node: node}:
	default:
		m.Logger.Debug(context.Background(), "unable to send node %q to multiagent %q; buffer full", id, m.ID)
	}
}

func (m *MultiAgent) UpdateSelf(node *Node) error {
	m.mu.Lock()
	agents := make([]uuid.UUID, 0, len(m.subscribedAgents))
	for agent := range m.subscribedAgents {
		agents = append(agents, agent)
	}
	m.mu.Unlock()

	return m.OnNodeUpdate(m.ID, agents, node)
}

func (m *MultiAgent) SubscribeAgent(agentID uuid.UUID, node *Node) error {
	m.mu.Lock()
	m.subscribedAgents[agentID] = struct{}{}
	m.mu.Unlock()

	return m.OnNodeUpdate(m.ID, []uuid.UUID{agentID}, node)
}

func (m *MultiAgent) UnsubscribeAgent(agentID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.subscribedAgents, agentID)
}

func (m *MultiAgent) NextUpdate(ctx context.Context) []AgentNode {
	var nodes []AgentNode

loop:
	// Read all buffered nodes.
	for {
		select {
		case <-ctx.Done():
			return nil

		case node := <-m.updates:
			nodes = append(nodes, node)

		default:
			break loop
		}
	}

	return nodes
}

func (m *MultiAgent) Close() error {
	m.mu.Lock()
	close(m.updates)
	m.mu.Unlock()

	m.OnClose(m.ID)

	return nil
}
