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

	mu          sync.Mutex
	closed      bool
	subs        map[*subscription]struct{}
	subsByNATS  map[*natsgo.Subscription]*subscription
	eventCounts map[string]int
	closeOnce   sync.Once

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

type subscription struct {
	sub        *natsgo.Subscription
	cancelOnce sync.Once

	event    string
	listener pubsub.ListenerWithErr

	dropMu      sync.Mutex
	lastDropped uint64
}

// Compile-time assertion that *Pubsub satisfies the pubsub.Pubsub interface.
var _ pubsub.Pubsub = (*Pubsub)(nil)

// newPubsub allocates a *Pubsub with maps initialized.
func newPubsub(logger slog.Logger, opts Options) *Pubsub {
	return &Pubsub{
		logger:      logger,
		opts:        opts,
		subs:        make(map[*subscription]struct{}),
		subsByNATS:  make(map[*natsgo.Subscription]*subscription),
		eventCounts: make(map[string]int),
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
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return xerrors.New("nats pubsub: closed")
	}
	p.mu.Unlock()

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
	if p.closed {
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
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return xerrors.New("nats pubsub: closed")
	}
	p.mu.Unlock()

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
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return xerrors.New("nats pubsub: closed")
	}
	p.mu.Unlock()

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
func (p *Pubsub) SubscribeWithErr(event string, listener pubsub.ListenerWithErr) (cancel func(), err error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, xerrors.New("nats pubsub: closed")
	}
	p.mu.Unlock()

	subj, err := LegacyEventSubject(event)
	if err != nil {
		return nil, xerrors.Errorf("map event %q: %w", event, err)
	}

	s := &subscription{
		event:    event,
		listener: listener,
	}

	// Use async Subscribe: nats.go spawns one delivery goroutine per
	// *Subscription and invokes the callback for each message. The
	// callback closes over s so we can route via subsByNATS for
	// slow-consumer error accounting.
	natsSub, err := p.subConn.Subscribe(string(subj), func(msg *natsgo.Msg) {
		s.listener(context.Background(), msg.Data, nil)
	})
	if err != nil {
		return nil, xerrors.Errorf("subscribe: %w", err)
	}
	// Flush so the SUB protocol message has actually reached the server
	// before we return; otherwise a Publish issued immediately after
	// Subscribe on pubConn could race ahead of subscription registration.
	if err := p.subConn.Flush(); err != nil {
		_ = natsSub.Unsubscribe()
		return nil, xerrors.Errorf("flush subscribe: %w", err)
	}
	limits := defaultPendingLimits(p.opts.PendingLimits)
	if err := natsSub.SetPendingLimits(limits.Msgs, limits.Bytes); err != nil {
		_ = natsSub.Unsubscribe()
		return nil, xerrors.Errorf("set pending limits: %w", err)
	}
	s.sub = natsSub

	p.mu.Lock()
	p.subs[s] = struct{}{}
	p.subsByNATS[natsSub] = s
	p.eventCounts[event]++
	p.mu.Unlock()

	cancelFn := func() {
		s.cancelOnce.Do(func() {
			// Drain so in-flight delivery completes; fall back to
			// Unsubscribe if drain doesn't return promptly.
			drainTimeout := p.opts.DrainTimeout
			if drainTimeout <= 0 {
				drainTimeout = 5 * time.Second
			}
			done := make(chan error, 1)
			go func() { done <- s.sub.Drain() }()
			select {
			case <-done:
			case <-time.After(drainTimeout):
				_ = s.sub.Unsubscribe()
			}
			p.unregisterSubscription(s)
		})
	}
	return cancelFn, nil
}

// unregisterSubscription removes s from all tracking maps. Safe to call
// multiple times only if guarded externally; callers use cancelOnce.
func (p *Pubsub) unregisterSubscription(s *subscription) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.subs, s)
	delete(p.subsByNATS, s.sub)
	if c, ok := p.eventCounts[s.event]; ok {
		if c <= 1 {
			delete(p.eventCounts, s.event)
		} else {
			p.eventCounts[s.event] = c - 1
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
	s, ok := p.subsByNATS[sub]
	p.mu.Unlock()
	if !ok {
		return
	}
	p.handleSlowConsumer(s)
}

// handleSlowConsumer is invoked for async slow-consumer signals on s.
// It queries NATS for the cumulative dropped count and emits at most
// one ErrDroppedMessages callback per delta.
func (p *Pubsub) handleSlowConsumer(s *subscription) {
	s.dropMu.Lock()
	defer s.dropMu.Unlock()

	dropped, err := s.sub.Dropped()
	if err != nil {
		p.logger.Warn(context.Background(), "nats: query dropped count", slog.Error(err))
		return
	}
	if dropped < 0 {
		p.logger.Warn(context.Background(), "nats: negative dropped count")
		return
	}
	cur := uint64(dropped)
	if cur < s.lastDropped {
		s.lastDropped = cur
		return
	}
	delta := cur - s.lastDropped
	if delta == 0 {
		return
	}
	s.lastDropped = cur
	s.listener(context.Background(), nil, pubsub.ErrDroppedMessages)
}

// Close drains and shuts down the Pubsub. It is idempotent.
func (p *Pubsub) Close() error {
	var errs []error
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		subs := make([]*subscription, 0, len(p.subs))
		for s := range p.subs {
			subs = append(subs, s)
		}
		p.mu.Unlock()

		// Unsubscribe each subscription. Don't drain individually here;
		// we drain subConn as a whole below.
		for _, s := range subs {
			s.cancelOnce.Do(func() {
				_ = s.sub.Unsubscribe()
				p.unregisterSubscription(s)
			})
		}

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
