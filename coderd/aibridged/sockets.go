package aibridged

import (
	"context"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/quartz"
)

// Responses WebSocket limits. No deployment option sets them yet: they are
// constants until operators need to tune them.
const (
	// maxSocketLifetime is OpenAI's own limit for one Responses WebSocket
	// connection, so clients already reconnect at this point.
	maxSocketLifetime = 60 * time.Minute
	// maxSocketsPerActor bounds the sockets one user holds on a replica.
	// Coding agents open one socket per session, so this leaves room for many
	// concurrent sessions while one user cannot take the replica's capacity.
	maxSocketsPerActor = 16
	// maxSocketsPerReplica bounds the sockets a replica holds. With the
	// per-socket memory bound documented in package responsesws, it bounds
	// the replica's socket buffer memory.
	maxSocketsPerReplica = 512
)

var (
	// ErrSocketActorLimit refuses a socket because its user holds
	// maxSocketsPerActor sockets on this replica. The caller refuses the
	// upgrade with 429.
	ErrSocketActorLimit = xerrors.New("too many open Responses WebSocket connections for this user; close one and retry")
	// ErrSocketReplicaLimit refuses a socket because this replica holds
	// maxSocketsPerReplica sockets. The caller refuses the upgrade with 429.
	ErrSocketReplicaLimit = xerrors.New("this AI Gateway replica is at its Responses WebSocket connection capacity; retry shortly")
	// ErrSocketLifetime is the cause a lease context ends with once the
	// socket reached maxSocketLifetime. Like ErrShutdown, the cause a
	// shutdown ends leases with, it means the socket closes as going away.
	ErrSocketLifetime = xerrors.New("Responses WebSocket connection reached its maximum lifetime")
)

// socketLimits configures a SocketRegistry.
type socketLimits struct {
	perActor   int
	perReplica int
	lifetime   time.Duration
}

var defaultSocketLimits = socketLimits{
	perActor:   maxSocketsPerActor,
	perReplica: maxSocketsPerReplica,
	lifetime:   maxSocketLifetime,
}

// SocketRegistry owns the Responses WebSockets of a Server. A socket holds a
// lease from before its upgrade, so sockets still connecting count against
// the per-actor and per-replica caps, until the socket ends.
//
// A lease context is detached from the request that acquired it: it keeps
// the request's values, such as the actor, but not its cancellation. A
// socket therefore survives the eviction of the per-user RequestBridge that
// served its upgrade, which cancels that bridge's request contexts. Bridge
// shutdown destroys nothing else a socket uses: the recorder, providers and
// their key pools, and the logger stay usable, and Responses WebSockets do
// not use the bridge's MCP proxies.
//
// Revocation: a socket is authorized once, at upgrade, and is not
// re-authorized afterwards, so a revoked API key keeps an open socket until
// its lease ends. The maximum lifetime bounds this, and every
// response.create of the socket is still rate limited and budget checked.
//
// Acquire, Release and Shutdown are atomic with respect to each other: no
// lease is granted once Shutdown began, and every granted lease is canceled
// by it.
type SocketRegistry struct {
	clock   quartz.Clock
	metrics *aibridge.Metrics
	limits  socketLimits

	mu       sync.Mutex
	closed   bool
	leases   map[*SocketLease]struct{}
	perActor map[string]int
	// drained is closed once the registry is closed and holds no lease.
	drained chan struct{}
}

func newSocketRegistry(clock quartz.Clock, metrics *aibridge.Metrics, limits socketLimits) *SocketRegistry {
	return &SocketRegistry{
		clock:    clock,
		metrics:  metrics,
		limits:   limits,
		leases:   make(map[*SocketLease]struct{}),
		perActor: make(map[string]int),
		drained:  make(chan struct{}),
	}
}

// SocketLease is one socket's hold on the registry. It is safe for
// concurrent use.
type SocketLease struct {
	registry *SocketRegistry
	actorID  string
	provider string
	ctx      context.Context
	cancel   context.CancelCauseFunc
	timer    *quartz.Timer
	once     sync.Once
}

// Acquire grants a lease for a socket of actorID to provider, or refuses it
// with ErrSocketActorLimit, ErrSocketReplicaLimit, or ErrShutdown once
// Shutdown began. ctx is the upgrade request's context. The caller must call
// Release once the socket ended, and should end the socket when the lease
// context ends.
func (r *SocketRegistry) Acquire(ctx context.Context, actorID, provider string) (*SocketLease, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var refusal error
	var reason string
	switch {
	case r.closed:
		refusal, reason = ErrShutdown, "shutdown"
	case r.perActor[actorID] >= r.limits.perActor:
		refusal, reason = ErrSocketActorLimit, "actor_limit"
	case len(r.leases) >= r.limits.perReplica:
		refusal, reason = ErrSocketReplicaLimit, "replica_limit"
	}
	if refusal != nil {
		if r.metrics != nil {
			r.metrics.ResponsesWebSocketRefusals.WithLabelValues(provider, reason).Inc()
		}
		return nil, refusal
	}
	leaseCtx, cancel := context.WithCancelCause(context.WithoutCancel(ctx))
	lease := &SocketLease{registry: r, actorID: actorID, provider: provider, ctx: leaseCtx, cancel: cancel}
	lease.timer = r.clock.AfterFunc(r.limits.lifetime, func() { cancel(ErrSocketLifetime) }, "socket", "lifetime")
	r.leases[lease] = struct{}{}
	r.perActor[actorID]++
	if r.metrics != nil {
		r.metrics.ResponsesWebSocketsOpen.WithLabelValues(provider).Inc()
	}
	return lease, nil
}

// Context returns the server-owned context of the socket. It ends when the
// lease is released, when the socket reached its maximum lifetime
// (ErrSocketLifetime), or when the server shuts down (ErrShutdown); read the
// reason with context.Cause.
func (l *SocketLease) Context() context.Context {
	return l.ctx
}

// Release ends the lease and frees its place. It is idempotent.
func (l *SocketLease) Release() {
	l.once.Do(func() {
		l.timer.Stop()
		l.cancel(nil)
		r := l.registry
		r.mu.Lock()
		delete(r.leases, l)
		r.perActor[l.actorID]--
		if r.perActor[l.actorID] == 0 {
			delete(r.perActor, l.actorID)
		}
		if r.closed && len(r.leases) == 0 {
			close(r.drained)
		}
		r.mu.Unlock()
		if r.metrics != nil {
			r.metrics.ResponsesWebSocketsOpen.WithLabelValues(l.provider).Dec()
		}
	})
}

// Shutdown refuses new leases, ends every lease context with ErrShutdown,
// and waits for every lease to be released or ctx to end, whichever comes
// first.
func (r *SocketRegistry) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		if len(r.leases) == 0 {
			close(r.drained)
		}
	}
	leases := make([]*SocketLease, 0, len(r.leases))
	for lease := range r.leases {
		leases = append(leases, lease)
	}
	r.mu.Unlock()
	for _, lease := range leases {
		lease.cancel(ErrShutdown)
	}
	select {
	case <-r.drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
