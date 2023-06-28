package tailnet

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"

	"cdr.dev/slog"
)

type MultiAgentConn interface {
	UpdateSelf(node *Node) error
	SubscribeAgent(agentID uuid.UUID) (func(), error)
	UnsubscribeAgent(agentID uuid.UUID)
	NextUpdate(ctx context.Context) []*Node
	AgentIsLegacy(agentID uuid.UUID) bool
	Close() error
}

type MultiAgent struct {
	mu sync.RWMutex

	ID     uuid.UUID
	Logger slog.Logger

	AgentIsLegacyFunc func(agentID uuid.UUID) bool
	OnSubscribe       func(enq Enqueueable, agent uuid.UUID) (close func(), err error)
	OnNodeUpdate      func(id uuid.UUID, agents []uuid.UUID, node *Node) error

	updates          chan []*Node
	subscribedAgents map[uuid.UUID]func()
}

func (m *MultiAgent) Init() *MultiAgent {
	m.updates = make(chan []*Node, 128)
	m.subscribedAgents = map[uuid.UUID]func(){}
	return m
}

func (m *MultiAgent) AgentIsLegacy(agentID uuid.UUID) bool {
	return m.AgentIsLegacyFunc(agentID)
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

func (m *MultiAgent) SubscribeAgent(agentID uuid.UUID) (func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if closer, ok := m.subscribedAgents[agentID]; ok {
		return closer, nil
	}

	closer, err := m.OnSubscribe(m.enqueuer(agentID), agentID)
	if err != nil {
		return nil, err
	}
	m.subscribedAgents[agentID] = closer
	return closer, nil
}

func (m *MultiAgent) UnsubscribeAgent(agentID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if closer, ok := m.subscribedAgents[agentID]; ok {
		closer()
	}
	delete(m.subscribedAgents, agentID)
}

func (m *MultiAgent) NextUpdate(ctx context.Context) []*Node {
	for {
		select {
		case <-ctx.Done():
			return nil

		case nodes := <-m.updates:
			return nodes
		}
	}
}

func (m *MultiAgent) enqueuer(agentID uuid.UUID) Enqueueable {
	return &multiAgentEnqueuer{
		agentID: agentID,
		m:       m,
	}
}

type multiAgentEnqueuer struct {
	m *MultiAgent

	agentID    uuid.UUID
	start      int64
	lastWrite  int64
	overwrites int64
}

func (m *multiAgentEnqueuer) UniqueID() uuid.UUID {
	return m.m.ID
}

func (m *multiAgentEnqueuer) Enqueue(nodes []*Node) error {
	select {
	case m.m.updates <- nodes:
		return nil
	default:
		return ErrWouldBlock
	}
}

func (m *multiAgentEnqueuer) Name() string {
	return "multiagent-" + m.m.ID.String()
}

func (m *multiAgentEnqueuer) Stats() (start int64, lastWrite int64) {
	return m.start, atomic.LoadInt64(&m.lastWrite)
}

func (m *multiAgentEnqueuer) Overwrites() int64 {
	return m.overwrites
}

func (m *multiAgentEnqueuer) Close() error {
	m.m.mu.Lock()
	defer m.m.mu.Unlock()

	// Delete without running the closer. If the enqueuer itself gets closed, we
	// can assume that the caller is removing it from the coordinator.
	delete(m.m.subscribedAgents, m.agentID)
	return nil
}

func (m *MultiAgent) Close() error {
	m.mu.Lock()
	close(m.updates)
	for _, closer := range m.subscribedAgents {
		closer()
	}
	m.mu.Unlock()
	return nil
}
