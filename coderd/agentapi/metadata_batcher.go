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

	// maxPubsubPayloadSize is the maximum size of a single pubsub message.
	// PostgreSQL NOTIFY has an 8KB limit for the payload.
	maxPubsubPayloadSize = 8000 // Leave some headroom below 8192 bytes
)

// metadataValue holds a single metadata key-value pair with its error state
// and collection timestamp.
type metadataValue struct {
	value       string
	error       string
	collectedAt time.Time
}

// MetadataBatcher holds a buffer of agent metadata updates and periodically
// flushes them to the database and pubsub. This reduces database write
// frequency and pubsub publish rate.
type MetadataBatcher struct {
	store  database.Store
	pubsub pubsub.Pubsub
	log    slog.Logger

	mu sync.Mutex
	// buf holds pending metadata updates indexed by agent ID and metadata key name.
	// Structure: map[agentID]map[metadataKeyName]metadataValue
	// This deduplicates updates to the same metadata key for the same agent within
	// a flush interval, keeping only the most recent value.
	buf        map[uuid.UUID]map[string]metadataValue
	entryCount int // Total number of metadata key-value pairs across all agents
	batchSize  int // Maximum number of metadata entries before forcing a flush

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

	b.buf = make(map[uuid.UUID]map[string]metadataValue)
	b.entryCount = 0

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
// Updates to the same metadata key for the same agent are deduplicated,
// keeping only the value with the most recent collectedAt timestamp. Older
// updates are silently ignored. If the buffer is at capacity, updates are
// dropped.
func (b *MetadataBatcher) Add(agentID uuid.UUID, keys []string, values []string, errors []string, collectedAt []time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Ensure agent map exists.
	if b.buf[agentID] == nil {
		b.buf[agentID] = make(map[string]metadataValue)
	}

	// Process each key one at a time.
	for i := range keys {
		// If buffer is already at max capacity, drop this and remaining keys.
		if b.entryCount >= b.batchSize {
			b.log.Warn(context.Background(), "metadata buffer at capacity, dropping remaining keys",
				slog.F("agent_id", agentID),
				slog.F("entry_count", b.entryCount),
				slog.F("batch_size", b.batchSize),
				slog.F("dropped_keys", len(keys)-i),
			)
			return
		}

		// Check if an entry already exists for this key.
		existingValue, exists := b.buf[agentID][keys[i]]
		if exists {
			// Only overwrite if the new data is newer than the existing data.
			if collectedAt[i].Before(existingValue.collectedAt) {
				// Skip this update - the existing data is newer.
				continue
			}
			// Existing entry will be replaced, no change to entryCount.
		} else {
			// New key - increment the entry count.
			b.entryCount++
		}

		b.buf[agentID][keys[i]] = metadataValue{
			value:       values[i],
			error:       errors[i],
			collectedAt: collectedAt[i],
		}

		// If we've reached capacity after adding this key, trigger immediate flush.
		if b.entryCount >= b.batchSize && !b.flushForced.Load() {
			b.flushLever <- struct{}{}
			b.flushForced.Store(true)
		}
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
	count := b.entryCount
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

	if b.entryCount == 0 {
		return
	}

	// Flatten the buffer into parallel arrays for the batch query.
	var (
		agentIDs    = make([]uuid.UUID, 0, b.entryCount)
		keys        = make([]string, 0, b.entryCount)
		values      = make([]string, 0, b.entryCount)
		errors      = make([]string, 0, b.entryCount)
		collectedAt = make([]time.Time, 0, b.entryCount)
	)

	for agentID, metadataMap := range b.buf {
		for key, metadata := range metadataMap {
			agentIDs = append(agentIDs, agentID)
			keys = append(keys, key)
			values = append(values, metadata.value)
			errors = append(errors, metadata.error)
			collectedAt = append(collectedAt, metadata.collectedAt)
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

	// Publish batched notifications with all agent IDs that have updates.
	// Listeners will re-fetch metadata for these agents from the database.
	// If the payload exceeds PostgreSQL's 8KB NOTIFY limit, split into
	// multiple messages.

	// Build list of unique agent IDs from the map keys.
	uniqueAgentIDs := make([]uuid.UUID, 0, len(b.buf))
	for agentID := range b.buf {
		uniqueAgentIDs = append(uniqueAgentIDs, agentID)
	}

	// Publish agent IDs in chunks that fit within the pubsub size limit.
	b.publishAgentIDsInChunks(ctx, uniqueAgentIDs)

	// Clear the buffer and reset entry count.
	b.buf = make(map[uuid.UUID]map[string]metadataValue)
	b.entryCount = 0
}

// publishAgentIDsInChunks publishes agent IDs in chunks that fit within the
// PostgreSQL NOTIFY 8KB payload size limit. Each chunk is published as a
// separate message.
func (b *MetadataBatcher) publishAgentIDsInChunks(ctx context.Context, agentIDs []uuid.UUID) {
	if len(agentIDs) == 0 {
		return
	}

	var currentChunk []uuid.UUID

	for _, agentID := range agentIDs {
		// Try adding this agent ID to the current chunk.
		testChunk := append(currentChunk, agentID)
		payload, err := json.Marshal(WorkspaceAgentMetadataBatchPayload{
			AgentIDs: testChunk,
		})
		if err != nil {
			b.log.Error(ctx, "failed to marshal workspace agent metadata payload", slog.Error(err))
			continue
		}

		// If adding this agent would exceed the limit, publish current chunk
		// and start a new one with this agent.
		if len(payload) > maxPubsubPayloadSize {
			// Publish current chunk if it has any agents.
			if len(currentChunk) > 0 {
				chunkPayload, err := json.Marshal(WorkspaceAgentMetadataBatchPayload{
					AgentIDs: currentChunk,
				})
				if err != nil {
					b.log.Error(ctx, "failed to marshal workspace agent metadata chunk", slog.Error(err))
				} else {
					err = b.pubsub.Publish(WatchWorkspaceAgentMetadataBatchChannel(), chunkPayload)
					if err != nil {
						b.log.Error(ctx, "failed to publish workspace agent metadata batch",
							slog.Error(err),
							slog.F("chunk_size", len(currentChunk)),
						)
					}
				}
			}

			// Start new chunk with current agent.
			currentChunk = []uuid.UUID{agentID}
		} else {
			// Agent fits in current chunk.
			currentChunk = testChunk
		}
	}

	// Publish final chunk if it has any agents.
	if len(currentChunk) > 0 {
		payload, err := json.Marshal(WorkspaceAgentMetadataBatchPayload{
			AgentIDs: currentChunk,
		})
		if err != nil {
			b.log.Error(ctx, "failed to marshal workspace agent metadata final chunk", slog.Error(err))
		} else {
			err = b.pubsub.Publish(WatchWorkspaceAgentMetadataBatchChannel(), payload)
			if err != nil {
				b.log.Error(ctx, "failed to publish workspace agent metadata batch",
					slog.Error(err),
					slog.F("chunk_size", len(currentChunk)),
				)
			}
		}
	}
}
