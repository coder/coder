package agentapi

import (
	"sync"

	"github.com/google/uuid"
)

// CachedAgentFields contains agent data that is safe to cache for the
// duration of an agent connection. These fields are used to reduce database calls
// in high-frequency operations like metadata updates, stats reporting, and connection logging.
//
// IMPORTANT: Only static fields that never change during an agent's lifetime should be cached here.
// Dynamic fields (like StartedAt, ReadyAt, LogsOverflowed) should NOT be cached as they can be
// modified by API calls or external processes.
type CachedAgentFields struct {
	lock sync.RWMutex

	// Static fields that never change during agent connection
	id   uuid.UUID
	name string
}

// UpdateValues sets the cached agent fields. This should only be called once
// at agent connection initialization.
func (caf *CachedAgentFields) UpdateValues(id uuid.UUID, name string) {
	caf.lock.Lock()
	defer caf.lock.Unlock()
	caf.id = id
	caf.name = name
}

// ID returns the cached agent ID.
func (caf *CachedAgentFields) ID() uuid.UUID {
	caf.lock.RLock()
	defer caf.lock.RUnlock()
	return caf.id
}

// Name returns the cached agent name.
func (caf *CachedAgentFields) Name() string {
	caf.lock.RLock()
	defer caf.lock.RUnlock()
	return caf.name
}

// IsPopulated returns true if the cache has been initialized with values.
func (caf *CachedAgentFields) IsPopulated() bool {
	caf.lock.RLock()
	defer caf.lock.RUnlock()
	return caf.id != uuid.Nil
}
