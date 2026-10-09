package chatd

import (
	"sync"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// agentConnCache keeps one workspace agent connection, and that workspace
// agent's home directory, for a chat runner between generation steps.
//
// Each step resolves its workspace agent from its own chat snapshot and
// borrows the connection only if the cache holds that workspace agent. The
// cache drops its connection when a step adopts another workspace agent or
// finds the cached one unusable, and releases it once the last borrower has
// returned it.
type agentConnCache struct {
	mu    sync.Mutex
	entry *agentConnEntry
}

type agentConnEntry struct {
	agentID   uuid.UUID
	conn      workspacesdk.AgentConn
	release   func()
	home      string // Empty until read from the workspace agent.
	borrowers int
	dropped   bool
}

// borrow returns the cached connection for agentID. ok is false if the cache
// holds no connection to agentID. The caller must call ret when done with
// conn.
func (c *agentConnCache) borrow(agentID uuid.UUID) (conn workspacesdk.AgentConn, ret func(), ok bool) {
	if c == nil {
		return nil, nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.entry
	if e == nil || e.agentID != agentID {
		return nil, nil, false
	}
	e.borrowers++
	return e.conn, c.returnFunc(e), true
}

// adopt caches conn, a new connection to agentID that release closes, and
// borrows it. If the cache already holds a connection to agentID, adopt
// releases conn and borrows the cached one. A cached connection to another
// workspace agent is dropped.
func (c *agentConnCache) adopt(agentID uuid.UUID, conn workspacesdk.AgentConn, release func()) (_ workspacesdk.AgentConn, ret func()) {
	if c == nil {
		return conn, release
	}
	c.mu.Lock()
	if e := c.entry; e != nil && e.agentID == agentID {
		e.borrowers++
		ret = c.returnFunc(e)
		c.mu.Unlock()
		callRelease(release)
		return e.conn, ret
	}
	stale := c.dropLocked()
	e := &agentConnEntry{agentID: agentID, conn: conn, release: release, borrowers: 1}
	c.entry = e
	ret = c.returnFunc(e)
	c.mu.Unlock()
	callRelease(stale)
	return conn, ret
}

// drop removes the cached connection. It is released once its borrowers have
// returned it.
func (c *agentConnCache) drop() {
	if c == nil {
		return
	}
	c.mu.Lock()
	stale := c.dropLocked()
	c.mu.Unlock()
	callRelease(stale)
}

// dropLocked removes the cached entry and returns its release func if no
// borrower holds it. c.mu must be held.
func (c *agentConnCache) dropLocked() func() {
	e := c.entry
	if e == nil {
		return nil
	}
	c.entry = nil
	e.dropped = true
	if e.borrowers > 0 {
		return nil
	}
	return e.release
}

func (c *agentConnCache) returnFunc(e *agentConnEntry) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			e.borrowers--
			var release func()
			if e.dropped && e.borrowers == 0 {
				release = e.release
			}
			c.mu.Unlock()
			callRelease(release)
		})
	}
}

// home returns the home directory read from agentID, if the cache holds a
// connection to agentID and has read it.
func (c *agentConnCache) home(agentID uuid.UUID) (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entry == nil || c.entry.agentID != agentID || c.entry.home == "" {
		return "", false
	}
	return c.entry.home, true
}

// setHome records the home directory read from agentID. It does nothing if
// the cache no longer holds a connection to agentID.
func (c *agentConnCache) setHome(agentID uuid.UUID, home string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entry != nil && c.entry.agentID == agentID {
		c.entry.home = home
	}
}

func callRelease(release func()) {
	if release != nil {
		release()
	}
}
