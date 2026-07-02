package cryptokeys

import (
	"context"
	"io"
	"sync"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/quartz"
)

// NATSCACache caches the active NATS cluster CA and refreshes it in the
// background to track rotation. It is strictly read-only: the key rotator is
// the sole creator of nats_ca rows, so the cache never inserts.
type NATSCACache interface {
	// CA returns the current NATS cluster CA. It returns ErrNATSCANotFound
	// until the rotator has minted the initial CA; callers should retry.
	CA(ctx context.Context) (*NATSCA, error)
	io.Closer
}

type natsCACache struct {
	logger slog.Logger
	db     database.Store
	clock  quartz.Clock

	// ctx is canceled by Close to stop background refreshes.
	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.Mutex
	ca        *NATSCA
	refresher *quartz.Timer
	closed    bool
}

// NATSCACacheOption configures a NATSCACache.
type NATSCACacheOption func(*natsCACache)

// WithNATSCACacheClock overrides the cache clock, for tests.
func WithNATSCACacheClock(clock quartz.Clock) NATSCACacheOption {
	return func(c *natsCACache) {
		c.clock = clock
	}
}

// NewNATSCACache constructs a NATSCACache and performs an initial fetch. A
// failed initial fetch is logged but not fatal: the active CA may not exist
// until the rotator mints it, and the next CA call (or the periodic refresh)
// retries.
func NewNATSCACache(ctx context.Context, logger slog.Logger, db database.Store, opts ...NATSCACacheOption) (NATSCACache, error) {
	c := &natsCACache{
		logger: logger.Named("nats_ca_cache"),
		db:     db,
		clock:  quartz.NewReal(),
	}
	for _, opt := range opts {
		opt(c)
	}

	c.ctx, c.cancel = context.WithCancel(ctx)
	c.refresher = c.clock.AfterFunc(refreshInterval, c.refresh)

	// A missing CA at construction is expected before the rotator mints it, and
	// any transient error self-heals via CA or the periodic refresh, so this is
	// not fatal.
	if ca, err := FetchNATSCA(c.ctx, c.logger, c.db); err != nil {
		c.logger.Warn(c.ctx, "initial NATS CA fetch failed; will retry", slog.Error(err))
	} else {
		c.ca = ca
	}

	return c, nil
}

func (c *natsCACache) CA(ctx context.Context) (*NATSCA, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrClosed
	}
	ca := c.ca
	c.mu.Unlock()

	if ca != nil {
		return ca, nil
	}

	// Not yet populated (initial fetch failed, or the rotator has not minted
	// the CA yet). Fetch on demand so a transient startup gap self-heals.
	fetched, err := FetchNATSCA(ctx, c.logger, c.db)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	if !c.closed {
		c.ca = fetched
	}
	c.mu.Unlock()
	return fetched, nil
}

func (c *natsCACache) refresh() {
	ca, err := FetchNATSCA(c.ctx, c.logger, c.db)
	if err != nil {
		if c.ctx.Err() == nil {
			c.logger.Warn(c.ctx, "failed to refresh NATS CA", slog.Error(err))
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	if err == nil {
		c.ca = ca
	}
	c.refresher.Reset(refreshInterval)
}

func (c *natsCACache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true
	c.cancel()
	c.refresher.Stop()
	return nil
}
