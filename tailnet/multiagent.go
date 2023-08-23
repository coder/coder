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
	NextUpdate(ctx context.Context) (CoordinatorReply, bool)
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
	OnRemove          func(id uuid.UUID)

	closed    bool
	replies   chan CoordinatorReply
	closeOnce sync.Once
	start     int64
	lastWrite int64
	// Client nodes normally generate a unique id for each connection so
	// overwrites are really not an issue, but is provided for compatibility.
	overwrites int64
}

func (m *MultiAgent) Init() *MultiAgent {
	m.replies = make(chan CoordinatorReply, 128)
	m.start = time.Now().Unix()
	return m
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
		return m.enqueueLocked(CoordinatorReply{
			AddNodes: []*Node{node},
		})
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

func (m *MultiAgent) NextUpdate(ctx context.Context) (CoordinatorReply, bool) {
	select {
	case <-ctx.Done():
		return CoordinatorReply{}, false

	case reply, ok := <-m.replies:
		return reply, ok
	}
}

func (m *MultiAgent) Enqueue(reply CoordinatorReply) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil
	}

	return m.enqueueLocked(reply)
}

func (m *MultiAgent) enqueueLocked(reply CoordinatorReply) error {
	atomic.StoreInt64(&m.lastWrite, time.Now().Unix())

	select {
	case m.replies <- reply:
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
		close(m.replies)
	}
	m.mu.Unlock()
	return nil
}

func (m *MultiAgent) Close() error {
	_ = m.CoordinatorClose()
	m.closeOnce.Do(func() { m.OnRemove(m.ID) })
	return nil
}
