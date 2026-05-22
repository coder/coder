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

// DefaultMaxPending is the per-client outbound pending byte budget
// (1 GiB), raised from the nats-server default of 64 MiB so wide
// local fan-out does not trip the slow-consumer threshold.
const DefaultMaxPending int64 = 1 << 30

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
	// MaxPayload is the NATS max payload. Zero means server default.
	MaxPayload int32

	// MaxPending is the per-client outbound pending byte budget on the
	// embedded server. Zero means DefaultMaxPending; negative means use
	// the nats-server default (64 MiB).
	MaxPending int64

	// DrainTimeout bounds connection drains in Close. Zero means 30
	// seconds, matching the NATS Go client default.
	DrainTimeout time.Duration

	// PendingLimits configures per-subscription NATS pending limits.
	// If both fields are zero, New defaults to {Msgs: -1, Bytes: 512 MiB}.
	PendingLimits PendingLimits

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
	// subscriptions coalesces concurrent local subscribers on the
	// same subject onto a single underlying *natsgo.Subscription.
	subscriptions map[string]*natsSub
	closeOnce     sync.Once

	// ctx is canceled by Close while holding p.mu so subscriber state
	// cleanup observes the canceled context.
	ctx    context.Context
	cancel context.CancelFunc
}

// natsSub maps to one underlying *natsgo.Subscription. The first
// local subscriber creates it; later local subscribers attach to it.
// When the last local subscriber detaches, the NATS subscription is
// unsubscribed.
//
// listeners and sub are guarded by the parent Pubsub.mu. dropMu /
// lastDropped use their own mutex so the async error callback can
// update drop accounting without taking p.mu.
type natsSub struct {
	subject   string
	sub       *natsgo.Subscription
	listeners map[*localSub]struct{}

	dropMu      sync.Mutex
	lastDropped uint64
}

// localSub is the local handle returned by Subscribe /
// SubscribeWithErr. Each local subscriber gets its own bounded inbox
// and dispatcher goroutine so one slow listener cannot block peers on
// the same subject.
type localSub struct {
	cancelOnce sync.Once

	pubsub *Pubsub

	event    string
	listener pubsub.ListenerWithErr

	// shared is the per-subject coalescing entry. Never nil after a
	// successful Subscribe.
	shared *natsSub

	// queue is the per-listener data fan-out inbox. The shared NATS
	// callback enqueues non-blockingly; on overflow the message is
	// dropped and a drop signal is raised.
	queue chan []byte
	// dropSignal is a size-1 buffered channel that coalesces drop
	// notifications from local overflow and NATS slow-consumer
	// broadcasts onto a single pending wake.
	dropSignal chan struct{}
	// stop is closed by close to signal the dispatcher goroutine to exit.
	stop chan struct{}
	// dispatcherDone is closed by the dispatcher goroutine on exit;
	// close waits on it so any in-flight user callback completes
	// before teardown.
	dispatcherDone chan struct{}
}

// Compile-time assertion that *Pubsub satisfies the pubsub.Pubsub interface.
var _ pubsub.Pubsub = (*Pubsub)(nil)

// newPubsub allocates a *Pubsub with initialized maps and cancel ctx.
func newPubsub(ctx context.Context, logger slog.Logger, opts Options) *Pubsub {
	ctx, cancel := context.WithCancel(ctx)
	return &Pubsub{
		logger:        logger,
		opts:          opts,
		subscriptions: make(map[string]*natsSub),
		ctx:           ctx,
		cancel:        cancel,
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
				p.logger.Warn(p.ctx, "nats client disconnected", slog.Error(err))
			}
		},
		reconnect: func(_ *natsgo.Conn) {
			p.logger.Info(p.ctx, "nats client reconnected")
		},
		closed: func(_ *natsgo.Conn) {
			p.logger.Debug(p.ctx, "nats client closed")
		},
		errH: func(_ *natsgo.Conn, sub *natsgo.Subscription, err error) {
			if err != nil && errors.Is(err, natsgo.ErrSlowConsumer) {
				p.handleAsyncError(sub, err)
				return
			}
			if err != nil {
				p.logger.Warn(p.ctx, "nats async error", slog.Error(err))
			}
		},
	}
}

