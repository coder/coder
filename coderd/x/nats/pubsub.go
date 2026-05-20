package nats

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

// Pubsub is an experimental embedded NATS-backed implementation of
// pubsub.Pubsub. See package doc for status.
//
// Connection model: Pubsub owns one embedded server and two pools of
// TCP-loopback *natsgo.Conns dialed at the server's client listener:
// one or more publishPool entries for publishes (configurable via
// Options.PublishConns, default 1) and one or more subscribePool
// entries for subscriptions (configurable via Options.SubscribeConns,
// default 1). Each Publish call selects a publisher connection by a
// stable hash of the subject so same-subject publishes preserve
// per-subject ordering. Each shared NATS subscription is likewise
// pinned to a subscriber connection by a stable hash of its subject,
// so a subject's underlying *natsgo.Subscription always lives on the
// same subscriber connection within a process. Local fan-out for a
// subject still coalesces onto that single shared NATS subscription;
// the pool only distributes shared subscriptions for distinct
// subjects across multiple TCP read/parser loops and per-conn
// server-side pending budgets.
type Pubsub struct {
	logger slog.Logger
	opts   Options

	ns *natsserver.Server
	// publishPool carries all publishes. Length is determined by
	// Options.PublishConns (default 1). Publish selects an entry by a
	// stable hash of the subject. The slice itself is immutable
	// after construction so the Publish hot path can index without
	// holding p.mu.
	publishPool []*natsgo.Conn
	// subscribePool carries every subscription created via Subscribe /
	// SubscribeWithErr. Length is determined by Options.SubscribeConns
	// (default 1). Each shared subscription is assigned an entry by
	// a stable hash of its subject so the assignment is deterministic
	// and same-subject subscribers all land on the same underlying
	// *natsgo.Subscription on the same connection. The slice itself
	// is immutable after construction so the subscribe hot path can
	// index without holding p.mu.
	subscribePool []*natsgo.Conn

	mu sync.Mutex
	// subs is the set of all local listeners across all subjects. Each
	// element is one Subscribe / SubscribeWithErr call's local handle.
	subs map[*subscription]struct{}
	// sharedBySubject coalesces concurrent local subscribers on the
	// same NATS subject onto a single underlying *natsgo.Subscription.
	// See sharedSub.
	sharedBySubject map[string]*sharedSub
	// sharedByNATS routes async NATS subscription-level errors (notably
	// ErrSlowConsumer) back to the sharedSub that owns them.
	sharedByNATS map[*natsgo.Subscription]*sharedSub
	// eventCounts tracks the number of local listeners per legacy event
	// name. Maintained for backward compatibility; unused by the
	// wrapper itself.
	eventCounts map[string]int
	closeOnce   sync.Once

	// ctx is canceled by Close to signal the hot path (Publish, Flush,
	// SubscribeWithErr) without taking p.mu. Close cancels it before
	// acquiring p.mu so racing callers bail before touching the
	// underlying *natsgo.Conn.
	ctx    context.Context
	cancel context.CancelFunc

	// testHookBeforeFlush and testHookBeforeSetPendingLimits are
	// internal test seams scoped to a single *Pubsub. Production code
	// never sets these. Tests set them via SetTestHooks (defined in a
	// _test.go file) to deterministically reproduce concurrent-attach,
	// Close-during-init, and Flush / SetPendingLimits failure races
	// without time.Sleep. Per-Pubsub scoping lets parallel tests
	// install distinct hooks without stomping on each other.
	testHookBeforeFlush            func(subject string)
	testHookBeforeSetPendingLimits func(subject string)
	// testHookOnFlushConn is invoked at the start of Flush for every
	// publisher connection, indexed by its position in publishPool. Used
	// by publish-pool tests to assert Flush touches every connection.
	// Production code never sets this.
	testHookOnFlushConn func(idx int)
}

// sharedSub coalesces local subscribers on the same NATS subject onto a
// single *natsgo.Subscription. The first local subscriber for a subject
// creates the underlying subscription; later subscribers attach to it.
// When the last local subscriber detaches, the underlying subscription
// is drained / unsubscribed.
//
// Readiness model: the creator inserts a sharedSub in an "initializing"
// state under p.mu (ready is open, sub is nil). It then performs the
// NATS Subscribe / Flush / SetPendingLimits sequence outside p.mu.
// On success it stores sub under p.mu and closes ready. On failure it
// stores readyErr, removes the shared entry from p.sharedBySubject /
// p.sharedByNATS, unsubscribes the underlying NATS sub (if any), and
// closes ready. Joiners attach under p.mu, then wait on ready outside
// the lock before returning, so they cannot observe a half-initialized
// shared subscription or publish before the SUB has reached the server.
//
// All mutable fields below (listeners, sub, lastDropped) are protected by
// the parent Pubsub.mu, except dropMu / lastDropped which use their own
// mutex so the async error callback can update drop accounting without
// taking the parent Pubsub.mu. readyErr is written exactly once by the
// creator before close(ready) and is observable by joiners after
// <-ready completes via channel-close happens-before.
type sharedSub struct {
	// subject is the full NATS subject this shared subscription is
	// registered against.
	subject string
	// sub is the underlying *natsgo.Subscription. Lifecycle is tied to
	// listeners: created on the first attach, drained/unsubscribed
	// when the last listener detaches. Set under p.mu by the creator
	// after a successful NATS Subscribe; reads outside p.mu are only
	// safe after <-ready confirms init completed.
	sub *natsgo.Subscription
	// listeners is the set of local listeners attached to this shared
	// subscription. Guarded by p.mu.
	listeners map[*subscription]struct{}

	// ready is closed by the creator after NATS Subscribe + Flush +
	// SetPendingLimits complete (success or failure). Joiners wait on
	// it (with a p.ctx.Done() escape) so they never return success
	// before the underlying subscription is registered and limit-set
	// at the server, and they never observe a half-initialized shared
	// sub. Set once by the creator at construction time.
	ready chan struct{}
	// readyErr is the init error if init failed; nil on success.
	// Written by the creator before close(ready); read by joiners
	// after <-ready. The channel close acts as the happens-before
	// barrier for readyErr.
	readyErr error

	// dropMu guards lastDropped, which dedups
	// pubsub.ErrDroppedMessages broadcasts: NATS reports a cumulative
	// dropped-count per subscription, so we only broadcast a new
	// callback when the count advances.
	dropMu      sync.Mutex
	lastDropped uint64
}

