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

// Default values for Options.
const (
	DefaultReadyTimeout = 10 * time.Second
	// DefaultMaxPending is the per-client outbound pending byte budget
	// (1 GiB), raised from the nats-server default of 64 MiB so wide
	// local fan-out does not trip the slow-consumer threshold.
	DefaultMaxPending int64 = 1 << 30
)

// PendingLimits configures per-subscription NATS pending limits set
// via SetPendingLimits on each *natsgo.Subscription.
type PendingLimits struct {
	// Msgs is the per-subscription pending message limit.
	// Zero keeps the NATS client default. Negative disables this limit.
	Msgs int

	// Bytes is the per-subscription pending byte limit.
	// Zero keeps the NATS client default. Negative disables this limit.
	Bytes int
}

// Options configures the embedded NATS Pubsub.
type Options struct {
	// ServerName is the NATS server name. If empty, New derives one.
	ServerName string

	// ClientName is the NATS client name. If empty, New derives one.
	ClientName string

	// MaxPayload is the NATS max payload. Zero means server default.
	MaxPayload int32

	// MaxPending is the per-client outbound pending byte budget on the
	// embedded server. Zero means DefaultMaxPending; negative means use
	// the nats-server default (64 MiB).
	MaxPending int64

	// DrainTimeout bounds subscription and connection drains in Close.
	// Zero means 30 seconds, matching the NATS Go client default.
	DrainTimeout time.Duration

	// PendingLimits configures per-subscription NATS pending limits.
	// If both fields are zero, New defaults to {Msgs: -1, Bytes: 512 MiB}.
	PendingLimits PendingLimits

	// ReadyTimeout bounds embedded server startup. Zero means
	// DefaultReadyTimeout.
	ReadyTimeout time.Duration

	// ReconnectWait controls client reconnect delay. Zero keeps the
	// NATS default.
	ReconnectWait time.Duration

	// InProcess, when true, uses nats.InProcessServer instead of TCP
	// loopback. Intended for benchmarks and tests.
	InProcess bool

	// PublishConns is the number of publisher connections. Each Publish
	// is routed by a stable hash of the subject. Zero or negative means 1.
	PublishConns int

	// SubscribeConns is the number of subscriber connections. Each
	// shared subscription is pinned to one connection by a stable hash
	// of its subject. Zero or negative means 1.
	SubscribeConns int
}

// Pubsub is an experimental embedded NATS-backed implementation of
// pubsub.Pubsub.
//
// Each Pubsub owns one embedded server, a pool of publisher
// *natsgo.Conns (Options.PublishConns) and a pool of subscriber
// *natsgo.Conns (Options.SubscribeConns). Publishes and shared
// subscriptions are pinned to a connection by a stable hash of the
// subject, so same-subject traffic preserves per-subject ordering and
// every local subscriber for a subject coalesces onto one underlying
// *natsgo.Subscription.
type Pubsub struct {
	logger slog.Logger
	opts   Options

	ns *natsserver.Server
	// publishPool and subscribePool are immutable after construction so
	// the hot path can index without holding p.mu.
	publishPool   []*natsgo.Conn
	subscribePool []*natsgo.Conn

	mu sync.Mutex
	// subs is the set of all local listener handles across all subjects.
	subs map[*subscription]struct{}
	// sharedBySubject coalesces concurrent local subscribers on the
	// same subject onto a single underlying *natsgo.Subscription.
	sharedBySubject map[string]*sharedSub
	// sharedByNATS routes async subscription-level errors (notably
	// ErrSlowConsumer) back to the owning sharedSub.
	sharedByNATS map[*natsgo.Subscription]*sharedSub
	// eventCounts tracks local listeners per event name. Retained for
	// backward compatibility; unused by the wrapper itself.
	eventCounts map[string]int
	closeOnce   sync.Once

	// ctx is canceled by Close before it acquires p.mu so racing hot
	// path callers (Publish, Flush, SubscribeWithErr) bail before
	// touching the underlying *natsgo.Conn.
	ctx    context.Context
	cancel context.CancelFunc

	// Test seam for readiness tests. Production code never sets this.
	testHookBeforeFlush func(subject string)
}

