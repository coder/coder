package agentgit

import (
	"sort"
	"sync"

	"github.com/google/uuid"
)

// PathStore tracks which file paths each chat has touched.
// It is safe for concurrent use.
type PathStore struct {
	mu          sync.RWMutex
	chatPaths   map[uuid.UUID]map[string]struct{}
	subscribers map[uuid.UUID][]chan<- struct{}
}

// NewPathStore creates a new PathStore.
func NewPathStore() *PathStore {
	return &PathStore{
		chatPaths:   make(map[uuid.UUID]map[string]struct{}),
		subscribers: make(map[uuid.UUID][]chan<- struct{}),
	}
}

// AddPaths adds paths to every chat in chatIDs and notifies
// their subscribers. Zero-value UUIDs are silently skipped.
func (ps *PathStore) AddPaths(chatIDs []uuid.UUID, paths []string) {
	affected := make([]uuid.UUID, 0, len(chatIDs))
	for _, id := range chatIDs {
		if id != uuid.Nil {
			affected = append(affected, id)
		}
	}
	if len(affected) == 0 {
		return
	}

	ps.mu.Lock()
	for _, id := range affected {
		m, ok := ps.chatPaths[id]
		if !ok {
			m = make(map[string]struct{})
			ps.chatPaths[id] = m
		}
		for _, p := range paths {
			m[p] = struct{}{}
		}
	}
	ps.mu.Unlock()

	ps.notifySubscribers(affected)
}

// Notify sends a signal to all subscribers of the given chat IDs
// without adding any paths. Zero-value UUIDs are silently skipped.
func (ps *PathStore) Notify(chatIDs []uuid.UUID) {
	affected := make([]uuid.UUID, 0, len(chatIDs))
	for _, id := range chatIDs {
		if id != uuid.Nil {
			affected = append(affected, id)
		}
	}
	if len(affected) == 0 {
		return
	}
	ps.notifySubscribers(affected)
}

// notifySubscribers sends a non-blocking signal to all subscriber
// channels for the given chat IDs.
func (ps *PathStore) notifySubscribers(chatIDs []uuid.UUID) {
	ps.mu.RLock()
	toNotify := make([]chan<- struct{}, 0)
	for _, id := range chatIDs {
		toNotify = append(toNotify, ps.subscribers[id]...)
	}
	ps.mu.RUnlock()

	for _, ch := range toNotify {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// GetPaths returns all paths tracked for a chat, deduplicated
// and sorted lexicographically.
func (ps *PathStore) GetPaths(chatID uuid.UUID) []string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	m := ps.chatPaths[chatID]
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for p := range m {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Len returns the number of chat IDs that have tracked paths.
func (ps *PathStore) Len() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.chatPaths)
}

// Subscribe returns a channel that receives a signal whenever
// paths change for chatID, along with an unsubscribe function
// that removes the channel.
func (ps *PathStore) Subscribe(chatID uuid.UUID) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)

	ps.mu.Lock()
	ps.subscribers[chatID] = append(ps.subscribers[chatID], ch)
	ps.mu.Unlock()

	unsub := func() {
		ps.mu.Lock()
		defer ps.mu.Unlock()
		subs := ps.subscribers[chatID]
		for i, s := range subs {
			if s == ch {
				ps.subscribers[chatID] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
	}

	return ch, unsub
}
