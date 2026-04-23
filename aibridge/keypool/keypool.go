package keypool

import (
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/quartz"
)

// KeyState represents the current state of a key in the pool.
type KeyState int

const (
	// KeyStateValid means the key is available for use.
	KeyStateValid KeyState = iota
	// KeyStateTemporary means the key is temporarily unavailable
	// (e.g. rate-limited) and will recover after a cooldown.
	KeyStateTemporary
	// KeyStatePermanent means the key is permanently unavailable
	// (e.g. revoked or unauthorized) until process restart.
	KeyStatePermanent
)

// DefaultCooldown is applied when MarkTemporary is called with a
// zero or negative cooldown duration.
const DefaultCooldown = 60 * time.Second

// keyEntry holds a key and its current state.
type keyEntry struct {
	key      string
	state    KeyState
	cooldown time.Time // Only meaningful when state == KeyStateTemporary.
}

// Pool manages a set of keys with state tracking and
// automatic cooldown expiry. It is safe for concurrent use.
type Pool struct {
	mu      sync.RWMutex
	entries []keyEntry
	clock   quartz.Clock
}

// New creates a pool from the given keys. All keys start in the
// valid state. Returns nil if keys is empty.
func New(keys []string, clk quartz.Clock) *Pool {
	if len(keys) == 0 {
		return nil
	}

	entries := make([]keyEntry, len(keys))
	for i, k := range keys {
		entries[i] = keyEntry{key: k, state: KeyStateValid}
	}

	return &Pool{
		entries: entries,
		clock:   clk,
	}
}

// Key is a handle to a specific key in the pool and provides
// methods to read its value and update its state.
type Key struct {
	pool  *Pool
	index int
}

// Value returns the key string.
func (k *Key) Value() string {
	return k.pool.entries[k.index].key
}

// State returns the current state of the key.
func (k *Key) State() KeyState {
	k.pool.mu.RLock()
	defer k.pool.mu.RUnlock()

	return k.pool.entries[k.index].state
}

// MarkValid transitions the key back to valid state. The call
// is a no-op if the key is permanent or if the key is temporary
// with an active cooldown.
func (k *Key) MarkValid() {
	k.pool.mu.Lock()
	defer k.pool.mu.Unlock()

	entry := &k.pool.entries[k.index]
	switch entry.state {
	case KeyStatePermanent:
		return
	case KeyStateTemporary:
		// Ignore stale successes from concurrent requests
		// that started before the key was rate-limited.
		if k.pool.clock.Now().Before(entry.cooldown) {
			return
		}
	}

	entry.state = KeyStateValid
}

// MarkTemporary marks the key as temporarily unavailable with
// the specified cooldown duration. If cooldown is zero or
// negative, DefaultCooldown is used. If the key is already in
// a permanent state, the call is a no-op.
func (k *Key) MarkTemporary(cooldown time.Duration) {
	k.pool.mu.Lock()
	defer k.pool.mu.Unlock()

	entry := &k.pool.entries[k.index]
	if entry.state == KeyStatePermanent {
		return
	}

	if cooldown <= 0 {
		cooldown = DefaultCooldown
	}

	newDeadline := k.pool.clock.Now().Add(cooldown)

	// Keep the longer cooldown when concurrent requests both
	// mark the same key as rate-limited.
	if entry.state == KeyStateTemporary && entry.cooldown.After(newDeadline) {
		return
	}

	entry.state = KeyStateTemporary
	entry.cooldown = newDeadline
}

// MarkPermanent marks the key as permanently unavailable. This
// is a terminal state.
func (k *Key) MarkPermanent() {
	k.pool.mu.Lock()
	defer k.pool.mu.Unlock()

	k.pool.entries[k.index].state = KeyStatePermanent
}

// Walker traverses a Pool for a single request. Each request
// creates its own walker so that it can independently iterate
// through keys without interfering with other requests.
type Walker struct {
	pool *Pool
	pos  int // Next index to consider.
}

// Walker creates a new Walker that follows a primary-with-fallback
// strategy, starting from the first key in the pool. The walker
// is not safe for concurrent use, it is intended for a single
// request's failover loop.
func (p *Pool) Walker() *Walker {
	return &Walker{pool: p, pos: 0}
}

// ErrAllKeysExhausted is returned when the walker has visited
// every key in the pool and none are available.
var ErrAllKeysExhausted = xerrors.New("all keys exhausted")

// Next returns a Key handle for the next available key. This is
// a read-only operation; it does not modify the pool state.
//
// Returns ErrAllKeysExhausted when no more keys are available.
func (w *Walker) Next() (*Key, error) {
	p := w.pool
	p.mu.RLock()
	defer p.mu.RUnlock()

	now := p.clock.Now()

	for i := w.pos; i < len(p.entries); i++ {
		entry := &p.entries[i]

		switch entry.state {
		case KeyStateValid:
			// Key is available, use it.
			w.pos = i + 1
			return &Key{pool: p, index: i}, nil

		case KeyStateTemporary:
			// Cooldown expired, treat as available.
			if now.After(entry.cooldown) {
				w.pos = i + 1
				return &Key{pool: p, index: i}, nil
			}
			// Still cooling down, skip.

		case KeyStatePermanent:
			// Permanently unavailable, skip.
		}
	}

	return nil, ErrAllKeysExhausted
}
