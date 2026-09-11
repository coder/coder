package keypool

import (
	"fmt"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/utils"
	"github.com/coder/quartz"
)

// Configuration validation type errors. These surface when the
// pool is built from invalid input.
var (
	// ErrNoKeys is returned when the input is empty.
	ErrNoKeys = xerrors.New("no keys provided")
	// ErrDuplicateKey is returned when the input contains
	// duplicate key values.
	ErrDuplicateKey = xerrors.New("duplicate key")
)

// ErrorKind classifies a runtime key-pool failure.
type ErrorKind int

const (
	// ErrorKindRateLimited means no key is currently available
	// but at least one key will recover after a cooldown.
	ErrorKindRateLimited ErrorKind = iota
	// ErrorKindUnauthorized means every unavailable key is in a
	// cooldown triggered by an authentication failure.
	ErrorKindUnauthorized
)

// Error is returned when no key is available for the
// current attempt. RetryAfter is the soonest remaining
// cooldown across the pool.
type Error struct {
	Kind       ErrorKind
	RetryAfter time.Duration
}

func (e *Error) Error() string {
	switch e.Kind {
	case ErrorKindUnauthorized:
		return "all configured keys failed authentication. Contact your Administrator"
	case ErrorKindRateLimited:
		return fmt.Sprintf("all configured keys are rate-limited (retry after %s)", e.RetryAfter)
	default:
		return "key pool error"
	}
}

// KeyState represents the current state of a key in the pool.
type KeyState string

const (
	// KeyStateValid means the key is available for use.
	KeyStateValid KeyState = "valid"
	// KeyStateTemporary means the key is temporarily unavailable
	// (e.g. rate-limited) and will recover after a cooldown.
	KeyStateTemporary KeyState = "temporary"
)

// defaultCooldown is applied when a key is marked temporary
// with a zero or negative cooldown duration.
const defaultCooldown = 60 * time.Second

// cooldownReason records why a key entered its current cooldown. It is
// meaningful only while the cooldown is active.
type cooldownReason string

const (
	// cooldownRateLimited means the current cooldown was triggered by a
	// rate-limit response (HTTP 429).
	cooldownRateLimited cooldownReason = "rate_limited"
	// cooldownUnauthorized means the current cooldown was triggered by an
	// authentication failure (HTTP 401).
	cooldownUnauthorized cooldownReason = "unauthorized"
)

const (
	outcomeRateLimited = "rate_limited"
	outcomeAuthFailed  = "auth_failed"
)

// Key holds a key value and its runtime state.
type Key struct {
	value         string
	cooldownUntil time.Time
	// reason records why the current cooldown was applied. It is only
	// meaningful while cooldownUntil is active.
	reason cooldownReason

	mu    sync.RWMutex
	clock quartz.Clock
}

// Pool manages a set of keys with state tracking and
// cooldown expiry. It is safe for concurrent use.
type Pool struct {
	keys         []Key
	metrics      *metrics.Metrics
	providerName string
}

// RecordAttempts records the total number of keys tried across an
// interception. Each upstream request uses its own walker, so the
// total sums the attempts across those per-request walkers. Call it
// once when the interception finishes.
func (p *Pool) RecordAttempts(attempts int) {
	if p == nil || p.metrics == nil || attempts == 0 {
		return
	}
	p.metrics.KeyPoolFailoverAttempts.WithLabelValues(p.providerName).Observe(float64(attempts))
}