// New creates an embedded NATS Pubsub. The returned *Pubsub owns the
// embedded server and the publisher and subscriber connection pools.
// Close shuts down all owned resources.
func New(ctx context.Context, logger slog.Logger, opts Options) (*Pubsub, error) {
	ns, err := startEmbeddedServer(logger, opts)
	if err != nil {
		return nil, err
	}

	p := newPubsub(ctx, logger, opts)
	p.ns = ns

	npub := opts.PublishConns
	if npub <= 0 {
		npub = 1
	}
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
			p.cancel()
			for _, c := range publishPool {
				c.Close()
			}
			ns.Shutdown()
			ns.WaitForShutdown()
			return nil, xerrors.Errorf("dial pub conn %d: %w", i, err)
		}
		publishPool = append(publishPool, nc)
	}
	nsub := opts.SubscribeConns
	if nsub <= 0 {
		nsub = 1
	}
	subscribePool := make([]*natsgo.Conn, 0, nsub)
	for i := 0; i < nsub; i++ {
		name := "coder-pubsub-sub"
		if nsub > 1 {
			name = fmt.Sprintf("coder-pubsub-sub-%d", i)
		}
		nc, err := connectClient(ns, opts, p.buildConnHandlers(), name)
		if err != nil {
			p.cancel()
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

	s := &localSub{
		pubsub:         p,
		event:          event,
		listener:       listener,
		queue:          make(chan []byte, listenerQueueSize(p.opts.PendingLimits)),
		dropSignal:     make(chan struct{}, 1),
		stop:           make(chan struct{}),
		dispatcherDone: make(chan struct{}),
	}

	// Start the per-listener goroutine before addSubscriber registers s
	// so a concurrent Close that snapshots s will find a live goroutine
	// ready to observe close(s.stop) and exit.
	s.init()

	nsub, err := p.addSubscriber(event, s)
	if err != nil {
		s.close()
		return nil, err
	}
	s.shared = nsub

	// Final guard against Close racing after addSubscriber returns
	// success. Cleanup remains safe if Close already stopped s.
	if p.ctx.Err() != nil {
		p.unsubscribeLocal(s)
		s.close()
		return nil, xerrors.New("nats pubsub: closed")
	}

	cancelFn := func() {
		// The shared NATS callback may still try a non-blocking send to
		// s.queue concurrently; offerData's select on s.stop drops in
		// that case.
		p.unsubscribeLocal(s)
		s.close()
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

// addSubscriber attaches s to the natsSub for subject. The first
// local subscriber initializes the underlying NATS subscription while
// holding p.mu, so later subscribers only observe ready natsSub entries.
func (p *Pubsub) addSubscriber(subject string, s *localSub) (*natsSub, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	nsub, ok := p.subscriptions[subject]
	if ok {
		nsub.listeners[s] = struct{}{}
		s.shared = nsub
		return nsub, nil
	}

	nsub = &natsSub{
		subject:   subject,
		listeners: map[*localSub]struct{}{s: {}},
	}
	s.shared = nsub
	p.subscriptions[subject] = nsub

	initErr := func() error {
		subConn := pickConn(p.subscribePool, subject)
		natsSub, err := subConn.Subscribe(subject, nsub.makeCallback(p))
		if err != nil {
			return xerrors.Errorf("subscribe: %w", err)
		}
		nsub.sub = natsSub

		// Flush the SUB to the server so a publish issued immediately
		// after Subscribe returns cannot race ahead of registration.
		if err := subConn.Flush(); err != nil {
			return xerrors.Errorf("flush subscribe: %w", err)
		}
		limits := defaultPendingLimits(p.opts.PendingLimits)
		if err := natsSub.SetPendingLimits(limits.Msgs, limits.Bytes); err != nil {
			return xerrors.Errorf("set pending limits: %w", err)
		}
		return nil
	}()
	if initErr != nil {
		delete(p.subscriptions, subject)
		delete(nsub.listeners, s)
		if nsub.sub != nil {
			_ = nsub.sub.Unsubscribe()
		}
		return nil, initErr
	}
	return nsub, nil
}

// unsubscribeLocal removes s from its natsSub. If s was the last
// listener, it also removes and unsubscribes the underlying NATS
// subscription.
func (p *Pubsub) unsubscribeLocal(s *localSub) {
	p.mu.Lock()
	nsub := s.shared
	if nsub == nil {
		p.mu.Unlock()
		return
	}
	if _, tracked := nsub.listeners[s]; !tracked {
		p.mu.Unlock()
		return
	}
	delete(nsub.listeners, s)
	if len(nsub.listeners) > 0 {
		p.mu.Unlock()
		return
	}
	// Last listener: remove the nsub entry so a new Subscribe to this
	// subject creates a fresh underlying subscription.
	if cur, ok := p.subscriptions[nsub.subject]; ok && cur == nsub {
		delete(p.subscriptions, nsub.subject)
	}
	natsSub := nsub.sub
	p.mu.Unlock()
	if natsSub != nil {
		_ = natsSub.Unsubscribe()
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
func (nsub *natsSub) makeCallback(p *Pubsub) natsgo.MsgHandler {
	return func(msg *natsgo.Msg) {
		p.mu.Lock()
		listeners := make([]*localSub, 0, len(nsub.listeners))
		for s := range nsub.listeners {
			listeners = append(listeners, s)
		}
		p.mu.Unlock()
		for _, s := range listeners {
			s.offerData(msg.Data)
		}
	}
}

// init starts the per-listener delivery goroutine.
func (s *localSub) init() {
	go s.dispatch()
}

// close stops the per-listener goroutine and waits for callbacks to finish.
func (s *localSub) close() {
	s.cancelOnce.Do(func() {
		close(s.stop)
		<-s.dispatcherDone
	})
}

// offerData non-blockingly enqueues data onto s.queue. On overflow it
// drops the message and raises a drop signal so the dispatcher surfaces
// pubsub.ErrDroppedMessages when the current callback completes. If s
// is canceled the message is silently dropped.
func (s *localSub) offerData(data []byte) {
	select {
	case s.queue <- data:
	case <-s.stop:
	default:
		s.signalDrop()
	}
}

// signalDrop pushes onto dropSignal without blocking. Multiple drops
// between dispatcher dequeues coalesce into a single pending signal, so
// the listener observes one ErrDroppedMessages per drop wave.
func (s *localSub) signalDrop() {
	select {
	case s.dropSignal <- struct{}{}:
	default:
	}
}

// dispatch is the per-listener delivery goroutine. It serializes data
// and drop callbacks for the listener so callers do not need to be safe
// for concurrent invocation.
func (s *localSub) dispatch() {
	defer close(s.dispatcherDone)
	for {
		select {
		case <-s.stop:
			return
		case data := <-s.queue:
			s.listener(s.pubsub.ctx, data, nil)
		case <-s.dropSignal:
			s.listener(s.pubsub.ctx, nil, pubsub.ErrDroppedMessages)
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
	var nsub *natsSub
	for _, candidate := range p.subscriptions {
		if candidate.sub == sub {
			nsub = candidate
			break
		}
	}
	p.mu.Unlock()
	if nsub == nil {
		return
	}
	p.handleSlowSubscriber(nsub)
}

// handleSlowSubscriber broadcasts pubsub.ErrDroppedMessages to every
// local listener on nsub when NATS reports a new drop delta. The
// slow-consumer signal is per-subscription and cannot be narrowed to a
// single local listener.
func (p *Pubsub) handleSlowSubscriber(nsub *natsSub) {
	nsub.dropMu.Lock()
	dropped, err := nsub.sub.Dropped()
	if err != nil {
		nsub.dropMu.Unlock()
		p.logger.Warn(p.ctx, "nats: query dropped count", slog.Error(err))
		return
	}
	if dropped < 0 {
		nsub.dropMu.Unlock()
		p.logger.Warn(p.ctx, "nats: negative dropped count")
		return
	}
	cur := uint64(dropped)
	if cur < nsub.lastDropped {
		nsub.lastDropped = cur
		nsub.dropMu.Unlock()
		return
	}
	delta := cur - nsub.lastDropped
	if delta == 0 {
		nsub.dropMu.Unlock()
		return
	}
	nsub.lastDropped = cur
	nsub.dropMu.Unlock()

	// Snapshot the listener set under p.mu so we don't hold the lock
	// while invoking user callbacks via the dispatcher.
	p.mu.Lock()
	listeners := make([]*localSub, 0, len(nsub.listeners))
	for s := range nsub.listeners {
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
		p.mu.Lock()
		// Cancel while holding p.mu so subscriber state cleanup below
		// observes the canceled context.
		p.cancel()
		var subs []*localSub
		shareds := make([]*natsSub, 0, len(p.subscriptions))
		for _, ss := range p.subscriptions {
			shareds = append(shareds, ss)
			for s := range ss.listeners {
				subs = append(subs, s)
			}
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
		// callbacks. Done directly on the handles, not via cancelFn.
		for _, s := range subs {
			s.close()
		}

		// Clear tracking maps so post-Close inspection sees no
		// dangling state.
		p.mu.Lock()
		for _, ss := range shareds {
			for s := range ss.listeners {
				delete(ss.listeners, s)
			}
		}
		for k := range p.subscriptions {
			delete(p.subscriptions, k)
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