// subscription is the local handle a Subscribe / SubscribeWithErr
// caller holds. Each local subscriber gets its own bounded inbox and
// dispatcher goroutine so a single slow listener cannot block deliveries
// to its peers on the same subject.
type subscription struct {
	// sub aliases shared.sub so existing internal tests that reach into
	// s.sub directly continue to compile. Do not call Unsubscribe /
	// Drain via this field: the shared subscription's lifecycle is
	// managed by Pubsub via shared.
	sub        *natsgo.Subscription
	cancelOnce sync.Once

	event    string
	listener pubsub.ListenerWithErr

	// shared is the per-subject coalescing entry this listener is
	// attached to. Never nil after a successful Subscribe.
	shared *sharedSub

	// queue is the per-listener data fan-out inbox. The shared NATS
	// callback enqueues non-blockingly; when full, the message is
	// dropped and a signal is pushed onto dropSignal so this listener
	// learns about the drop independent of dispatcher progress.
	queue chan []byte
	// dropSignal is a size-1 buffered channel used to wake the drop
	// emitter goroutine without blocking. Multiple drop sources
	// (local overflow, NATS slow-consumer broadcast) coalesce onto a
	// single pending signal between emitter dequeues.
	dropSignal chan struct{}
	// stop is closed by cancelFn to signal both dispatcher and drop
	// emitter to exit.
	stop chan struct{}
	// dispatcherDone is closed by the dispatcher goroutine on exit;
	// cancel waits on it so any in-flight data user callback completes
	// before Drain.
	dispatcherDone chan struct{}
	// emitterDone is closed by the drop emitter goroutine on exit.
	emitterDone chan struct{}
}

// Compile-time assertion that *Pubsub satisfies the pubsub.Pubsub interface.
var _ pubsub.Pubsub = (*Pubsub)(nil)

// newPubsub allocates a *Pubsub with maps initialized.
func newPubsub(logger slog.Logger, opts Options) *Pubsub {
	ctx, cancel := context.WithCancel(context.Background())
	return &Pubsub{
		logger:          logger,
		opts:            opts,
		subs:            make(map[*subscription]struct{}),
		sharedBySubject: make(map[string]*sharedSub),
		sharedByNATS:    make(map[*natsgo.Subscription]*sharedSub),
		eventCounts:     make(map[string]int),
		ctx:             ctx,
		cancel:          cancel,
	}
}

// defaultPendingLimits returns the effective per-subscription pending
// limits applied at Subscribe time. If the caller left Options.PendingLimits
// fully zero, we default to {Msgs: -1, Bytes: 512 MiB} so wide fan-out
// workloads aren't truncated by nats.go's default limits. Any explicit
// caller value wins.
func defaultPendingLimits(in PendingLimits) PendingLimits {
	if in.Msgs == 0 && in.Bytes == 0 {
		return PendingLimits{Msgs: -1, Bytes: 512 * 1024 * 1024}
	}
	return in
}

// buildConnHandlers returns the connHandlers stack installed on every
// connection the wrapper owns. Handlers are closures over p so
// slow-consumer routing keeps working.
func (p *Pubsub) buildConnHandlers() connHandlers {
	return connHandlers{
		disconnectErr: func(_ *natsgo.Conn, err error) {
			if err != nil {
				p.logger.Warn(context.Background(), "nats client disconnected", slog.Error(err))
			}
		},
		reconnect: func(_ *natsgo.Conn) {
			p.logger.Info(context.Background(), "nats client reconnected")
		},
		closed: func(_ *natsgo.Conn) {
			p.logger.Debug(context.Background(), "nats client closed")
		},
		errH: func(_ *natsgo.Conn, sub *natsgo.Subscription, err error) {
			if err != nil && errors.Is(err, natsgo.ErrSlowConsumer) {
				p.handleAsyncError(sub, err)
				return
			}
			if err != nil {
				p.logger.Warn(context.Background(), "nats async error", slog.Error(err))
			}
		},
	}
}

// publishConnCount returns the effective number of publisher
// connections for opts. Zero or negative means 1 (single connection,
// historical default).
func publishConnCount(opts Options) int {
	if opts.PublishConns <= 0 {
		return 1
	}
	return opts.PublishConns
}

