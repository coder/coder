package batchstats

import (
	"context"
	"os"
	"sync"
	"time"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk/agentsdk"
)

const (
	// DefaultBatchInterval is the default interval at which the batcher
	// flushes its buffer.
	DefaultBatchInterval = 1 * time.Second
	// DefaultBatchSize is the default size of the batcher's buffer.
	DefaultBatchSize = 1024
)

// Batcher holds a buffer of agent stats and periodically flushes them to
// its configured store. It also updates the workspace's last used time.
type Batcher struct {
	store database.Store
	log   slog.Logger

	mu  sync.RWMutex
	buf []agentsdk.AgentMetric

	ticker <-chan time.Time
}

// Option is a functional option for configuring a Batcher.
type Option func(b *Batcher)

// WithStore sets the store to use for storing stats.
func WithStore(store database.Store) Option {
	return func(b *Batcher) {
		b.store = store
	}
}

// WithBatchSize sets the number of stats to store in a batch.
func WithBatchSize(size int) Option {
	return func(b *Batcher) {
		b.buf = make([]agentsdk.AgentMetric, 0, size)
	}
}

// WithTicker sets the ticker to use for batching stats.
func WithTicker(ticker <-chan time.Time) Option {
	return func(b *Batcher) {
		b.ticker = ticker
	}
}

// WithLogger sets the logger to use for logging.
func WithLogger(log slog.Logger) Option {
	return func(b *Batcher) {
		b.log = log
	}
}

// New creates a new Batcher.
func New(opts ...Option) (*Batcher, func()) {
	b := &Batcher{}
	closer := func() {}
	buf := make([]agentsdk.AgentMetric, 0, DefaultBatchSize)
	b.buf = buf
	b.log = slog.Make(sloghuman.Sink(os.Stderr))
	for _, opt := range opts {
		opt(b)
	}

	if b.store == nil {
		panic("batcher needs a store")
	}

	if b.ticker == nil {
		t := time.NewTicker(DefaultBatchInterval)
		b.ticker = t.C
		closer = t.Stop
	}

	return b, closer
}

// Add adds a stat to the batcher.
func (b *Batcher) Add(m agentsdk.AgentMetric) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.buf = append(b.buf, m)
}

// Run runs the batcher.
func (b *Batcher) Run(ctx context.Context) {
	for {
		select {
		case tick, ok := <-b.ticker:
			if !ok {
				b.log.Warn(ctx, "ticker closed, flushing batcher")
				b.flush(ctx, time.Now())
				return
			}
			b.flush(ctx, tick)
		case <-ctx.Done():
			b.log.Warn(ctx, "context done, flushing batcher")
			b.flush(ctx, time.Now())
			return
		}
	}
}

// flush flushes the batcher's buffer.
func (b *Batcher) flush(ctx context.Context, now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.buf) == 0 {
		return
	}

	b.log.Debug(context.Background(), "flushing batcher", slog.F("count", len(b.buf)))
	// TODO(cian): update the query to batch-insert multiple stats
	for range b.buf {
		if _, err := b.store.InsertWorkspaceAgentStat(ctx, database.InsertWorkspaceAgentStatParams{
			// TODO: fill
		}); err != nil {
			b.log.Error(context.Background(), "insert workspace agent stat", slog.Error(err))
		}
	}
}
