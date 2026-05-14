package nats

import (
	"context"
	"errors"
	"net/url"
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
// Connection model: when constructed via New, Pubsub owns one embedded
// server and two TCP-loopback *natsgo.Conns dialed at the server's
// client listener: pubConn for all publishes and subConn for all
// subscriptions. Subscriptions multiplex over the single subConn and
// rely on client-side PendingLimits for per-subscription slow-consumer
// isolation. NewFromConn is the exception: a single caller-supplied
// connection is used for both publish and subscribe.
type Pubsub struct {
	logger slog.Logger
	opts   Options

	ns *natsserver.Server
	// pubConn carries all publishes. In the NewFromConn path it doubles
	// as the subscribe connection.
	pubConn *natsgo.Conn
	// subConn carries every subscription created via Subscribe /
	// SubscribeWithErr. Nil in the NewFromConn path (subscribes share
	// pubConn there).
	subConn *natsgo.Conn

	ownsServer  bool
	ownsPubConn bool
	// ownsSubConn is true when the wrapper opened its own subConn. False
	// for NewFromConn, which reuses the external connection for subs.
	ownsSubConn bool

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
	// SubscribeWithErr, RefreshPeers) without taking p.mu. Close cancels
	// it before acquiring p.mu so racing callers bail before touching
	// the underlying *natsgo.Conn.
	ctx    context.Context
	cancel context.CancelFunc

	// provider is captured at construction time so RefreshPeers can
	// re-query peer membership at runtime. Nil for NewFromConn or for
	// New called without a PeerProvider.
	provider PeerProvider

	// serverOpts is the effective startup *natsserver.Options. It is
	// cloned on every successful refresh so the next refresh starts
	// from the most recent reloaded state.
	serverOpts *natsserver.Options

	// refreshMu serializes RefreshPeers calls so a slow provider or
	// ReloadOptions cannot interleave.
	refreshMu sync.Mutex

	// currentRoutes is the sorted set of route URLs most recently
	// applied to the embedded server. Compared in RefreshPeers to
	// detect no-op refreshes.
	currentRoutes []*url.URL

	// effectiveClusterToken is the cluster token that was actually
	// applied to the embedded server. It mirrors opts.ClusterToken when
	// the caller supplied one and otherwise holds an internally
	// generated ephemeral token. RefreshPeers uses this to construct
	// route URLs so that auto-generated tokens stay consistent across
	// refreshes.
	effectiveClusterToken string
}

// sharedSub coalesces local subscribers on the same NATS subject onto a
// single *natsgo.Subscription. The first local subscriber for a subject
// creates the underlying subscription; later subscribers attach to it.
// When the last local subscriber detaches, the underlying subscription
// is drained / unsubscribed.
//
// All mutable fields below (listeners, lastDropped) are protected by
// the parent Pubsub.mu, except dropMu / lastDropped which use their own
// mutex so the async error callback can update drop accounting without
// taking the parent Pubsub.mu.
type sharedSub struct {
	// subject is the full NATS subject this shared subscription is
	// registered against.
	subject string
	// sub is the underlying *natsgo.Subscription. Lifecycle is tied to
	// listeners: created on the first attach, drained/unsubscribed
	// when the last listener detaches.
	sub *natsgo.Subscription
	// listeners is the set of local listeners attached to this shared
	// subscription. Guarded by p.mu.
	listeners map[*subscription]struct{}

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

// New creates a new embedded NATS Pubsub. The returned *Pubsub owns the
// embedded server, one TCP-loopback publisher connection, and one
// TCP-loopback subscriber connection. All subscriptions multiplex over
// the subscriber connection. Close shuts down all owned resources.
func New(ctx context.Context, logger slog.Logger, opts Options) (*Pubsub, error) {
	var peers []Peer
	if opts.PeerProvider != nil {
		raw, err := opts.PeerProvider.Peers(ctx)
		if err != nil {
			return nil, xerrors.Errorf("nats peer discovery: %w", err)
		}
		normalized, err := normalizePeers(raw)
		if err != nil {
			return nil, xerrors.Errorf("nats peer normalize: %w", err)
		}
		peers = normalized
	}
	ns, sopts, token, err := startEmbeddedServer(logger, opts, peers)
	if err != nil {
		return nil, err
	}

	p := newPubsub(logger, opts)
	p.ns = ns
	p.ownsServer = true
	p.ownsPubConn = true
	p.ownsSubConn = true
	p.provider = opts.PeerProvider
	p.serverOpts = sopts
	p.currentRoutes = sortRouteURLs(cloneRouteURLs(sopts.Routes))
	p.effectiveClusterToken = token

	pubConn, err := connectClient(ns, opts, p.buildConnHandlers(), "coder-pubsub-pub")
	if err != nil {
		ns.Shutdown()
		ns.WaitForShutdown()
		return nil, xerrors.Errorf("dial pub conn: %w", err)
	}
	subConn, err := connectClient(ns, opts, p.buildConnHandlers(), "coder-pubsub-sub")
	if err != nil {
		pubConn.Close()
		ns.Shutdown()
		ns.WaitForShutdown()
		return nil, xerrors.Errorf("dial sub conn: %w", err)
	}
	p.pubConn = pubConn
	p.subConn = subConn
	return p, nil
}

// NewFromConn wraps an externally provided *natsgo.Conn. The returned
// *Pubsub does not own the connection; Close cancels package-owned
// subscriptions but does not drain or close the connection or any server.
//
// NewFromConn does not get the publish/subscribe split: the supplied
// connection is reused for both publish and subscribe. Callers choosing
// this path own their own connection budgeting.
func NewFromConn(logger slog.Logger, nc *natsgo.Conn) (*Pubsub, error) {
	if nc == nil {
		return nil, xerrors.New("nats: nil connection")
	}
	p := newPubsub(logger, Options{})
	p.pubConn = nc
	// subConn aliases pubConn so Subscribe always uses p.subConn. The
	// ownership flags stay false; Close will not drain or close it.
	p.subConn = nc
	return p, nil
}

// RefreshPeers re-queries the configured PeerProvider and applies any
// route additions or removals to the embedded NATS server via
// ReloadOptions, without restarting the server.
//
// RefreshPeers returns:
//   - ErrNoEmbeddedServer when called on a Pubsub created via
//     NewFromConn (no embedded server to reload).
//   - A configuration error when the Pubsub was created via New
//     without a PeerProvider.
//   - nil when the resulting route set is identical to the
//     currently-applied one (no-op refresh), including the empty-set
//     case for a "cluster of 1".
//
// RefreshPeers is safe to call concurrently with publish/subscribe
// traffic. Concurrent RefreshPeers calls are serialized internally.
func (p *Pubsub) RefreshPeers(ctx context.Context) error {
	if p.ctx.Err() != nil {
		return xerrors.New("nats pubsub: closed")
	}

	if p.ns == nil {
		return ErrNoEmbeddedServer
	}
	if p.provider == nil {
		return xerrors.New("nats pubsub: no PeerProvider configured")
	}

	p.refreshMu.Lock()
	defer p.refreshMu.Unlock()

	raw, err := p.provider.Peers(ctx)
	if err != nil {
		return xerrors.Errorf("nats peer discovery: %w", err)
	}
	normalized, err := normalizePeers(raw)
	if err != nil {
		return xerrors.Errorf("normalize peers: %w", err)
	}
	urls, err := routeURLs(normalized, p.effectiveClusterToken)
	if err != nil {
		return xerrors.Errorf("build route urls: %w", err)
	}

	urls = p.dropSelfRoutes(urls)
	urls = sortRouteURLs(urls)

	if routeURLsEqual(urls, p.currentRoutes) {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ctx.Err() != nil {
		return xerrors.New("nats pubsub: closed")
	}

	newOpts := p.serverOpts.Clone()
	newOpts.Routes = cloneRouteURLs(urls)

	if err := p.ns.ReloadOptions(newOpts); err != nil {
		return xerrors.Errorf("reload nats routes: %w", err)
	}
	p.serverOpts = newOpts
	p.currentRoutes = sortRouteURLs(cloneRouteURLs(urls))
	return nil
}

// dropSelfRoutes filters route URLs whose host matches the server's
// own cluster listener address or configured ClusterAdvertise.
func (p *Pubsub) dropSelfRoutes(in []*url.URL) []*url.URL {
	if len(in) == 0 {
		return in
	}
	selfHosts := make(map[string]struct{}, 2)
	if addr := p.ns.ClusterAddr(); addr != nil {
		selfHosts[addr.String()] = struct{}{}
	}
	if adv := p.opts.ClusterAdvertise; adv != "" {
		selfHosts[adv] = struct{}{}
	}
	if len(selfHosts) == 0 {
		return in
	}
	out := make([]*url.URL, 0, len(in))
	for _, u := range in {
		if u == nil {
			continue
		}
		if _, ok := selfHosts[u.Host]; ok {
			p.logger.Debug(context.Background(), "nats refresh: dropping self route",
				slog.F("host", u.Host),
			)
			continue
		}
		out = append(out, u)
	}
	return out
}

// Publish publishes a message under the given legacy event name.
func (p *Pubsub) Publish(event string, message []byte) error {
	if p.ctx.Err() != nil {
		return xerrors.New("nats pubsub: closed")
	}

	subj, err := LegacyEventSubject(event)
	if err != nil {
		return xerrors.Errorf("map event %q: %w", event, err)
	}
	if err := p.pubConn.Publish(string(subj), message); err != nil {
		return xerrors.Errorf("publish: %w", err)
	}
	return nil
}

// Flush blocks until the publish connection has flushed all buffered
// publishes to the embedded server. Mirrors nats.Conn.Flush. Useful in
// benchmarks and tests where the caller needs to know that all preceding
// Publish calls have reached the server.
func (p *Pubsub) Flush() error {
	if p.ctx.Err() != nil {
		return xerrors.New("nats pubsub: closed")
	}

	if err := p.pubConn.Flush(); err != nil {
		return xerrors.Errorf("flush: %w", err)
	}
	return nil
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

	// attachListener creates the shared *natsgo.Subscription on first
	// attach for this subject and links s to it. On error we return
	// without leaking any registry state or NATS subscription.
	shared, created, err := p.attachListener(string(subj), s)
	if err != nil {
		return nil, err
	}
	s.shared = shared
	s.sub = shared.sub

	if created {
		// First attach for this subject: flush the SUB protocol message
		// to the server before returning so a publish issued
		// immediately after Subscribe cannot race ahead of subscription
		// registration.
		if err := p.subConn.Flush(); err != nil {
			// Undo the attach atomically: removes s from listeners,
			// tears down the shared sub since it was just created.
			p.detachListener(s)
			return nil, xerrors.Errorf("flush subscribe: %w", err)
		}
		limits := defaultPendingLimits(p.opts.PendingLimits)
		if err := shared.sub.SetPendingLimits(limits.Msgs, limits.Bytes); err != nil {
			p.detachListener(s)
			return nil, xerrors.Errorf("set pending limits: %w", err)
		}
	}

	// Start the per-listener goroutines only after attach has succeeded
	// so a failure path above never leaks a goroutine.
	go s.dispatch()
	go s.emitDrops()

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

// attachListener attaches s to the sharedSub for subject. If no shared
// subscription exists yet, it creates the underlying *natsgo.Subscription
// (with the wrapper's shared callback that fans out to every attached
// listener) and registers it. Returns the shared entry, whether this
// call created it, and any error from the NATS subscribe call.
func (p *Pubsub) attachListener(subject string, s *subscription) (*sharedSub, bool, error) {
	p.mu.Lock()
	if shared, ok := p.sharedBySubject[subject]; ok {
		shared.listeners[s] = struct{}{}
		p.subs[s] = struct{}{}
		p.eventCounts[s.event]++
		p.mu.Unlock()
		return shared, false, nil
	}
	// Drop the lock around the actual NATS Subscribe call: it issues a
	// SUB protocol frame on subConn and we must not hold p.mu while
	// doing network I/O. The window between unlock and re-lock could
	// allow a racing Subscribe for the same subject to also miss the
	// map entry and create its own shared. We resolve that race below
	// after re-acquiring the lock.
	p.mu.Unlock()

	shared := &sharedSub{
		subject:   subject,
		listeners: make(map[*subscription]struct{}),
	}
	natsSub, err := p.subConn.Subscribe(subject, shared.makeCallback(p))
	if err != nil {
		return nil, false, xerrors.Errorf("subscribe: %w", err)
	}
	shared.sub = natsSub

	p.mu.Lock()
	if existing, ok := p.sharedBySubject[subject]; ok {
		// Lost the race; tear down our duplicate underlying sub and
		// attach to the winner instead.
		p.mu.Unlock()
		_ = natsSub.Unsubscribe()
		p.mu.Lock()
		existing.listeners[s] = struct{}{}
		p.subs[s] = struct{}{}
		p.eventCounts[s.event]++
		p.mu.Unlock()
		return existing, false, nil
	}
	shared.listeners[s] = struct{}{}
	p.sharedBySubject[subject] = shared
	p.sharedByNATS[natsSub] = shared
	p.subs[s] = struct{}{}
	p.eventCounts[s.event]++
	p.mu.Unlock()
	return shared, true, nil
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
	// subscription rather than attaching to a draining one.
	delete(p.sharedBySubject, shared.subject)
	delete(p.sharedByNATS, shared.sub)
	p.mu.Unlock()
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
		// Flush / Subscribe calls bail before touching pubConn/subConn.
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
		// individually here; we drain subConn as a whole below.
		for _, ss := range shareds {
			_ = ss.sub.Unsubscribe()
		}

		// Stop every per-listener dispatcher goroutine and wait for
		// in-flight user callbacks to complete. We do this on the
		// originating subscription handles (not via cancelFn) so the
		// individual cancel paths do not also try to drain shared
		// subscriptions; the subConn drain below handles flushing
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

		// Drain subConn first so any in-flight deliveries flush to
		// listeners, then close it.
		if p.ownsSubConn && p.subConn != nil {
			if err := drainConn(p.subConn, drainTimeout); err != nil {
				errs = append(errs, xerrors.Errorf("drain subConn: %w", err))
			}
		}
		if p.ownsPubConn && p.pubConn != nil {
			if err := drainConn(p.pubConn, drainTimeout); err != nil {
				errs = append(errs, xerrors.Errorf("drain pubConn: %w", err))
			}
		}

		if p.ownsServer {
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