// subscribeConnCount returns the effective number of subscriber
// connections for opts. Zero or negative means 1 (single connection,
// historical default).
func subscribeConnCount(opts Options) int {
	if opts.SubscribeConns <= 0 {
		return 1
	}
	return opts.SubscribeConns
}

// New creates a new embedded NATS Pubsub. The returned *Pubsub owns the
// embedded server, one or more TCP-loopback publisher connections
// (Options.PublishConns, default 1), and one or more TCP-loopback
// subscriber connections (Options.SubscribeConns, default 1).
// Subscriptions for distinct subjects are distributed across the
// subscriber pool by a stable hash of the subject. Close shuts down
// all owned resources.
func New(_ context.Context, logger slog.Logger, opts Options) (*Pubsub, error) {
	ns, err := startEmbeddedServer(logger, opts)
	if err != nil {
		return nil, err
	}

	p := newPubsub(logger, opts)
	p.ns = ns

	npub := publishConnCount(opts)
	publishPool := make([]*natsgo.Conn, 0, npub)
	for i := 0; i < npub; i++ {
		// Per-conn name suffix when the pool has more than one entry
		// so server logs can distinguish them. With a single conn we
		// keep the historical "coder-pubsub-pub" name.
		name := "coder-pubsub-pub"
		if npub > 1 {
			name = fmt.Sprintf("coder-pubsub-pub-%d", i)
		}
		nc, err := connectClient(ns, opts, p.buildConnHandlers(), name)
		if err != nil {
			for _, c := range publishPool {
				c.Close()
			}
			ns.Shutdown()
			ns.WaitForShutdown()
			return nil, xerrors.Errorf("dial pub conn %d: %w", i, err)
		}
		publishPool = append(publishPool, nc)
	}
	nsub := subscribeConnCount(opts)
	subscribePool := make([]*natsgo.Conn, 0, nsub)
	for i := 0; i < nsub; i++ {
		// Per-conn name suffix when the pool has more than one entry
		// so server logs can distinguish them. With a single conn we
		// keep the historical "coder-pubsub-sub" name.
		name := "coder-pubsub-sub"
		if nsub > 1 {
			name = fmt.Sprintf("coder-pubsub-sub-%d", i)
		}
		nc, err := connectClient(ns, opts, p.buildConnHandlers(), name)
		if err != nil {
			for _, c := range publishPool {
				c.Close()
			}
			for _, c := range subscribePool {
				c.Close()
			}
			ns.Shutdown()
			ns.WaitForShutdown()
			return nil, xerrors.Errorf("dial sub conn %d: %w", i, err)
		}
		subscribePool = append(subscribePool, nc)
	}
	p.publishPool = publishPool
	p.subscribePool = subscribePool
	return p, nil
}

// pickPubConn returns the publisher connection for subject. The
// publishPool slice is immutable after construction so this lookup is
// safe without holding p.mu, keeping the Publish hot path lock-free.
//
// Selection uses a stable FNV-1a hash of the subject so same-subject
// publishes always target the same connection within a process. That
// preserves per-subject publish ordering: NATS guarantees ordering
// per-connection per-subject, and routing same-subject traffic to a
// single connection preserves that guarantee at the wrapper level.
// FNV-1a is deterministic (no per-process seed), which makes the
// selection reproducible across test runs.
func (p *Pubsub) pickPubConn(subject string) *natsgo.Conn {
	conns := p.publishPool
	if len(conns) == 1 {
		return conns[0]
	}
	h := fnv.New32a()
	// fnv.Hash32a.Write never returns an error.
	_, _ = h.Write([]byte(subject))
	// len(conns) is bounded by Options.PublishConns, which is set by
	// the caller and in practice is well below MaxInt32. The
	// int -> uint32 conversion is therefore safe.
	n := uint32(len(conns)) //nolint:gosec // pool size bounded by Options.PublishConns
	return conns[h.Sum32()%n]
}

// pickSubConn returns the subscriber connection assigned to subject.
// The subscribePool slice is immutable after construction so this lookup
// is safe without holding p.mu.
//
// Selection uses a stable FNV-1a hash of the subject so the chosen
// connection is deterministic per subject within a process. This
// pairs with same-subject subscription coalescing: every local
// subscriber for a subject attaches to the one shared
// *natsgo.Subscription registered on this conn. Distributing distinct
// subjects across multiple TCP read/parser loops and per-conn
// server-side pending budgets is the throughput reason for the pool;
// pinning a subject to a single conn keeps the shared-subscription
// model intact and makes async slow-consumer routing (which is keyed
// on *natsgo.Subscription, not on the owning conn) work unchanged.
//
// FNV-1a is deterministic (no per-process seed), which makes the
// selection reproducible across test runs.
func (p *Pubsub) pickSubConn(subject string) *natsgo.Conn {
	conns := p.subscribePool
	if len(conns) == 1 {
		return conns[0]
	}
	h := fnv.New32a()
	// fnv.Hash32a.Write never returns an error.
	_, _ = h.Write([]byte(subject))
	// len(conns) is bounded by Options.SubscribeConns, which is set
	// by the caller and in practice is well below MaxInt32.
	n := uint32(len(conns)) //nolint:gosec // pool size bounded by Options.SubscribeConns
	return conns[h.Sum32()%n]
}

