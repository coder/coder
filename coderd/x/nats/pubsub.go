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
type Pubsub struct {
	logger slog.Logger
	opts   Options

	ns *natsserver.Server
	nc *natsgo.Conn

	ownsServer bool
	ownsConn   bool

	mu          sync.Mutex
	closed      bool
	subs        map[*subscription]struct{}
	subsByNATS  map[*natsgo.Subscription]*subscription
	eventCounts map[string]int
	closeOnce   sync.Once

	// closedCh is signaled by the NATS ClosedHandler so Close can wait
	// for Drain to fully complete without polling.
	closedCh chan struct{}

	metrics pubsubMetrics

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
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	cancelOnce sync.Once

	event    string
	listener pubsub.ListenerWithErr

	dropMu      sync.Mutex
	lastDropped uint64
}

// Compile-time assertion that *Pubsub satisfies the pubsub.Pubsub interface.
var _ pubsub.Pubsub = (*Pubsub)(nil)

// newPubsub allocates a *Pubsub with maps and metrics initialized.
func newPubsub(logger slog.Logger, opts Options) *Pubsub {
	return &Pubsub{
		logger:      logger,
		opts:        opts,
		subs:        make(map[*subscription]struct{}),
		subsByNATS:  make(map[*natsgo.Subscription]*subscription),
		eventCounts: make(map[string]int),
		metrics:     newPubsubMetrics(),
	}
}

// New creates a new embedded NATS Pubsub. The returned *Pubsub owns the
// embedded server and client connection and shuts them down on Close.
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

	closedCh := make(chan struct{})
	var closeOnce sync.Once

	p := newPubsub(logger, opts)
	p.ns = ns
	p.ownsServer = true
	p.ownsConn = true
	p.closedCh = closedCh
	p.provider = opts.PeerProvider
	p.serverOpts = sopts
	p.currentRoutes = sortRouteURLs(cloneRouteURLs(sopts.Routes))
	p.effectiveClusterToken = token

	handlers := connHandlers{
		disconnectErr: func(_ *natsgo.Conn, err error) {
			p.metrics.disconnectsTotal.Inc()
			if err != nil {
				logger.Warn(context.Background(), "nats client disconnected", slog.Error(err))
			}
		},
		reconnect: func(_ *natsgo.Conn) {
			p.metrics.reconnectsTotal.Inc()
			logger.Info(context.Background(), "nats client reconnected")
		},
		closed: func(_ *natsgo.Conn) {
			closeOnce.Do(func() { close(closedCh) })
			logger.Debug(context.Background(), "nats client closed")
		},
		errH: func(_ *natsgo.Conn, sub *natsgo.Subscription, err error) {
			if err != nil && errors.Is(err, natsgo.ErrSlowConsumer) {
				p.handleAsyncError(sub, err)
				return
			}
			if err != nil {
				logger.Warn(context.Background(), "nats async error", slog.Error(err))
			}
		},
	}

	nc, err := connectInProcess(ns, opts, handlers)
	if err != nil {
		ns.Shutdown()
		ns.WaitForShutdown()
		return nil, err
	}
	p.nc = nc
	return p, nil
}