// sharedSub coalesces local subscribers on the same NATS subject onto
// a single *natsgo.Subscription. The first subscriber creates the
// underlying subscription; later subscribers attach to it. When the
// last subscriber detaches, the underlying subscription is drained.
//
// Readiness: the creator inserts a sharedSub in an "initializing"
// state under p.mu, performs NATS Subscribe / Flush / SetPendingLimits
// outside p.mu, then publishes the result by closing ready. Joiners
// wait on ready (with a p.ctx.Done() escape) so they never observe a
// half-initialized shared subscription. readyErr is written before
// close(ready); the channel close is the happens-before barrier.
//
// listeners and sub are guarded by the parent Pubsub.mu. dropMu /
// lastDropped use their own mutex so the async error callback can
// update drop accounting without taking p.mu.
type sharedSub struct {
	subject   string
	sub       *natsgo.Subscription
	listeners map[*subscription]struct{}

	ready    chan struct{}
	readyErr error

	dropMu      sync.Mutex
	lastDropped uint64
}

// subscription is the local handle returned by Subscribe /
// SubscribeWithErr. Each local subscriber gets its own bounded inbox
// and dispatcher goroutine so one slow listener cannot block peers on
// the same subject.
type subscription struct {
	// sub aliases shared.sub for white-box tests. Do not call
	// Unsubscribe / Drain via this field; the shared subscription's
	// lifecycle is managed by Pubsub via shared.
	sub        *natsgo.Subscription
	cancelOnce sync.Once

	event    string
	listener pubsub.ListenerWithErr

	// shared is the per-subject coalescing entry. Never nil after a
	// successful Subscribe.
	shared *sharedSub

	// queue is the per-listener data fan-out inbox. The shared NATS
	// callback enqueues non-blockingly; on overflow the message is
	// dropped and a drop signal is raised.
	queue chan []byte
	// dropSignal is a size-1 buffered channel that coalesces drop
	// notifications from local overflow and NATS slow-consumer
	// broadcasts onto a single pending wake.
	dropSignal chan struct{}
	// stop is closed by cancelFn to signal both goroutines to exit.
	stop chan struct{}
	// dispatcherDone / emitterDone are closed by the respective
	// goroutines on exit; cancel waits on both so any in-flight user
	// callback completes before teardown.
	dispatcherDone chan struct{}
	emitterDone    chan struct{}
}

// Compile-time assertion that *Pubsub satisfies the pubsub.Pubsub interface.
var _ pubsub.Pubsub = (*Pubsub)(nil)

// newPubsub allocates a *Pubsub with initialized maps and cancel ctx.
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
// limits applied at Subscribe time. When the caller leaves
// PendingLimits fully zero, we default to {Msgs: -1, Bytes: 512 MiB}.
func defaultPendingLimits(in PendingLimits) PendingLimits {
	if in.Msgs == 0 && in.Bytes == 0 {
		return PendingLimits{Msgs: -1, Bytes: 512 * 1024 * 1024}
	}
	return in
}

// buildConnHandlers returns the connHandlers stack installed on every
// owned connection. Handlers close over p so slow-consumer routing
// keeps working.
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

// publishConnCount returns the effective publisher pool size; zero or
// negative means 1.
func publishConnCount(opts Options) int {
	if opts.PublishConns <= 0 {
		return 1
	}
	return opts.PublishConns
}

// subscribeConnCount returns the effective subscriber pool size; zero
// or negative means 1.
func subscribeConnCount(opts Options) int {
	if opts.SubscribeConns <= 0 {
		return 1
	}
	return opts.SubscribeConns
}

// New creates an embedded NATS Pubsub. The returned *Pubsub owns the
// embedded server and the publisher and subscriber connection pools.
// Close shuts down all owned resources.
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
		// Suffix names when the pool has more than one entry so server
		// logs can distinguish connections.
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