// Publish publishes a message under the given legacy event name. The
// underlying NATS connection is selected by a stable hash of the
// resolved subject so same-subject publishes preserve per-subject
// ordering across multiple publisher connections.
func (p *Pubsub) Publish(event string, message []byte) error {
	if p.ctx.Err() != nil {
		return xerrors.New("nats pubsub: closed")
	}

	subj, err := LegacyEventSubject(event)
	if err != nil {
		return xerrors.Errorf("map event %q: %w", event, err)
	}
	if err := p.pickPubConn(string(subj)).Publish(string(subj), message); err != nil {
		return xerrors.Errorf("publish: %w", err)
	}
	return nil
}

// Flush blocks until every publisher connection has flushed all
// buffered publishes to the embedded server. Mirrors nats.Conn.Flush.
// Useful in benchmarks and tests where the caller needs to know that
// all preceding Publish calls have reached the server.
//
// Flush returns the first error encountered while flushing the pool;
// remaining connections are still flushed before returning so a
// transient error on one connection does not silently leave buffered
// publishes on another.
func (p *Pubsub) Flush() error {
	if p.ctx.Err() != nil {
		return xerrors.New("nats pubsub: closed")
	}

	var firstErr error
	for i, nc := range p.publishPool {
		if hook := p.testHookOnFlushConn; hook != nil {
			hook(i)
		}
		if err := nc.Flush(); err != nil && firstErr == nil {
			firstErr = xerrors.Errorf("flush pub conn %d: %w", i, err)
		}
	}
	return firstErr
}

// Subscribe subscribes a Listener to the given legacy event name. Errors
// such as ErrDroppedMessages are silently ignored, mirroring the legacy
// pubsub Listener semantics.
func (p *Pubsub) Subscribe(event string, listener pubsub.Listener) (cancel func(), err error) {
	return p.SubscribeWithErr(event, func(ctx context.Context, msg []byte, err error) {
		if err != nil {
			return
		}
		listener(ctx, msg)
	})
}

// SubscribeWithErr subscribes a ListenerWithErr to the given legacy event
// name. The listener also receives error deliveries such as
// pubsub.ErrDroppedMessages.
//
// Multiple local subscribers on the same event share a single underlying
// *natsgo.Subscription. Each local subscriber gets its own bounded inbox
// and dispatcher goroutine so a slow user listener can drop its own
// messages (surfaced as pubsub.ErrDroppedMessages) without blocking
// other listeners attached to the same shared subscription.
func (p *Pubsub) SubscribeWithErr(event string, listener pubsub.ListenerWithErr) (cancel func(), err error) {
	if p.ctx.Err() != nil {
		return nil, xerrors.New("nats pubsub: closed")
	}

	subj, err := LegacyEventSubject(event)
	if err != nil {
		return nil, xerrors.Errorf("map event %q: %w", event, err)
	}

	s := &subscription{
		event:          event,
		listener:       listener,
		queue:          make(chan []byte, listenerQueueSize(p.opts.PendingLimits)),
		dropSignal:     make(chan struct{}, 1),
		stop:           make(chan struct{}),
		dispatcherDone: make(chan struct{}),
		emitterDone:    make(chan struct{}),
	}

	// Start the per-listener goroutines BEFORE inserting s into p.subs.
	// Once attachListener registers s, Close may snapshot it and wait
	// on s.dispatcherDone / s.emitterDone. Starting the goroutines first
	// guarantees those channels are owned by live goroutines that will
	// observe a future close(s.stop) and exit, so Close can never
	// deadlock on goroutines that were never scheduled. The goroutines
	// idle on s.queue / s.dropSignal / s.stop until either work arrives
	// or s.stop closes.
	go s.dispatch()
	go s.emitDrops()

	// stopGoroutines tears down the listener goroutines started above.
	// Used on every error path so we never leak a goroutine pair when
	// attach or readiness fails.
	stopGoroutines := func() {
		s.cancelOnce.Do(func() {
			close(s.stop)
			<-s.dispatcherDone
			<-s.emitterDone
		})
	}

	// attachListener registers s in the per-subject coalescing tables
	// and, on the first attach for this subject, drives the underlying
	// NATS Subscribe / Flush / SetPendingLimits sequence. It does not
	// return until the shared subscription is fully ready or has
	// deterministically failed. On error it has already detached s and
	// (when this caller was the creator) cleaned up the shared registry
	// state and the underlying NATS subscription. Joiners on a failed
	// creator are also detached.
	shared, _, err := p.attachListener(string(subj), s)
	if err != nil {
		stopGoroutines()
		return nil, err
	}
	s.shared = shared
	s.sub = shared.sub

	// Final guard against Close racing in between attachListener
	// returning success and us handing a cancel function to the caller.
	// If Close has begun, every other code path will see p.ctx.Err()
	// and bail; we mirror that behavior here so callers never receive
	// a "successful" cancel function for a Pubsub that is being torn
	// down. The cancel-once interlock with Close means our cleanup
	// here is harmless if Close already cleaned us up.
	if p.ctx.Err() != nil {
		toDrain := p.detachListener(s)
		stopGoroutines()
		if toDrain != nil {
			p.drainShared(toDrain)
		}
		return nil, xerrors.New("nats pubsub: closed")
	}

	cancelFn := func() {
		s.cancelOnce.Do(func() {
			// detachListener returns the shared entry to drain when s
			// was the last listener (otherwise nil).
			toDrain := p.detachListener(s)
			// Signal both goroutines to exit and wait for in-flight
			// user callbacks to complete. The shared NATS callback
			// may still attempt a non-blocking send to s.queue
			// concurrently; it uses a select on s.stop and silently
			// drops in that case so there is no panic on a
			// closed-but-still-targeted queue.
			close(s.stop)
			<-s.dispatcherDone
			<-s.emitterDone
			if toDrain != nil {
				p.drainShared(toDrain)
			}
		})
	}
	return cancelFn, nil
}

