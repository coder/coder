package agentapi

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

const (
	// defaultMetadataBatchSize is the maximum number of agents to batch in a
	// single flush. This provides headroom for growth beyond typical 300
	// agents per flush.
	defaultMetadataBatchSize = 500

	// defaultMetadataFlushInterval is how frequently to flush batched metadata
	// updates to the database and pubsub. 5 seconds provides a good balance
	// between reducing database load and maintaining reasonable UI update
	// latency.
	defaultMetadataFlushInterval = 5 * time.Second
)

// agentMetadataUpdate holds metadata updates for a single agent that are
// pending flush.
type agentMetadataUpdate struct {
	agentID     uuid.UUID
	keys        []string
	values      []string
	errors      []string
	collectedAt []time.Time
}

// MetadataBatcher holds a buffer of agent metadata updates and periodically
// flushes them to the database and pubsub. This reduces database write
// frequency and pubsub publish rate.
type MetadataBatcher struct {
	store  database.Store
	pubsub pubsub.Pubsub
	log    slog.Logger

	mu sync.Mutex
	// buf holds pending metadata updates up to batchSize capacity.
	// When full, new updates are dropped.
	buf       []agentMetadataUpdate
	batchSize int

	// tickCh is used to periodically flush the buffer.
	tickCh   <-chan time.Time
	ticker   *time.Ticker
	interval time.Duration

	// flushLever is used to signal the flusher to flush the buffer immediately.
	flushLever  chan struct{}
	flushForced atomic.Bool

	// flushed is used during testing to signal that a flush has completed.
	flushed chan<- int

	timeNowFn func() time.Time
}

// MetadataBatcherOption is a functional option for configuring a MetadataBatcher.
type MetadataBatcherOption func(b *MetadataBatcher)

// MetadataBatcherWithStore sets the store to use for storing metadata.
func MetadataBatcherWithStore(store database.Store) MetadataBatcherOption {
	return func(b *MetadataBatcher) {
		b.store = store
	}
}

// MetadataBatcherWithPubsub sets the pubsub to use for publishing metadata updates.
func MetadataBatcherWithPubsub(pubsub pubsub.Pubsub) MetadataBatcherOption {
	return func(b *MetadataBatcher) {
		b.pubsub = pubsub
	}
}

// MetadataBatcherWithBatchSize sets the maximum number of agents to batch.
func MetadataBatcherWithBatchSize(size int) MetadataBatcherOption {
	return func(b *MetadataBatcher) {
		b.batchSize = size
	}
}

// MetadataBatcherWithInterval sets the interval for flushes.
func MetadataBatcherWithInterval(d time.Duration) MetadataBatcherOption {
	return func(b *MetadataBatcher) {
		b.interval = d
	}
}

// MetadataBatcherWithLogger sets the logger to use for logging.
func MetadataBatcherWithLogger(log slog.Logger) MetadataBatcherOption {
	return func(b *MetadataBatcher) {
		b.log = log
	}
}

// MetadataBatcherWithTimeNow sets a custom time function for testing.
func MetadataBatcherWithTimeNow(fn func() time.Time) MetadataBatcherOption {
	return func(b *MetadataBatcher) {
		b.timeNowFn = fn
	}
}

// NewMetadataBatcher creates a new MetadataBatcher and starts it.
func NewMetadataBatcher(ctx context.Context, opts ...MetadataBatcherOption) (*MetadataBatcher, func(), error) {
	b := &MetadataBatcher{}
	b.log = slog.Logger{}
	b.flushLever = make(chan struct{}, 1) // Buffered so that it doesn't block.
	b.timeNowFn = dbtime.Now

	for _, opt := range opts {
		opt(b)
	}

	if b.store == nil {
		return nil, nil, xerrors.Errorf("no store configured for metadata batcher")
	}

	if b.pubsub == nil {
		return nil, nil, xerrors.Errorf("no pubsub configured for metadata batcher")
	}

	if b.interval == 0 {
		b.interval = defaultMetadataFlushInterval
	}

	if b.batchSize == 0 {
		b.batchSize = defaultMetadataBatchSize
	}

	if b.tickCh == nil {
		b.ticker = time.NewTicker(b.interval)
		b.tickCh = b.ticker.C
	}

	b.buf = make([]agentMetadataUpdate, 0, b.batchSize)

	cancelCtx, cancelFunc := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		b.run(cancelCtx)
		close(done)
	}()

	closer := func() {
		cancelFunc()
		if b.ticker != nil {
			b.ticker.Stop()
		}
		<-done
	}

	return b, closer, nil
}