// pickConn returns the connection assigned to subject. Selection uses
// a stable FNV-1a hash so same-subject traffic always targets the same
// connection within a process; pools are immutable after construction
// so the lookup is lock-free.
func pickConn(pool []*natsgo.Conn, subject string) *natsgo.Conn {
	if len(pool) == 1 {
		return pool[0]
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(subject))
	n := uint32(len(pool)) //nolint:gosec // pool size bounded by Options.{Publish,Subscribe}Conns
	return pool[h.Sum32()%n]
}

// Publish publishes a message under the given event name. The
// publisher connection is selected by a stable hash of the subject so
// same-subject publishes preserve per-subject ordering.
func (p *Pubsub) Publish(event string, message []byte) error {
	if p.ctx.Err() != nil {
		return xerrors.New("nats pubsub: closed")
	}

	if err := pickConn(p.publishPool, event).Publish(event, message); err != nil {
		return xerrors.Errorf("publish: %w", err)
	}
	return nil
}

// Flush blocks until every publisher connection has flushed buffered
// publishes to the embedded server. Returns the first error
// encountered; remaining connections are still flushed.
func (p *Pubsub) Flush() error {
	if p.ctx.Err() != nil {
		return xerrors.New("nats pubsub: closed")
	}

	var firstErr error
	for i, nc := range p.publishPool {
		if err := nc.Flush(); err != nil && firstErr == nil {
			firstErr = xerrors.Errorf("flush pub conn %d: %w", i, err)
		}
	}
	return firstErr
}

// Subscribe subscribes a Listener to the given event name. Errors
// such as ErrDroppedMessages are silently ignored, mirroring the
// legacy pubsub Listener semantics.
func (p *Pubsub) Subscribe(event string, listener pubsub.Listener) (cancel func(), err error) {
	return p.SubscribeWithErr(event, func(ctx context.Context, msg []byte, err error) {
		if err != nil {
			return
		}
		listener(ctx, msg)
	})
}