// listenerQueueSize returns the per-listener inbox channel capacity.
// When the caller explicitly sets PendingLimits.Msgs to a positive
// value we use that as the local-queue cap too: same-subject
// coalescing means tight pending limits on the underlying
// *natsgo.Subscription are no longer sufficient to surface
// pubsub.ErrDroppedMessages on their own (the shared NATS callback
// drains the per-sub pending queue quickly into per-listener inboxes,
// so the NATS-level slow-consumer signal rarely fires). Sizing the
// local inbox from PendingLimits.Msgs gives callers a knob that
// reliably triggers local-overflow drops when they want it. When the
// caller leaves PendingLimits at the zero or unlimited setting, we
// use a generous default.
func listenerQueueSize(in PendingLimits) int {
	if in.Msgs > 0 {
		return in.Msgs
	}
	return defaultListenerQueueSize
}

// defaultListenerQueueSize is the per-listener inbox channel capacity
// applied when the caller has not opted into a tighter PendingLimits.
// It is large enough to absorb realistic publish bursts while still
// bounding the per-listener memory footprint at a few KiB of pointers.
const defaultListenerQueueSize = 1024

// attachListener attaches s to the sharedSub for subject and blocks
// until the shared subscription is ready (or has deterministically
// failed).
//
// The first attacher for a subject becomes the creator: it inserts a
// sharedSub in an initializing state under p.mu, then issues NATS
// Subscribe / Flush / SetPendingLimits outside the lock. On success it
// stores the underlying *natsgo.Subscription under p.mu and closes
// shared.ready. On failure it stores shared.readyErr, force-removes the
// shared entry from the registries so future Subscribes start fresh,
// unsubscribes the underlying NATS subscription if one was created,
// detaches s, and closes shared.ready so any joiners that attached in
// the meantime wake up and clean up too.
//
// Joiners (second and later attachers for the same subject) insert
// themselves into the existing shared.listeners under p.mu, then wait
// outside the lock for either shared.ready or p.ctx.Done(). They never
// return success before the creator has flushed the SUB to the server
// and set pending limits, so a Publish issued immediately after the
// second SubscribeWithErr returns cannot race ahead of subscription
// registration.
//
// On any error path attachListener has already detached s and (for the
// creator) cleaned up registry / NATS state, so the caller need only
// stop its already-started listener goroutines and return the error.
//
// The returned bool reports whether this call created the shared
// subscription; it is preserved for symmetry but is currently
// informational only (callers no longer drive Flush / SetPendingLimits
// themselves; attachListener owns the full readiness sequence).
func (p *Pubsub) attachListener(subject string, s *subscription) (*sharedSub, bool, error) {
	p.mu.Lock()
	// Authoritative close check while holding p.mu. Close sets
	// p.ctx.Err() before acquiring p.mu, so any registration that
	// makes it past this point is guaranteed to be observed by Close's
	// snapshot of p.subs (which is also taken under p.mu). This is
	// what prevents Close from missing a sub that is registering
	// concurrently.
	if p.ctx.Err() != nil {
		p.mu.Unlock()
		return nil, false, xerrors.New("nats pubsub: closed")
	}
	if shared, ok := p.sharedBySubject[subject]; ok {
		// Joiner path: register before unlocking so the creator's
		// success callback path (and any subsequent fan-out) sees
		// this listener in shared.listeners, and so Close sees s in
		// p.subs.
		shared.listeners[s] = struct{}{}
		s.shared = shared
		p.subs[s] = struct{}{}
		p.eventCounts[s.event]++
		p.mu.Unlock()

		// Wait for the creator's NATS Subscribe + Flush +
		// SetPendingLimits to complete (or fail) before returning.
		// p.ctx.Done() lets us bail if Close happens while we're
		// waiting; in that case the joiner cleans itself up and
		// reports the closed error. We do not hold p.mu here so
		// concurrent fan-out and Close can both make progress.
		select {
		case <-shared.ready:
		case <-p.ctx.Done():
			p.detachListener(s)
			return nil, false, xerrors.New("nats pubsub: closed")
		}
		if shared.readyErr != nil {
			// Creator failed; clean ourselves up. The creator has
			// already removed shared from the registries and
			// unsubscribed the underlying NATS sub, so detach is
			// just listener bookkeeping for this s.
			p.detachListener(s)
			return nil, false, xerrors.Errorf("shared subscription init: %w", shared.readyErr)
		}
		return shared, false, nil
	}

	// Creator path: insert a placeholder shared in the initializing
	// state. Joiners that arrive between now and close(shared.ready)
	// will find this entry and wait. We hold p.mu only long enough to
	// register the placeholder; the network I/O below runs lock-free.
	shared := &sharedSub{
		subject:   subject,
		listeners: map[*subscription]struct{}{s: {}},
		ready:     make(chan struct{}),
	}
	s.shared = shared
	p.sharedBySubject[subject] = shared
	p.subs[s] = struct{}{}
	p.eventCounts[s.event]++
	p.mu.Unlock()

	// finishCreator publishes the init result to joiners and (on
	// failure) tears down all registry state we inserted above and
	// any underlying NATS subscription we created. It is invoked
	// exactly once on every creator-path return.
	finishCreator := func(initErr error) (*sharedSub, bool, error) {
		if initErr == nil {
			close(shared.ready)
			return shared, true, nil
		}

		// Failure: force-remove the shared entry from the registries
		// so that (a) future Subscribes for this subject create a
		// fresh shared rather than attaching to this dead one, and
		// (b) Close's snapshot iteration doesn't see a phantom entry
		// after we close ready. Also detach self.
		p.mu.Lock()
		if cur, ok := p.sharedBySubject[subject]; ok && cur == shared {
			delete(p.sharedBySubject, subject)
		}
		if shared.sub != nil {
			if cur, ok := p.sharedByNATS[shared.sub]; ok && cur == shared {
				delete(p.sharedByNATS, shared.sub)
			}
		}
		delete(shared.listeners, s)
		delete(p.subs, s)
		if c, ok := p.eventCounts[s.event]; ok {
			if c <= 1 {
				delete(p.eventCounts, s.event)
			} else {
				p.eventCounts[s.event] = c - 1
			}
		}
		natsSub := shared.sub
		p.mu.Unlock()

		shared.readyErr = initErr
		// Unsubscribe the underlying NATS sub if we created one.
		// detachListener does not do this for us, and we cannot rely
		// on Close to drain the conn because the caller's
		// SubscribeWithErr will return now with an error.
		if natsSub != nil {
			_ = natsSub.Unsubscribe()
		}
		// Close ready last so any joiners that attached between
		// placeholder insertion and finishCreator wake up after the
		// registry cleanup is complete and observe a consistent
		// state when they call detachListener.
		close(shared.ready)
		return nil, false, initErr
	}

	// Choose the subscriber connection for this subject. Subject hashing
	// keeps the choice deterministic so same-subject subscribers all
	// coalesce onto the same underlying *natsgo.Subscription on the
	// same conn, and async slow-consumer routing (keyed on
	// *natsgo.Subscription via sharedByNATS) keeps working regardless
	// of which conn owns the subscription.
	subConn := p.pickSubConn(subject)
	natsSub, err := subConn.Subscribe(subject, shared.makeCallback(p))
	if err != nil {
		return finishCreator(xerrors.Errorf("subscribe: %w", err))
	}

	// Publish shared.sub and sharedByNATS under p.mu before we start
	// the network-side readiness handshake. This makes the natsSub
	// observable from Close, async error routing, and (after ready
	// closes) from joiners. shared.sub never reverts to nil after
	// being set; if init fails below, finishCreator removes the
	// sharedByNATS entry and calls Unsubscribe but leaves shared.sub
	// pointing at the now-dead *natsgo.Subscription so debug paths
	// and tests can still inspect it.
	p.mu.Lock()
	shared.sub = natsSub
	p.sharedByNATS[natsSub] = shared
	p.mu.Unlock()

	// Test seam: simulate a Flush failure or expose the initialization
	// window deterministically. Production code never sets this hook.
	if hook := p.testHookBeforeFlush; hook != nil {
		hook(subject)
	}

	// Flush the SUB protocol message to the server before returning
	// so a publish issued immediately after Subscribe cannot race
	// ahead of subscription registration. This is the critical
	// readiness gate that joiners are waiting on. We flush the
	// subscriber connection that owns natsSub, not an arbitrary entry
	// of p.subscribePool, so the SUB protocol message we just enqueued is
	// the one we wait for.
	if err := subConn.Flush(); err != nil {
		return finishCreator(xerrors.Errorf("flush subscribe: %w", err))
	}
	if hook := p.testHookBeforeSetPendingLimits; hook != nil {
		hook(subject)
	}
	limits := defaultPendingLimits(p.opts.PendingLimits)
	if err := natsSub.SetPendingLimits(limits.Msgs, limits.Bytes); err != nil {
		return finishCreator(xerrors.Errorf("set pending limits: %w", err))
	}
	return finishCreator(nil)
}