// Add adds metadata updates for an agent to the batcher.
// If the buffer is full, the update is dropped.
func (b *MetadataBatcher) Add(agentID uuid.UUID, keys []string, values []string, errors []string, collectedAt []time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// If buffer is full, drop this update.
	if len(b.buf) >= b.batchSize {
		b.log.Warn(context.Background(), "metadata buffer full, dropping update",
			slog.F("agent_id", agentID),
			slog.F("batch_size", b.batchSize),
		)
		return
	}

	// Append to buffer.
	b.buf = append(b.buf, agentMetadataUpdate{
		agentID:     agentID,
		keys:        keys,
		values:      values,
		errors:      errors,
		collectedAt: collectedAt,
	})

	// If the buffer is now full, signal immediate flush.
	if len(b.buf) >= b.batchSize && !b.flushForced.Load() {
		b.flushLever <- struct{}{}
		b.flushForced.Store(true)
	}
}

// run runs the batcher.
func (b *MetadataBatcher) run(ctx context.Context) {
	// nolint:gocritic // This is only ever used for one thing - updating agent metadata.
	authCtx := dbauthz.AsSystemRestricted(ctx)
	for {
		select {
		case <-b.tickCh:
			b.flush(authCtx, false, "scheduled")
		case <-b.flushLever:
			// If the flush lever is depressed, flush the buffer immediately.
			b.flush(authCtx, true, "reaching capacity")
		case <-ctx.Done():
			b.log.Debug(ctx, "context done, flushing before exit")

			// We must create a new context here as the parent context is done.
			ctxTimeout, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel() //nolint:revive // We're returning, defer is fine.

			// nolint:gocritic // This is only ever used for one thing - updating agent metadata.
			b.flush(dbauthz.AsSystemRestricted(ctxTimeout), true, "exit")
			return
		}
	}
}

// flush flushes the batcher's buffer.
func (b *MetadataBatcher) flush(ctx context.Context, forced bool, reason string) {
	b.mu.Lock()
	b.flushForced.Store(true)
	start := time.Now()
	count := len(b.buf)
	defer func() {
		b.flushForced.Store(false)
		b.mu.Unlock()
		if count > 0 {
			elapsed := time.Since(start)
			b.log.Debug(ctx, "flush complete",
				slog.F("count", count),
				slog.F("elapsed", elapsed),
				slog.F("forced", forced),
				slog.F("reason", reason),
			)
		}
		// Notify that a flush has completed. This only happens in tests.
		if b.flushed != nil {
			select {
			case <-ctx.Done():
				close(b.flushed)
			default:
				b.flushed <- count
			}
		}
	}()

	if len(b.buf) == 0 {
		return
	}

	// Flatten the buffer into parallel arrays for the batch query.
	var (
		agentIDs    = make([]uuid.UUID, 0, len(b.buf)*15) // Estimate 15 keys per agent
		keys        = make([]string, 0, len(b.buf)*15)
		values      = make([]string, 0, len(b.buf)*15)
		errors      = make([]string, 0, len(b.buf)*15)
		collectedAt = make([]time.Time, 0, len(b.buf)*15)
	)

	for _, update := range b.buf {
		for i := range update.keys {
			agentIDs = append(agentIDs, update.agentID)
			keys = append(keys, update.keys[i])
			values = append(values, update.values[i])
			errors = append(errors, update.errors[i])
			collectedAt = append(collectedAt, update.collectedAt[i])
		}
	}

	// Update the database with all metadata updates in a single query.
	err := b.store.BatchUpdateWorkspaceAgentMetadata(ctx, database.BatchUpdateWorkspaceAgentMetadataParams{
		WorkspaceAgentID: agentIDs,
		Key:              keys,
		Value:            values,
		Error:            errors,
		CollectedAt:      collectedAt,
	})
	elapsed := time.Since(start)
	if err != nil {
		if database.IsQueryCanceledError(err) {
			b.log.Debug(ctx, "query canceled, skipping update of workspace agent metadata", slog.F("elapsed", elapsed))
			return
		}
		b.log.Error(ctx, "error updating workspace agent metadata", slog.Error(err), slog.F("elapsed", elapsed))
		return
	}

	// Publish single batched notification with all agent IDs that have updates.
	// Listeners will re-fetch metadata for these agents from the database.
	// This scales to O(1) NOTIFY calls per flush rather than O(N) where N = agent count.

	// Build list of unique agent IDs.
	agentIDs = agentIDs[:0] // Reuse slice, clear it first
	for _, update := range b.buf {
		agentIDs = append(agentIDs, update.agentID)
	}

	batchPayload, err2 := json.Marshal(WorkspaceAgentMetadataBatchPayload{
		AgentIDs: agentIDs,
	})
	if err2 != nil {
		b.log.Error(ctx, "failed to marshal batched workspace agent metadata payload", slog.Error(err2))
	} else {
		err2 = b.pubsub.Publish(WatchWorkspaceAgentMetadataBatchChannel(), batchPayload)
		if err2 != nil {
			b.log.Error(ctx, "failed to publish batched workspace agent metadata", slog.Error(err2))
		}
	}

	// Clear the buffer by resetting to empty slice with same capacity.
	b.buf = b.buf[:0]
}
