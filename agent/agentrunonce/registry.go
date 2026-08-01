// Package agentrunonce provides keyed reservations that let one caller
// perform an operation while later callers presenting the same input
// attach to the value it published. Values are never interpreted, so
// any kind of operation can use the same rule.
package agentrunonce

import (
	"context"
	"errors"
	"sync"
	"time"

	"golang.org/x/xerrors"
)

var (
	// ErrInputMismatch reports a key reserved earlier with a
	// different input fingerprint. Retrying cannot resolve it.
	ErrInputMismatch = xerrors.New("key was already reserved with a different input")

	// ErrPublicationPending reports a reservation that was still
	// pending when the wait ended. The outcome is unresolved rather
	// than failed.
	ErrPublicationPending = xerrors.New("gave up waiting for the reservation to be published")

	// ErrClosed reports a new reservation attempt on a closed registry.
	ErrClosed = xerrors.New("registry is closed")

	// ErrNotReserved reports a key that holds no reservation, because
	// it was never reserved or its reservation was released or reaped.
	ErrNotReserved = xerrors.New("key is not reserved")
)

// entry is one reservation. completedAt starts the retention window,
// so a zero completedAt keeps the entry indefinitely.
type entry[V any] struct {
	fingerprint string
	value       V
	published   bool
	completedAt time.Time
	done        chan struct{}
	// generation numbers successive reservations of one key. Forget
	// uses it to avoid deleting a replacement reservation.
	generation uint64
}

// Registry tracks reservations for one kind of operation.
type Registry[K comparable, V any] struct {
	mu         sync.Mutex
	retention  time.Duration
	entries    map[K]*entry[V]
	generation uint64
	closed     bool
	// closeCh releases waiting callers on shutdown instead of
	// stranding them until their own contexts expire.
	closeCh chan struct{}
}

func NewRegistry[K comparable, V any](retention time.Duration) *Registry[K, V] {
	return &Registry[K, V]{
		retention: retention,
		entries:   make(map[K]*entry[V]),
		closeCh:   make(chan struct{}),
	}
}

// Outcome reports how a reservation attempt resolved. Either Ticket is
// set, meaning this caller must perform the operation, or Value came
// from a caller that already did.
type Outcome[K comparable, V any] struct {
	Ticket *Ticket[K, V]
	Value  V
	// Generation identifies the reservation Value came from. Pass it
	// back to Forget so a stale value cannot evict a newer one.
	Generation uint64
}

// Ticket is the handle to a pending reservation. Its holder must call
// Publish or Release; whichever lands first decides the outcome.
type Ticket[K comparable, V any] struct {
	registry *Registry[K, V]
	key      K
	entry    *entry[V]
}

// Reserve resolves a key to either a ticket for a new reservation or
// the value already published for the same fingerprint. The context
// only limits how long a caller waits for a pending reservation. It
// does not bound the operation itself.
func (r *Registry[K, V]) Reserve(ctx context.Context, key K, fingerprint string) (Outcome[K, V], error) {
	var zero Outcome[K, V]
	for {
		outcome, pending, err := r.tryReserve(key, fingerprint)
		if err != nil {
			return zero, err
		}
		if pending == nil {
			return outcome, nil
		}
		if err := r.waitForPublication(ctx, pending); err != nil {
			return zero, err
		}
	}
}

// tryReserve takes a free key, and otherwise reports either the
// resolved outcome or the pending entry to wait on.
func (r *Registry[K, V]) tryReserve(key K, fingerprint string) (Outcome[K, V], *entry[V], error) {
	var zero Outcome[K, V]

	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.entries[key]
	if !ok {
		if r.closed {
			return zero, nil, ErrClosed
		}
		r.generation++
		reserved := &entry[V]{
			fingerprint: fingerprint,
			done:        make(chan struct{}),
			generation:  r.generation,
		}
		r.entries[key] = reserved
		return Outcome[K, V]{
			Ticket:     &Ticket[K, V]{registry: r, key: key, entry: reserved},
			Generation: reserved.generation,
		}, nil, nil
	}
	if existing.fingerprint != fingerprint {
		return zero, nil, ErrInputMismatch
	}
	if existing.published {
		return Outcome[K, V]{Value: existing.value, Generation: existing.generation}, nil, nil
	}
	return zero, existing, nil
}

// waitForPublication blocks until the reservation is published or
// released. It must be called without holding r.mu.
func (r *Registry[K, V]) waitForPublication(ctx context.Context, pending *entry[V]) error {
	select {
	case <-pending.done:
		return nil
	case <-r.closeCh:
		return ErrPublicationPending
	case <-ctx.Done():
		return xerrors.Errorf("wait for reservation to be published: %w", errors.Join(ErrPublicationPending, ctx.Err()))
	}
}

// Await returns the value published for a key, so callers can act on
// an operation by its identity rather than by whatever handle it
// returned. A pending reservation is waited out until ctx ends, so a
// caller cannot mistake work that has not published yet for work that
// never happened. It reports ErrNotReserved for an unreserved key and
// ErrPublicationPending for one still pending at the deadline.
func (r *Registry[K, V]) Await(ctx context.Context, key K) (V, error) {
	var zero V
	for {
		r.mu.Lock()
		existing, ok := r.entries[key]
		if !ok {
			r.mu.Unlock()
			return zero, ErrNotReserved
		}
		if existing.published {
			value := existing.value
			r.mu.Unlock()
			return value, nil
		}
		r.mu.Unlock()

		if err := r.waitForPublication(ctx, existing); err != nil {
			return zero, err
		}
	}
}

func (r *Registry[K, V]) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.entries)
}

// Complete starts a published reservation's retention window.
func (r *Registry[K, V]) Complete(key K, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.entries[key]; ok && existing.published {
		existing.completedAt = now
	}
}

// Forget drops the published reservation identified by generation,
// whose value the caller found unusable, so the next reservation for
// that key starts fresh. Deleting by key alone could evict a newer
// reservation that replaced it and let its operation run twice.
// Pending reservations are never dropped.
func (r *Registry[K, V]) Forget(key K, generation uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.entries[key]; ok && existing.published && existing.generation == generation {
		delete(r.entries, key)
	}
}

// Reap removes reservations whose retention window has elapsed and
// returns their values so the caller can release what they refer to.
func (r *Registry[K, V]) Reap(now time.Time) []V {
	r.mu.Lock()
	defer r.mu.Unlock()

	var evicted []V
	for key, existing := range r.entries {
		if existing.completedAt.IsZero() || now.Sub(existing.completedAt) <= r.retention {
			continue
		}
		delete(r.entries, key)
		evicted = append(evicted, existing.value)
	}
	return evicted
}

// Close releases waiting callers and refuses new reservations. Already
// published reservations keep serving their value.
func (r *Registry[K, V]) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return
	}
	r.closed = true
	close(r.closeCh)
}

// Publish records the value later callers attach to and wakes waiters.
func (t *Ticket[K, V]) Publish(value V) {
	t.registry.mu.Lock()
	defer t.registry.mu.Unlock()

	if t.entry.published {
		return
	}
	t.entry.value = value
	t.entry.published = true
	close(t.entry.done)
}

// Release abandons an unpublished reservation so another caller can
// take over the key.
func (t *Ticket[K, V]) Release() {
	t.registry.mu.Lock()
	defer t.registry.mu.Unlock()

	if t.entry.published {
		return
	}
	if t.registry.entries[t.key] == t.entry {
		delete(t.registry.entries, t.key)
	}
	close(t.entry.done)
}
