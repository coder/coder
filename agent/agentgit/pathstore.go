package agentgit

import (
	"slices"
	"sync"

	"github.com/google/uuid"
)

// maxPathsPerChat bounds the paths retained per chat. Every
// notification re-reads a chat's paths, and a long chat that reads
// many files would otherwise grow agent memory and that work without
// limit. Read-only paths are evicted before written ones because
// writes and process workdirs are what produce diffs.
const maxPathsPerChat = 1000

// pathEntry records how a chat touched a path. seq orders entries by
// their most recent use for eviction.
type pathEntry struct {
	seq     uint64
	written bool
}

// PathStore tracks which file paths each chat has touched.
// It is safe for concurrent use.
type PathStore struct {
	mu          sync.RWMutex
	seq         uint64
	chatPaths   map[uuid.UUID]map[string]pathEntry
	subscribers map[uuid.UUID][]chan<- struct{}
}

// NewPathStore creates a new PathStore.
func NewPathStore() *PathStore {
	return &PathStore{
		chatPaths:   make(map[uuid.UUID]map[string]pathEntry),
		subscribers: make(map[uuid.UUID][]chan<- struct{}),
	}
}

// AddPaths records paths a chat wrote or ran a process in, for every
// chat in chatIDs, and notifies their subscribers. It always notifies,
// even for known paths, because a write changes the repository.
// Zero-value UUIDs are silently skipped.
func (ps *PathStore) AddPaths(chatIDs []uuid.UUID, paths []string) {
	affected := nonNilIDs(chatIDs)
	if len(affected) == 0 {
		return
	}

	ps.mu.Lock()
	for _, id := range affected {
		ps.addLocked(id, paths, true)
	}
	ps.mu.Unlock()

	ps.notifySubscribers(affected)
}

// AddReadPaths records paths a chat only read, for every chat in
// chatIDs. A read does not change the repository, so subscribers are
// notified only for chats that gain a new path. Zero-value UUIDs are
// silently skipped.
func (ps *PathStore) AddReadPaths(chatIDs []uuid.UUID, paths []string) {
	ids := nonNilIDs(chatIDs)
	if len(ids) == 0 {
		return
	}

	affected := make([]uuid.UUID, 0, len(ids))
	ps.mu.Lock()
	for _, id := range ids {
		if ps.addLocked(id, paths, false) {
			affected = append(affected, id)
		}
	}
	ps.mu.Unlock()

	if len(affected) > 0 {
		ps.notifySubscribers(affected)
	}
}

// addLocked records paths for chatID and reports whether any path
// was new. A written path stays written when it is read again.
func (ps *PathStore) addLocked(chatID uuid.UUID, paths []string, written bool) bool {
	m, ok := ps.chatPaths[chatID]
	if !ok {
		m = make(map[string]pathEntry)
		ps.chatPaths[chatID] = m
	}
	added := false
	for _, p := range paths {
		ps.seq++
		entry, exists := m[p]
		if !exists {
			added = true
		}
		m[p] = pathEntry{seq: ps.seq, written: entry.written || written}
	}
	if len(m) > maxPathsPerChat {
		evictOldest(m, len(m)-maxPathsPerChat)
	}
	return added
}

// evictOldest removes n entries from m: least recently used
// read-only paths first, then least recently used written paths.
func evictOldest(m map[string]pathEntry, n int) {
	type keyed struct {
		path  string
		entry pathEntry
	}
	all := make([]keyed, 0, len(m))
	for p, e := range m {
		all = append(all, keyed{path: p, entry: e})
	}
	slices.SortFunc(all, func(a, b keyed) int {
		if a.entry.written != b.entry.written {
			if a.entry.written {
				return 1
			}
			return -1
		}
		if a.entry.seq < b.entry.seq {
			return -1
		}
		return 1
	})
	for _, k := range all[:n] {
		delete(m, k.path)
	}
}

func nonNilIDs(chatIDs []uuid.UUID) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(chatIDs))
	for _, id := range chatIDs {
		if id != uuid.Nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// Notify sends a signal to all subscribers of the given chat IDs
// without adding any paths. Zero-value UUIDs are silently skipped.
func (ps *PathStore) Notify(chatIDs []uuid.UUID) {
	affected := nonNilIDs(chatIDs)
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
	slices.Sort(out)
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