// SubscribeWithErr subscribes a ListenerWithErr to the given event
// name. The listener also receives error deliveries such as
// pubsub.ErrDroppedMessages. Multiple local subscribers on the same
// event share a single underlying *natsgo.Subscription with
// per-listener bounded inboxes so a slow listener cannot block its
// peers.
func (p *Pubsub) SubscribeWithErr(event string, listener pubsub.ListenerWithErr) (cancel func(), err error) {
	if p.ctx.Err() != nil {
		return nil, xerrors.New("nats pubsub: closed")
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

	// Start per-listener goroutines before attachListener registers s
	// so a concurrent Close that snapshots s will find live goroutines
	// ready to observe close(s.stop) and exit.
	go s.dispatch()
	go s.emitDrops()

	stopGoroutines := func() {
		s.cancelOnce.Do(func() {
			close(s.stop)
			<-s.dispatcherDone
			<-s.emitterDone
		})
	}

	shared, _, err := p.attachListener(event, s)
	if err != nil {
		stopGoroutines()
		return nil, err
	}
	s.shared = shared
	s.sub = shared.sub

	// Final guard against Close racing after attachListener returns
	// success. The cancelOnce interlock with Close keeps this cleanup
	// safe if Close already cleaned us up.
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
			// was the last listener (otherwise nil). The shared NATS
			// callback may still try a non-blocking send to s.queue
			// concurrently; offerData's select on s.stop drops in
			// that case.
			toDrain := p.detachListener(s)
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

// listenerQueueSize returns the per-listener inbox capacity. A
// positive PendingLimits.Msgs sets the cap (giving callers a knob to
// trigger local-overflow drops since coalescing makes NATS-level
// slow-consumer signals rare). Otherwise the default is used.
func listenerQueueSize(in PendingLimits) int {
	if in.Msgs > 0 {
		return in.Msgs
	}
	return defaultListenerQueueSize
}

const defaultListenerQueueSize = 1024

// attachListener attaches s to the sharedSub for subject and blocks
// until the shared subscription is ready or has deterministically
// failed. The first attacher becomes the creator and drives NATS
// Subscribe / Flush / SetPendingLimits outside p.mu, then publishes
// the result by closing shared.ready. Joiners wait on shared.ready
// (with a p.ctx.Done() escape) so a Publish issued immediately after
// SubscribeWithErr returns cannot race ahead of registration.
//
// On any error path attachListener has already detached s and (for
// the creator) cleaned up registry / NATS state.
//
// The returned bool reports whether this call created the shared
// subscription; it is informational only.
func (p *Pubsub) attachListener(subject string, s *subscription) (*sharedSub, bool, error) {
	p.mu.Lock()
	// Close sets p.ctx.Err() before acquiring p.mu, so any registration
	// past this point is guaranteed to be visible to Close's snapshot
	// of p.subs.
	if p.ctx.Err() != nil {
		p.mu.Unlock()
		return nil, false, xerrors.New("nats pubsub: closed")
	}
	if shared, ok := p.sharedBySubject[subject]; ok {
		// Joiner: register before unlocking so the creator's fan-out
		// and Close both see this listener.
		shared.listeners[s] = struct{}{}
		s.shared = shared
		p.subs[s] = struct{}{}
		p.eventCounts[s.event]++
		p.mu.Unlock()

		select {
		case <-shared.ready:
		case <-p.ctx.Done():
			p.detachListener(s)
			return nil, false, xerrors.New("nats pubsub: closed")
		}
		if shared.readyErr != nil {
			// Creator already cleaned up registry / NATS state.
			p.detachListener(s)
			return nil, false, xerrors.Errorf("shared subscription init: %w", shared.readyErr)
		}
		return shared, false, nil
	}

	// Creator: insert a placeholder so joiners find it and wait.
	// Network I/O below runs lock-free.
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

	// finishCreator publishes the init result to joiners and tears
	// down inserted state on failure. Invoked exactly once per
	// creator-path return.
	finishCreator := func(initErr error) (*sharedSub, bool, error) {
		if initErr == nil {
			close(shared.ready)
			return shared, true, nil
		}

		// Force-remove the shared entry so future Subscribes start
		// fresh and Close's snapshot does not see a phantom.
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
		// Unsubscribe the underlying NATS sub we created; the caller
		// is about to return an error so we can't rely on Close to
		// drain the conn for us.
		if natsSub != nil {
			_ = natsSub.Unsubscribe()
		}
		// Close ready last so joiners observe a consistent state.
		close(shared.ready)
		return nil, false, initErr
	}

	subConn := pickConn(p.subscribePool, subject)
	natsSub, err := subConn.Subscribe(subject, shared.makeCallback(p))
	if err != nil {
		return finishCreator(xerrors.Errorf("subscribe: %w", err))
	}

	// Publish shared.sub and sharedByNATS so the natsSub is observable
	// from Close and async error routing. shared.sub remains set even
	// after init failure so debug paths and tests can inspect it.
	p.mu.Lock()
	shared.sub = natsSub
	p.sharedByNATS[natsSub] = shared
	p.mu.Unlock()

	if hook := p.testHookBeforeFlush; hook != nil {
		hook(subject)
	}

	// Flush the SUB to the server so a publish issued immediately
	// after Subscribe returns cannot race ahead of registration. Flush
	// the conn that owns natsSub, not an arbitrary pool entry.
	if err := subConn.Flush(); err != nil {
		return finishCreator(xerrors.Errorf("flush subscribe: %w", err))
	}
	limits := defaultPendingLimits(p.opts.PendingLimits)
	if err := natsSub.SetPendingLimits(limits.Msgs, limits.Bytes); err != nil {
		return finishCreator(xerrors.Errorf("set pending limits: %w", err))
	}
	return finishCreator(nil)
}

// detachListener removes s from its shared subscription and from the
// Pubsub-wide tracking maps. When s was the last listener, the shared
// entry is removed and returned so the caller can drain outside p.mu;
// otherwise returns nil. Safe to call multiple times.
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
	// Last listener: remove the shared entry so a new Subscribe to
	// this subject creates a fresh underlying subscription. Identity-
	// check deletes because a parallel creator-failure path may have
	// already replaced this entry.
	if cur, ok := p.sharedBySubject[shared.subject]; ok && cur == shared {
		delete(p.sharedBySubject, shared.subject)
	}
	if shared.sub != nil {
		if cur, ok := p.sharedByNATS[shared.sub]; ok && cur == shared {
			delete(p.sharedByNATS, shared.sub)
		}
	}
	p.mu.Unlock()
	// Caller should not try to drain a sub that was never published.
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

// makeCallback returns the NATS message handler for the shared
// subscription. It snapshots the listener set under p.mu, then
// non-blocking-enqueues to each listener so one slow listener cannot
// stall the NATS delivery goroutine.
//
// Zero-copy fan-out: msg.Data is delivered to every local listener
// without cloning. Listeners on a coalesced subject MUST treat the
// delivered bytes as immutable; the slice is owned by nats.go's
// per-conn read buffer and is reused for the next message.
func (ss *sharedSub) makeCallback(p *Pubsub) natsgo.MsgHandler {
	return func(msg *natsgo.Msg) {
		p.mu.Lock()
		listeners := make([]*subscription, 0, len(ss.listeners))
		for s := range ss.listeners {
			listeners = append(listeners, s)
		}
		p.mu.Unlock()
		for _, s := range listeners {
			s.offerData(msg.Data)
		}
	}
}

// offerData non-blockingly enqueues data onto s.queue. On overflow it
// drops the message and raises a drop signal so the emitter surfaces
// pubsub.ErrDroppedMessages independent of dispatcher progress. If s
// is canceled the message is silently dropped.
func (s *subscription) offerData(data []byte) {
	select {
	case s.queue <- data:
	case <-s.stop:
	default:
		s.signalDrop()
	}
}

// signalDrop pushes onto dropSignal without blocking. Multiple drops
// between emitter dequeues coalesce into a single pending signal, so
// the listener observes one ErrDroppedMessages per drop wave.
func (s *subscription) signalDrop() {
	select {
	case s.dropSignal <- struct{}{}:
	default:
	}
}

// dispatch is the per-listener data delivery goroutine. It serializes
// data callbacks while the emitter goroutine delivers drops, so a slow
// data callback cannot block drop notifications.
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
// concurrently with dispatch so a blocked data callback cannot
// suppress drop signaling.
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
// errors trigger drop accounting.
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

// handleSharedSlowConsumer broadcasts pubsub.ErrDroppedMessages to
// every local listener on shared when NATS reports a new drop delta.
// The slow-consumer signal is per-subscription and cannot be narrowed
// to a single local listener.
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
		// Flush / Subscribe calls bail before touching the pools.
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

		// Unsubscribe shared subscriptions; subConn drains below
		// handle the rest. ss.sub may be nil if a creator is still
		// mid-init.
		for _, ss := range shareds {
			if ss.sub != nil {
				_ = ss.sub.Unsubscribe()
			}
		}

		// Stop per-listener goroutines and wait for in-flight user
		// callbacks. Done directly on the handles (not via cancelFn)
		// so cancel paths don't also try to drain shared subscriptions.
		for _, s := range subs {
			s.cancelOnce.Do(func() {
				close(s.stop)
				<-s.dispatcherDone
				<-s.emitterDone
			})
		}

		// Clear tracking maps so post-Close inspection sees no
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

		// Drain subscriber connections first so in-flight deliveries
		// reach listeners, then publisher connections.
		for i, nc := range p.subscribePool {
			if nc == nil {
				continue
			}
			if err := drainConn(nc, drainTimeout); err != nil {
				errs = append(errs, xerrors.Errorf("drain sub conn %d: %w", i, err))
			}
		}
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
