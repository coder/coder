package xreplicasync

import (
	"context"
	"errors"
	"hash/fnv"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/nats"
	"github.com/coder/quartz"
)

// ReplicaSource is the subset of replicasync.Manager used by Provider.
// The interface keeps Provider testable without spinning up a real
// replicasync.Manager and database in tests.
type ReplicaSource interface {
	ID() uuid.UUID
	Regional() []database.Replica
	SetCallback(func())
}

// Options configures a Provider.
type Options struct {
	Logger          slog.Logger
	Source          ReplicaSource
	RouteURL        RouteURLFunc
	RefreshFailures prometheus.Counter
	RetryMinBackoff time.Duration
	RetryMaxBackoff time.Duration
	Clock           quartz.Clock
}

// Provider implements nats.PeerProvider on top of a ReplicaSource and
// drives RefreshPeers on a Pubsub when the replica set changes.
type Provider struct {
	logger          slog.Logger
	source          ReplicaSource
	routeURL        RouteURLFunc
	refreshFailures prometheus.Counter
	retryMin        time.Duration
	retryMax        time.Duration
	clock           quartz.Clock

	signal chan struct{}
	done   chan struct{}

	mu      sync.Mutex
	applied uint64
	hasApp  bool
	started bool
	closed  bool
	cancel  context.CancelFunc
}

// pubsubRefresher is the subset of *nats.Pubsub used by the Provider's
// refresh loop. Defining it as an interface lets tests substitute a fake.
type pubsubRefresher interface {
	RefreshPeers(context.Context) error
}

// New constructs a Provider. It does not start the refresh loop; callers
// must invoke Start to begin reacting to replica changes.
func New(opts Options) (*Provider, error) {
	if opts.Source == nil {
		return nil, xerrors.New("xreplicasync: Source is required")
	}
	if opts.RouteURL == nil {
		return nil, xerrors.New("xreplicasync: RouteURL is required")
	}
	retryMin := opts.RetryMinBackoff
	if retryMin == 0 {
		retryMin = time.Second
	}
	retryMax := opts.RetryMaxBackoff
	if retryMax == 0 {
		retryMax = 60 * time.Second
	}
	if retryMin <= 0 {
		return nil, xerrors.Errorf("xreplicasync: RetryMinBackoff must be positive, got %s", retryMin)
	}
	if retryMax < retryMin {
		return nil, xerrors.Errorf("xreplicasync: RetryMaxBackoff %s is less than RetryMinBackoff %s", retryMax, retryMin)
	}
	clk := opts.Clock
	if clk == nil {
		clk = quartz.NewReal()
	}
	return &Provider{
		logger:          opts.Logger,
		source:          opts.Source,
		routeURL:        opts.RouteURL,
		refreshFailures: opts.RefreshFailures,
		retryMin:        retryMin,
		retryMax:        retryMax,
		clock:           clk,
		signal:          make(chan struct{}, 1),
		done:            make(chan struct{}),
	}, nil
}

// Peers implements nats.PeerProvider. It snapshots the current replica
// set and converts each primary, non-self replica into a nats.Peer using
// the configured RouteURLFunc. The ctx parameter is accepted for
// interface compatibility; the snapshot path does not block on it.
func (p *Provider) Peers(_ context.Context) ([]nats.Peer, error) {
	selfID := p.source.ID()
	replicas := p.source.Regional()
	peers := make([]nats.Peer, 0, len(replicas))
	for _, r := range replicas {
		if !r.Primary {
			continue
		}
		if r.ID == selfID {
			continue
		}
		routeURL, err := p.routeURL(r)
		if err != nil {
			return nil, xerrors.Errorf("xreplicasync: route url for replica %s: %w", r.ID, err)
		}
		name := r.Hostname
		if name == "" {
			name = r.ID.String()
		}
		peers = append(peers, nats.Peer{Name: name, RouteURL: routeURL})
	}
	return peers, nil
}