// detachListener removes s from its shared subscription and from the
// Pubsub-wide tracking maps. When s was the last listener on its
// shared subscription, the shared entry is also removed from the
// registries and returned so the caller can drain / unsubscribe the
// underlying *natsgo.Subscription outside p.mu. Otherwise returns nil.
//
// Safe to call multiple times: subsequent calls find s already
// detached and become no-ops.
func (p *Pubsub) detachListener(s *subscription) *sharedSub {
	p.mu.Lock()
	if _, tracked := p.subs[s]; !tracked {
		p.mu.Unlock()
		return nil
	}
	delete(p.subs, s)
	if c, ok := p.eventCounts[s.event]; ok {
		if c <= 1 {
			delete(p.eventCounts, s.event)
		} else {
			p.eventCounts[s.event] = c - 1
		}
	}
	shared := s.shared
	if shared == nil {
		p.mu.Unlock()
		return nil
	}
	delete(shared.listeners, s)
	if len(shared.listeners) > 0 {
		p.mu.Unlock()
		return nil
	}
	// Last listener: remove the shared entry from registries so a new
	// Subscribe to the same subject creates a fresh underlying
	// subscription rather than attaching to a draining one. Use
	// identity-checked deletes because a parallel creator-failure
	// path may have already replaced this entry with a new shared
	// (or removed it entirely) since we last looked.
	if cur, ok := p.sharedBySubject[shared.subject]; ok && cur == shared {
		delete(p.sharedBySubject, shared.subject)
	}
	if shared.sub != nil {
		if cur, ok := p.sharedByNATS[shared.sub]; ok && cur == shared {
			delete(p.sharedByNATS, shared.sub)
		}
	}
	p.mu.Unlock()
	// If the underlying NATS subscription was never published (creator
	// failed before storing sub, or this listener attached as a
	// joiner to a failed-init shared whose sub field was cleared),
	// the caller should not try to drain it.
	if shared.sub == nil {
		return nil
	}
	return shared
}

