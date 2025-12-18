package agentapi

import (
	"github.com/google/uuid"
)

// CachedAgentFields contains agent data that is safe to cache for the
// duration of an agent connection. These fields are used to reduce database calls
// in high-frequency operations like metadata updates, stats reporting, and connection logging.
//
// IMPORTANT: Only static fields that never change during an agent's lifetime should be cached here.
// Dynamic fields (like StartedAt, ReadyAt, LogsOverflowed) should NOT be cached as they can be
// modified by API calls or external processes.
//
// Unlike CachedWorkspaceFields, this struct does not need a mutex because the values are set once
// at initialization and never modified after that.
type CachedAgentFields struct {
	// Static fields that never change during agent connection
	id   uuid.UUID
	name string
}

// UpdateValues sets the cached agent fields. This should only be called once
// at agent connection initialization.
func (caf *CachedAgentFields) UpdateValues(id uuid.UUID, name string) {
	caf.id = id
	caf.name = name
}

// ID returns the agent ID.
func (caf *CachedAgentFields) ID() uuid.UUID {
	return caf.id
}

// Name returns the agent name.
func (caf *CachedAgentFields) Name() string {
	return caf.name
}
