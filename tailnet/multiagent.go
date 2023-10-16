package tailnet

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

type MultiAgentConn interface {
	UpdateSelf(node *Node) error
	SubscribeAgent(agentID uuid.UUID) error
	UnsubscribeAgent(agentID uuid.UUID) error
	NextUpdate(ctx context.Context) ([]*Node, bool)
	AgentIsLegacy(agentID uuid.UUID) bool
	Close() error
	IsClosed() bool
}

type MultiAgent struct {
	mu sync.RWMutex

	ID uuid.UUID

	AgentIsLegacyFunc func(agentID uuid.UUID) bool
	OnSubscribe       func(enq Queue, agent uuid.UUID) (*Node, error)
	OnUnsubscribe     func(enq Queue, agent uuid.UUID) error
	OnNodeUpdate      func(id uuid.UUID, node *Node) error
	OnRemove          func(enq Queue)

	ctx       context.Context
	ctxCancel func()
	closed    bool

	updates   chan []*Node
	closeOnce sync.Once
	start     int64
	lastWrite int64
	// Client nodes normally generate a unique id for each connection so
	// overwrites are really not an issue, but is provided for compatibility.
	overwrites int64
}

func (m *MultiAgent) Init() *MultiAgent {
	m.updates = make(chan []*Node, 128)
	m.start = time.Now().Unix()
	m.ctx, m.ctxCancel = context.WithCancel(context.Background())
	return m
}

func (*MultiAgent) Kind() QueueKind {
	return QueueKindClient
}

func (m *MultiAgent) UniqueID() uuid.UUID {
	return m.ID
}

func (m *MultiAgent) AgentIsLegacy(agentID uuid.UUID) bool {
	return m.AgentIsLegacyFunc(agentID)
}

var ErrMultiAgentClosed = xerrors.New("multiagent is closed")

func (m *MultiAgent) UpdateSelf(node *Node) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return ErrMultiAgentClosed
	}

	return m.OnNodeUpdate(m.ID, node)
}

func (m *MultiAgent) SubscribeAgent(agentID uuid.UUID) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return ErrMultiAgentClosed
	}

	node, err := m.OnSubscribe(m, agentID)
	if err != nil {
		return err
	}

	if node != nil {
		return m.enqueueLocked([]*Node{node})
	}

	return nil
}

func (m *MultiAgent) UnsubscribeAgent(agentID uuid.UUID) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return ErrMultiAgentClosed
	}

	return m.OnUnsubscribe(m, agentID)
}

func (m *MultiAgent) NextUpdate(ctx context.Context) ([]*Node, bool) {
	select {
	case <-ctx.Done():
		return nil, false

	case nodes, ok := <-m.updates:
		return nodes, ok
	}
}

func (m *MultiAgent) Enqueue(nodes []*Node) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil
	}

	return m.enqueueLocked(nodes)
}

func (m *MultiAgent) enqueueLocked(nodes []*Node) error {
	atomic.StoreInt64(&m.lastWrite, time.Now().Unix())

	select {
	case m.updates <- nodes:
		return nil
	default:
		return ErrWouldBlock
	}
}

func (m *MultiAgent) Name() string {
	return m.ID.String()
}

func (m *MultiAgent) Stats() (start int64, lastWrite int64) {
	return m.start, atomic.LoadInt64(&m.lastWrite)
}

func (m *MultiAgent) Overwrites() int64 {
	return m.overwrites
}

func (m *MultiAgent) IsClosed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closed
}

func (m *MultiAgent) CoordinatorClose() error {
	m.mu.Lock()
	if !m.closed {
		m.closed = true
		close(m.updates)
	}
	m.mu.Unlock()
	return nil
}

func (m *MultiAgent) Done() <-chan struct{} {
	return m.ctx.Done()
}

func (m *MultiAgent) Close() error {
	_ = m.CoordinatorClose()
	m.ctxCancel()
	m.closeOnce.Do(func() { m.OnRemove(m) })
	return nil
}
