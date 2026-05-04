package keypool

import (
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/quartz"
)

var (
	// ErrNoKeys is returned when the input is empty.
	ErrNoKeys = xerrors.New("no keys provided")
	// ErrDuplicateKey is returned when the input contains
	// duplicate key values.
	ErrDuplicateKey = xerrors.New("duplicate key")
	// ErrAllKeysExhausted is returned when the walker has visited
	// every key in the pool and none are available.
	ErrAllKeysExhausted = xerrors.New("all keys exhausted")
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

// defaultCooldown is applied when a key is marked temporary
// with a zero or negative cooldown duration.
const defaultCooldown = 60 * time.Second

// Key holds a key value and its runtime state.
type Key struct {
	value         string
	permanent     bool
	cooldownUntil time.Time

	mu    sync.RWMutex
	clock quartz.Clock
}

// Pool manages a set of keys with state tracking and
// cooldown expiry. It is safe for concurrent use.
type Pool struct {
	keys []Key
}

// New creates a pool from the given keys. All keys start in
// the valid state. Returns ErrNoKeys if keys is empty and
// ErrDuplicateKey if any key appears more than once.
func New(keys []string, clk quartz.Clock) (*Pool, error) {
	if len(keys) == 0 {
		return nil, ErrNoKeys
	}
	pool := &Pool{
		keys: make([]Key, len(keys)),
	}

	seen := make(map[string]struct{}, len(keys))
	for i, val := range keys {
		if _, exists := seen[val]; exists {
			return nil, ErrDuplicateKey
		}
		seen[val] = struct{}{}
		pool.keys[i] = Key{
			clock: clk,
			value: val,
		}
	}

	return pool, nil
}

// Value returns the key string.
func (k *Key) Value() string {
	return k.value
}

// State returns the current state of the key, derived from its
// permanent flag and cooldown deadline.
func (k *Key) State() KeyState {
	k.mu.RLock()
	defer k.mu.RUnlock()

	if k.permanent {
		return KeyStatePermanent
	}
	// Cooldown still active: key is temporarily unavailable.
	if k.clock.Now().Before(k.cooldownUntil) {
		return KeyStateTemporary
	}
	return KeyStateValid
}

// MarkTemporary marks the key as temporarily unavailable with
// the specified cooldown duration. Returns true if this call
// transitions the key to temporary.
func (k *Key) MarkTemporary(cooldown time.Duration) bool {
	k.mu.Lock()
	defer k.mu.Unlock()

	// Permanent is irreversible.
	if k.permanent {
		return false
	}

	if cooldown <= 0 {
		cooldown = defaultCooldown
	}

	now := k.clock.Now()
	// Used to detect the valid -> temporary transition.
	inCooldown := k.cooldownUntil.After(now)
	newDeadline := now.Add(cooldown)

	// In case the key has a later expiry, keep it.
	if k.cooldownUntil.After(newDeadline) {
		return false
	}

	k.cooldownUntil = newDeadline
	return !inCooldown
}

// MarkPermanent marks the key as permanently unavailable. This
// is a terminal state. Returns true if this call transitions
// the key to permanent.
func (k *Key) MarkPermanent() bool {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.permanent {
		return false
	}

	k.permanent = true
	return true
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
// is not safe for concurrent use. It is intended for a single
// request's failover loop.
func (p *Pool) Walker() *Walker {
	return &Walker{pool: p, pos: 0}
}

// Next returns a Key handle for the next available key. This is
// a read-only operation; it does not modify the pool state.
//
// Returns ErrAllKeysExhausted when no more keys are available.
func (w *Walker) Next() (*Key, error) {
	pool := w.pool
	if pool == nil {
		return nil, ErrAllKeysExhausted
	}

	for i := w.pos; i < len(pool.keys); i++ {
		key := &pool.keys[i]
		if key.State() != KeyStateValid {
			continue
		}
		// Key is available.
		w.pos = i + 1
		return key, nil
	}

	// No keys available.
	return nil, ErrAllKeysExhausted
}