// New creates a pool from the given keys, labeled by providerName in its
// metrics and logs. All keys start in the valid state. Returns ErrNoKeys
// if keys is empty and ErrDuplicateKey if any key appears more than once.
func New(providerName string, keys []string, clk quartz.Clock, m *metrics.Metrics) (*Pool, error) {
	if len(keys) == 0 {
		return nil, ErrNoKeys
	}
	pool := &Pool{
		keys:         make([]Key, len(keys)),
		metrics:      m,
		providerName: providerName,
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

// Hint returns a masked, identifiable fragment of the key, suitable
// for logs and persisted records.
func (k *Key) Hint() string {
	return utils.MaskSecret(k.value)
}

// Length returns the length of the key value, for logs.
func (k *Key) Length() int {
	return len(k.value)
}

// State returns the current state of the key, derived from its cooldown
// deadline.
func (k *Key) State() KeyState {
	k.mu.RLock()
	defer k.mu.RUnlock()

	// Cooldown still active: key is temporarily unavailable.
	if k.clock.Now().Before(k.cooldownUntil) {
		return KeyStateTemporary
	}
	return KeyStateValid
}

// stateAndCooldown returns the key's state, remaining cooldown, and the
// reason for the current cooldown as a single atomic snapshot.
func (k *Key) stateAndCooldown() (KeyState, time.Duration, cooldownReason) {
	k.mu.RLock()
	defer k.mu.RUnlock()

	now := k.clock.Now()
	if now.Before(k.cooldownUntil) {
		return KeyStateTemporary, k.cooldownUntil.Sub(now), k.reason
	}
	return KeyStateValid, 0, k.reason
}

// markTemporary marks the key unavailable for the given cooldown. Returns
// true on the valid -> temporary transition.
func (k *Key) markTemporary(cooldown time.Duration) bool {
	return k.applyCooldown(cooldown, cooldownRateLimited)
}

// applyCooldown marks the key unavailable for the given cooldown, recording
// reason as the cause. Returns true on the valid -> temporary transition.
func (k *Key) applyCooldown(cooldown time.Duration, reason cooldownReason) bool {
	k.mu.Lock()
	defer k.mu.Unlock()

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
	k.reason = reason
	return !inCooldown
}

// keyPoolError returns an Error summarizing why no key is currently
// available. When at least one key is temporary, the smallest remaining
// cooldown is used as the retry-after. A rate limit anywhere in the pool
// takes precedence, so the exhaustion is classified as unauthorized only
// when every cooldown was triggered by an auth failure.
func (p *Pool) keyPoolError() *Error {
	var retryAfter time.Duration
	var isRateLimited bool
	for i := range p.keys {
		state, cooldown, reason := p.keys[i].stateAndCooldown()
		switch state {
		// Recoverable now: a key's cooldown expired between the walker's
		// check and this scan. Return Retry-After: 0 to indicate that
		// an immediate retry will succeed.
		case KeyStateValid:
			return &Error{Kind: ErrorKindRateLimited}
		// Recoverable later: track soonest remaining cooldown and reason.
		case KeyStateTemporary:
			if retryAfter == 0 || cooldown < retryAfter {
				retryAfter = cooldown
			}
			if reason == cooldownRateLimited {
				isRateLimited = true
			}
		}
	}
	kind := ErrorKindUnauthorized
	if isRateLimited {
		kind = ErrorKindRateLimited
	}
	return &Error{Kind: kind, RetryAfter: retryAfter}
}

// recordExhaustion increments the exhaustion counter, labeling the outcome
// as an auth failure for unauthorized errors, else a rate limit.
func (p *Pool) recordExhaustion(err *Error) {
	if p.metrics == nil {
		return
	}
	outcome := outcomeRateLimited
	if err.Kind == ErrorKindUnauthorized {
		outcome = outcomeAuthFailed
	}
	p.metrics.KeyPoolExhaustions.WithLabelValues(p.providerName, outcome).Inc()
}

// PoolState returns a snapshot of each key's state in the pool's
// original order, used by tests and other diagnostic callers. Use
// Walker for the failover iteration path.
func (p *Pool) PoolState() []KeyState {
	states := make([]KeyState, len(p.keys))
	for i := range p.keys {
		states[i] = p.keys[i].State()
	}
	return states
}

// Walker traverses a Pool for a single request. Each request
// creates its own walker so that it can independently iterate
// through keys without interfering with other requests.
type Walker struct {
	pool     *Pool
	pos      int // Next index to consider.
	attempts int // Number of attempts, one per upstream HTTP request.
}

// Walker creates a new Walker that follows a primary-with-fallback
// strategy, starting from the first key in the pool. The walker
// is not safe for concurrent use. It is intended for a single
// request's failover loop.
func (p *Pool) Walker() *Walker {
	return &Walker{pool: p, pos: 0}
}

// Next returns a Key handle for the next available key without
// modifying the pool state.
//
// Returns *Error when no more keys are available.
func (w *Walker) Next() (*Key, *Error) {
	for i := w.pos; i < len(w.pool.keys); i++ {
		key := &w.pool.keys[i]
		if key.State() != KeyStateValid {
			continue
		}
		// Key is available.
		w.pos = i + 1
		w.attempts++
		return key, nil
	}

	// No keys available.
	err := w.pool.keyPoolError()
	w.pool.recordExhaustion(err)
	return nil, err
}

// Attempts returns the number of keys this walker handed out.
func (w *Walker) Attempts() int {
	if w == nil {
		return 0
	}
	return w.attempts
}