// peerRouteFingerprint returns a stable hash over the peer route URLs.
// The hash is independent of slice order and ignores Name, so peer-name
// changes alone do not trigger refreshes.
func peerRouteFingerprint(peers []nats.Peer) uint64 {
	if len(peers) == 0 {
		return 0
	}
	urls := make([]string, len(peers))
	for i, peer := range peers {
		urls[i] = peer.RouteURL
	}
	sort.Strings(urls)
	h := fnv.New64a()
	for _, u := range urls {
		_, _ = h.Write([]byte(u))
		// Delimiter prevents accidental collisions across boundaries.
		_, _ = h.Write([]byte{0})
	}
	return h.Sum64()
}

// Start launches the refresh loop and registers a callback on the source.
// Start may be called at most once. It returns an error if the Provider
// has already been started or closed, or if pubsub is nil.
func (p *Provider) Start(ctx context.Context, pubsub *nats.Pubsub) error {
	if pubsub == nil {
		return xerrors.New("xreplicasync: pubsub is required")
	}
	return p.startWithRefresher(ctx, pubsub)
}

func (p *Provider) startWithRefresher(ctx context.Context, refresher pubsubRefresher) error {
	if refresher == nil {
		return xerrors.New("xreplicasync: refresher is required")
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return xerrors.New("xreplicasync: provider is closed")
	}
	if p.started {
		p.mu.Unlock()
		return xerrors.New("xreplicasync: provider already started")
	}
	workerCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.started = true
	p.mu.Unlock()

	go p.run(workerCtx, refresher)

	// Register the callback only after lifecycle state is committed so
	// the worker is guaranteed to be live before it can be signaled.
	p.source.SetCallback(func() {
		select {
		case p.signal <- struct{}{}:
		default:
		}
	})
	return nil
}

func (p *Provider) run(ctx context.Context, refresher pubsubRefresher) {
	defer close(p.done)
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.signal:
		}
		p.handleSignal(ctx, refresher)
	}
}

// handleSignal processes a single change notification, retrying on
// transient errors with exponential backoff capped at retryMax.
func (p *Provider) handleSignal(ctx context.Context, refresher pubsubRefresher) {
	delay := p.retryMin
	for {
		if ctx.Err() != nil {
			return
		}
		peers, err := p.Peers(ctx)
		if err != nil {
			p.logger.Error(ctx, "xreplicasync: build peers", slog.Error(err))
			if p.refreshFailures != nil {
				p.refreshFailures.Inc()
			}
			if !p.sleep(ctx, delay) {
				return
			}
			delay = nextDelay(delay, p.retryMax)
			continue
		}
		fp := peerRouteFingerprint(peers)
		p.mu.Lock()
		unchanged := p.hasApp && fp == p.applied
		p.mu.Unlock()
		if unchanged {
			return
		}
		err = refresher.RefreshPeers(ctx)
		if err == nil {
			p.mu.Lock()
			p.applied = fp
			p.hasApp = true
			p.mu.Unlock()
			return
		}
		if errors.Is(err, nats.ErrStandalone) {
			p.logger.Warn(ctx, "xreplicasync: pubsub is standalone, refresh is terminal for this signal", slog.Error(err))
			return
		}
		p.logger.Error(ctx, "xreplicasync: refresh peers", slog.Error(err))
		if p.refreshFailures != nil {
			p.refreshFailures.Inc()
		}
		if !p.sleep(ctx, delay) {
			return
		}
		delay = nextDelay(delay, p.retryMax)
	}
}

func nextDelay(cur, maxDelay time.Duration) time.Duration {
	next := cur * 2
	if next > maxDelay {
		return maxDelay
	}
	return next
}

// sleep waits for the given duration on the configured clock or returns
// false if the context is canceled first.
func (p *Provider) sleep(ctx context.Context, d time.Duration) bool {
	timer := p.clock.NewTimer(d, "xreplicasync", "refresh")
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// Close stops the refresh loop and waits for the worker goroutine to
// exit. Close is idempotent and safe to call before Start.
func (p *Provider) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	started := p.started
	cancel := p.cancel
	p.mu.Unlock()

	if !started {
		return nil
	}
	cancel()
	<-p.done
	return nil
}
