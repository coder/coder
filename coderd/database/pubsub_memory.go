package database

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// memoryPubsub is an in-memory Pubsub implementation.
type memoryPubsub struct {
	mut       sync.RWMutex
	listeners map[string]map[uuid.UUID]Listener
}

func (m *memoryPubsub) Subscribe(event string, listener Listener) (cancel func(), err error) {
	m.mut.Lock()
	defer m.mut.Unlock()

	var listeners map[uuid.UUID]Listener
	var ok bool
	if listeners, ok = m.listeners[event]; !ok {
		listeners = map[uuid.UUID]Listener{}
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
	for _, listener := range listeners {
		go listener(context.Background(), message)
	}

	return nil
}

func (*memoryPubsub) Close() error {
	return nil
}

func NewPubsubInMemory() Pubsub {
	return &memoryPubsub{
		listeners: make(map[string]map[uuid.UUID]Listener),
	}
}
