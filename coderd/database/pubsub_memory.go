package database

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// genericListener is either a Listener or ListenerWithErr
type genericListener struct {
	l  Listener
	le ListenerWithErr
}

func (g genericListener) send(ctx context.Context, message []byte) {
	if g.l != nil {
		g.l(ctx, message)
	}
	if g.le != nil {
		g.le(ctx, message, nil)
	}
}

// memoryPubsub is an in-memory Pubsub implementation.
type memoryPubsub struct {
	mut       sync.RWMutex
	listeners map[string]map[uuid.UUID]genericListener
}

func (m *memoryPubsub) Subscribe(event string, listener Listener) (cancel func(), err error) {
	return m.subscribeGeneric(event, genericListener{l: listener})
}

func (m *memoryPubsub) SubscribeWithErr(event string, listener ListenerWithErr) (cancel func(), err error) {
	return m.subscribeGeneric(event, genericListener{le: listener})
}

func (m *memoryPubsub) subscribeGeneric(event string, listener genericListener) (cancel func(), err error) {
	m.mut.Lock()
	defer m.mut.Unlock()

	var listeners map[uuid.UUID]genericListener
	var ok bool
	if listeners, ok = m.listeners[event]; !ok {
		listeners = map[uuid.UUID]genericListener{}
		m.listeners[event] = listeners
	}
	var id uuid.UUID
	for {
		id = uuid.New()
		if _, ok = listeners[id]; !ok {
			break
		}
	}
	listeners[id] = listener
	return func() {
		m.mut.Lock()
		defer m.mut.Unlock()
		listeners := m.listeners[event]
		delete(listeners, id)
	}, nil
}

func (m *memoryPubsub) Publish(event string, message []byte) error {
	m.mut.RLock()
	defer m.mut.RUnlock()
	listeners, ok := m.listeners[event]
	if !ok {
		return nil
	}
	var wg sync.WaitGroup
	for _, listener := range listeners {
		wg.Add(1)
		listener := listener
		go func() {
			defer wg.Done()
			listener.send(context.Background(), message)
		}()
	}
	wg.Wait()

	return nil
}

func (*memoryPubsub) Close() error {
	return nil
}

func NewPubsubInMemory() Pubsub {
	return &memoryPubsub{
		listeners: make(map[string]map[uuid.UUID]genericListener),
	}
}