// drainShared drains and unsubscribes the underlying NATS subscription
// for shared. Called when the last local listener detaches.
func (p *Pubsub) drainShared(shared *sharedSub) {
	drainTimeout := p.opts.DrainTimeout
	if drainTimeout <= 0 {
		drainTimeout = 5 * time.Second
	}
	done := make(chan error, 1)
	go func() { done <- shared.sub.Drain() }()
	select {
	case <-done:
	case <-time.After(drainTimeout):
		_ = shared.sub.Unsubscribe()
	}
}

// makeCallback returns the *natsgo.Conn callback installed on the
// shared *natsgo.Subscription. It snapshots the listener set under
// p.mu, then performs a non-blocking enqueue per listener so no single
// slow listener can stall the NATS delivery goroutine.
//
// Zero-copy fan-out: msg.Data is delivered to every local listener as
// the same []byte (no clone). This preserves throughput for large
// payloads and matches the legacy single-subscriber path's semantics,
// where the user already received the *natsgo.Msg payload directly.
// Listeners attached to a coalesced subject MUST treat the delivered
// bytes as immutable: do not mutate the slice, retain a reference past
// the callback, or pass it to anything that may mutate it. The slice
// is owned by nats.go's per-conn read buffer and is reused for the
// next message once all listeners' callbacks return.
func (ss *sharedSub) makeCallback(p *Pubsub) natsgo.MsgHandler {
	return func(msg *natsgo.Msg) {
		// Snapshot listeners under p.mu so concurrent detach /
		// attach observes a consistent view. The snapshot is small in
		// the common case (<= a handful of subscribers per subject)
		// and we don't invoke user callbacks while holding the lock.
		p.mu.Lock()
		listeners := make([]*subscription, 0, len(ss.listeners))
		for s := range ss.listeners {
			listeners = append(listeners, s)
		}
		p.mu.Unlock()
		for _, s := range listeners {
			// Pass msg.Data directly: every listener observes the
			// same backing array. See zero-copy contract above.
			s.offerData(msg.Data)
		}
	}
}

// offerData performs a non-blocking enqueue of data onto s.queue. The
// select prefers a successful send; if s has been canceled (stop
// closed) it silently drops; otherwise if the queue is full the
// message is dropped and a drop signal is raised so the emitter
// goroutine surfaces it to the user listener as
// pubsub.ErrDroppedMessages, independent of dispatcher progress.
func (s *subscription) offerData(data []byte) {
	select {
	case s.queue <- data:
	case <-s.stop:
	default:
		s.signalDrop()
	}
}

// signalDrop pushes onto dropSignal without blocking. Multiple drop
// sources between emitter dequeues coalesce onto a single pending
// signal, so the user listener observes one ErrDroppedMessages
// callback per drop wave rather than per dropped message.
func (s *subscription) signalDrop() {
	select {
	case s.dropSignal <- struct{}{}:
	default:
	}
}

// dispatch is the per-listener data delivery goroutine. It serializes
// data callbacks for one subscriber while a separate emitter goroutine
// delivers drop notifications, so a slow user listener cannot prevent
// pubsub.ErrDroppedMessages from being surfaced.
func (s *subscription) dispatch() {
	defer close(s.dispatcherDone)
	for {
		select {
		case <-s.stop:
			return
		case data := <-s.queue:
			s.listener(context.Background(), data, nil)
		}
	}
}

// emitDrops is the per-listener drop-notification goroutine. It runs
// concurrently with dispatch so a blocked data callback does not
// prevent drop signaling. The existing wrapper already permitted
// concurrent listener invocations: in the previous code path drop
// callbacks were dispatched on the NATS connection's async error
// goroutine while data callbacks ran on the per-subscription delivery
// goroutine.
func (s *subscription) emitDrops() {
	defer close(s.emitterDone)
	for {
		select {
		case <-s.stop:
			return
		case <-s.dropSignal:
			s.listener(context.Background(), nil, pubsub.ErrDroppedMessages)
		}
	}
}

// handleAsyncError routes async error callbacks. Only slow-consumer
// errors trigger drop accounting; other errors are ignored here and
// logged elsewhere.
func (p *Pubsub) handleAsyncError(sub *natsgo.Subscription, err error) {
	if sub == nil || !errors.Is(err, natsgo.ErrSlowConsumer) {
		return
	}
	p.mu.Lock()
	shared, ok := p.sharedByNATS[sub]
	p.mu.Unlock()
	if !ok {
		return
	}
	p.handleSharedSlowConsumer(shared)
}