// NewFromConn wraps an externally provided *natsgo.Conn. The returned
// *Pubsub does not own the connection; Close cancels package-owned
// subscriptions but does not drain or close the connection or any server.
func NewFromConn(logger slog.Logger, nc *natsgo.Conn) (*Pubsub, error) {
	if nc == nil {
		return nil, xerrors.New("nats: nil connection")
	}
	p := newPubsub(logger, Options{})
	p.nc = nc
	// NewFromConn does not own a server, so refresh has nothing to
	// reload. RefreshPeers returns ErrNoEmbeddedServer.
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

	// Drop any routes pointing back at this server (self routes). NATS
	// already filters self loops at runtime, but eliminating them up
	// front keeps currentRoutes meaningful for comparison.
	urls = p.dropSelfRoutes(urls)
	urls = sortRouteURLs(urls)

	if routeURLsEqual(urls, p.currentRoutes) {
		return nil
	}

	// Take p.mu through ReloadOptions so a concurrent Close cannot
	// shut the server down between our closed check and the reload.
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
		p.metrics.publishesTotal.WithLabelValues("false").Inc()
		return xerrors.New("nats pubsub: closed")
	}
	p.mu.Unlock()

	subj, err := LegacyEventSubject(event)
	if err != nil {
		p.metrics.publishesTotal.WithLabelValues("false").Inc()
		return xerrors.Errorf("map event %q: %w", event, err)
	}
	if err := p.nc.Publish(string(subj), message); err != nil {
		p.metrics.publishesTotal.WithLabelValues("false").Inc()
		return xerrors.Errorf("publish: %w", err)
	}

	p.metrics.publishesTotal.WithLabelValues("true").Inc()
	p.metrics.publishedBytesTotal.Add(float64(len(message)))
	sizeLabel := messageSizeNormal
	if len(message) >= colossalThreshold {
		sizeLabel = messageSizeColossal
	}
	p.metrics.messagesTotal.WithLabelValues(sizeLabel).Inc()
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
		p.metrics.subscribesTotal.WithLabelValues("false").Inc()
		return nil, xerrors.New("nats pubsub: closed")
	}
	p.mu.Unlock()

	subj, err := LegacyEventSubject(event)
	if err != nil {
		p.metrics.subscribesTotal.WithLabelValues("false").Inc()
		return nil, xerrors.Errorf("map event %q: %w", event, err)
	}
	natsSub, err := p.nc.SubscribeSync(string(subj))
	if err != nil {
		p.metrics.subscribesTotal.WithLabelValues("false").Inc()
		return nil, xerrors.Errorf("subscribe: %w", err)
	}
	if p.opts.PendingLimits.Msgs != 0 || p.opts.PendingLimits.Bytes != 0 {
		if err := natsSub.SetPendingLimits(p.opts.PendingLimits.Msgs, p.opts.PendingLimits.Bytes); err != nil {
			_ = natsSub.Unsubscribe()
			p.metrics.subscribesTotal.WithLabelValues("false").Inc()
			return nil, xerrors.Errorf("set pending limits: %w", err)
		}
	}

	ctx, cancelCtx := context.WithCancel(context.Background())
	s := &subscription{
		sub:      natsSub,
		ctx:      ctx,
		cancel:   cancelCtx,
		event:    event,
		listener: listener,
	}

	p.mu.Lock()
	p.subs[s] = struct{}{}
	p.subsByNATS[natsSub] = s
	p.eventCounts[event]++
	p.mu.Unlock()

	s.wg.Add(1)
	go p.runSubscription(s)
	p.metrics.subscribesTotal.WithLabelValues("true").Inc()

	cancelFn := func() {
		s.cancelOnce.Do(func() {
			s.cancel()
			_ = s.sub.Unsubscribe()
			s.wg.Wait()
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

func (p *Pubsub) runSubscription(s *subscription) {
	defer s.wg.Done()
	for {
		msg, err := s.sub.NextMsgWithContext(s.ctx)
		if err != nil {
			switch {
			case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
				return
			case errors.Is(err, natsgo.ErrConnectionClosed),
				errors.Is(err, natsgo.ErrBadSubscription):
				return
			case errors.Is(err, natsgo.ErrSlowConsumer):
				p.handleSlowConsumer(s)
				continue
			default:
				p.logger.Warn(s.ctx, "nats subscription error", slog.Error(err))
				return
			}
		}
		p.metrics.receivedBytesTotal.Add(float64(len(msg.Data)))
		s.listener(s.ctx, msg.Data, nil)
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

// handleSlowConsumer is invoked for both sync (NextMsg) and async slow
// consumer signals on s. It increments slow-consumer metrics, queries
// NATS for the cumulative dropped count, and emits at most one
// ErrDroppedMessages callback per delta.
func (p *Pubsub) handleSlowConsumer(s *subscription) {
	s.dropMu.Lock()
	defer s.dropMu.Unlock()

	p.metrics.slowConsumersTotal.Inc()

	dropped, err := s.sub.Dropped()
	if err != nil {
		p.logger.Warn(s.ctx, "nats: query dropped count", slog.Error(err))
		return
	}
	if dropped < 0 {
		p.logger.Warn(s.ctx, "nats: negative dropped count")
		return
	}
	cur := uint64(dropped)
	if cur < s.lastDropped {
		// Counter went backwards (e.g., subscription replaced); reset
		// baseline without emitting a callback.
		s.lastDropped = cur
		return
	}
	delta := cur - s.lastDropped
	if delta == 0 {
		return
	}
	p.metrics.droppedMsgsTotal.Add(float64(delta))
	s.lastDropped = cur
	s.listener(s.ctx, nil, pubsub.ErrDroppedMessages)
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

		for _, s := range subs {
			s.cancelOnce.Do(func() {
				s.cancel()
				_ = s.sub.Unsubscribe()
				s.wg.Wait()
				p.unregisterSubscription(s)
			})
		}

		if p.ownsConn {
			drainTimeout := p.opts.DrainTimeout
			if drainTimeout <= 0 {
				drainTimeout = 30 * time.Second
			}
			if err := p.nc.Drain(); err != nil {
				p.nc.Close()
				errs = append(errs, xerrors.Errorf("drain: %w", err))
			} else {
				select {
				case <-p.closedCh:
				case <-time.After(drainTimeout):
					p.nc.Close()
					errs = append(errs, xerrors.Errorf("drain timeout after %s", drainTimeout))
				}
			}
		}

		if p.ownsServer {
			p.ns.Shutdown()
			p.ns.WaitForShutdown()
		}
	})
	return errors.Join(errs...)
}