// handleSlowConsumer is preserved for white-box test access. It
// forwards to handleSharedSlowConsumer on the listener's shared
// subscription. Production code paths use handleSharedSlowConsumer
// directly via handleAsyncError.
func (p *Pubsub) handleSlowConsumer(s *subscription) {
	if s == nil || s.shared == nil {
		return
	}
	p.handleSharedSlowConsumer(s.shared)
}

// handleSharedSlowConsumer is invoked for async slow-consumer signals
// on a shared subscription. It queries NATS for the cumulative dropped
// count and, on each new delta, broadcasts pubsub.ErrDroppedMessages
// to every local listener attached to the shared subscription. The
// underlying NATS slow-consumer signal is per-subscription, so we
// cannot narrow it to a single local listener.
func (p *Pubsub) handleSharedSlowConsumer(shared *sharedSub) {
	shared.dropMu.Lock()
	dropped, err := shared.sub.Dropped()
	if err != nil {
		shared.dropMu.Unlock()
		p.logger.Warn(context.Background(), "nats: query dropped count", slog.Error(err))
		return
	}
	if dropped < 0 {
		shared.dropMu.Unlock()
		p.logger.Warn(context.Background(), "nats: negative dropped count")
		return
	}
	cur := uint64(dropped)
	if cur < shared.lastDropped {
		shared.lastDropped = cur
		shared.dropMu.Unlock()
		return
	}
	delta := cur - shared.lastDropped
	if delta == 0 {
		shared.dropMu.Unlock()
		return
	}
	shared.lastDropped = cur
	shared.dropMu.Unlock()

	// Snapshot the listener set under p.mu so we don't hold the lock
	// while invoking user callbacks via the dispatcher.
	p.mu.Lock()
	listeners := make([]*subscription, 0, len(shared.listeners))
	for s := range shared.listeners {
		listeners = append(listeners, s)
	}
	p.mu.Unlock()
	for _, s := range listeners {
		s.signalDrop()
	}
}

// Close drains and shuts down the Pubsub. It is idempotent.
func (p *Pubsub) Close() error {
	var errs []error
	p.closeOnce.Do(func() {
		// Signal the hot path before taking p.mu so racing Publish /
		// Flush / Subscribe calls bail before touching publishPool/subscribePool.
		p.cancel()
		p.mu.Lock()
		subs := make([]*subscription, 0, len(p.subs))
		for s := range p.subs {
			subs = append(subs, s)
		}
		shareds := make([]*sharedSub, 0, len(p.sharedBySubject))
		for _, ss := range p.sharedBySubject {
			shareds = append(shareds, ss)
		}
		p.mu.Unlock()

		// Unsubscribe each shared subscription. Don't drain
		// individually here; we drain every owned subConn as a whole
		// below. shared.sub may be nil if a creator is still mid-init
		// at Close time; the conn drains below tear that case down.
		for _, ss := range shareds {
			if ss.sub != nil {
				_ = ss.sub.Unsubscribe()
			}
		}

		// Stop every per-listener dispatcher goroutine and wait for
		// in-flight user callbacks to complete. We do this on the
		// originating subscription handles (not via cancelFn) so the
		// individual cancel paths do not also try to drain shared
		// subscriptions; the subConn drains below handle flushing
		// in-flight server-to-client deliveries.
		for _, s := range subs {
			s.cancelOnce.Do(func() {
				close(s.stop)
				<-s.dispatcherDone
				<-s.emitterDone
			})
		}

		// Clear tracking maps so a post-Close inspection sees no
		// dangling state.
		p.mu.Lock()
		for s := range p.subs {
			delete(p.subs, s)
		}
		for k := range p.sharedBySubject {
			delete(p.sharedBySubject, k)
		}
		for k := range p.sharedByNATS {
			delete(p.sharedByNATS, k)
		}
		for k := range p.eventCounts {
			delete(p.eventCounts, k)
		}
		p.mu.Unlock()

		drainTimeout := p.opts.DrainTimeout
		if drainTimeout <= 0 {
			drainTimeout = 30 * time.Second
		}

		// Drain every subscriber connection first so any in-flight
		// deliveries flush to listeners, then close them.
		for i, nc := range p.subscribePool {
			if nc == nil {
				continue
			}
			if err := drainConn(nc, drainTimeout); err != nil {
				errs = append(errs, xerrors.Errorf("drain sub conn %d: %w", i, err))
			}
		}
		// Drain every publisher connection.
		for i, nc := range p.publishPool {
			if nc == nil {
				continue
			}
			if err := drainConn(nc, drainTimeout); err != nil {
				errs = append(errs, xerrors.Errorf("drain pub conn %d: %w", i, err))
			}
		}

		if p.ns != nil {
			p.ns.Shutdown()
			p.ns.WaitForShutdown()
		}
	})
	return errors.Join(errs...)
}

// drainConn issues Drain on nc and waits for it to reach the closed
// state, falling back to Close after the timeout.
func drainConn(nc *natsgo.Conn, timeout time.Duration) error {
	if nc.IsClosed() {
		return nil
	}
	if err := nc.Drain(); err != nil {
		nc.Close()
		return err
	}
	deadline := time.Now().Add(timeout)
	for !nc.IsClosed() {
		if time.Now().After(deadline) {
			nc.Close()
			return xerrors.Errorf("drain timeout after %s", timeout)
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}
