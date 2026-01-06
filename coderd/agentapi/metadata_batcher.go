package agentapi

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/quartz"
)

const (
	// defaultMetadataBatchSize is the maximum number of metadata entries
	// (key-value pairs across all agents) to batch before forcing a flush.
	// With typical agents having 5-15 metadata keys, this accommodates
	// 30-100 agents per batch.
	defaultMetadataBatchSize = 500

	// defaultMetadataFlushInterval is how frequently to flush batched metadata
	// updates to the database and pubsub. 5 seconds provides a good balance
	// between reducing database load and maintaining reasonable UI update
	// latency.
	defaultMetadataFlushInterval = 5 * time.Second

	// maxPubsubPayloadSize is the maximum size of a single pubsub message.
	// PostgreSQL NOTIFY has an 8KB limit for the payload.
	maxPubsubPayloadSize = 8000 // Leave some headroom below 8192 bytes

	// estimatedUUIDJSONSize is the approximate size of a UUID in JSON format.
	// A UUID string is 36 characters, plus quotes (38), plus comma separator (1).
	// We use 40 bytes as a conservative estimate including JSON overhead.
	estimatedUUIDJSONSize = 40

	// pubsubBaseOverhead is the estimated JSON overhead for the pubsub payload
	// structure: {"agent_ids":[]}
	pubsubBaseOverhead = 20

	// Timeout to use for the context created when flushing the final batch due to the top level context being 'Done'
	finalFlushTimeout = 15 * time.Second
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
	store database.Store
	ps    pubsub.Pubsub
	log   slog.Logger
	clock quartz.Clock

	mu sync.Mutex
	// buf holds pending metadata updates indexed by agent ID and metadata key name.
	// Structure: map[agentID]map[metadataKeyName]metadataValue
	// This deduplicates updates to the same metadata key for the same agent within
	// a flush interval, keeping only the most recent value.
	buf        map[uuid.UUID]map[string]metadataValue
	entryCount int // Total number of metadata key-value pairs across all agents
	batchSize  int // Maximum number of metadata entries before forcing a flush

	// ticker is used to periodically flush the buffer.
	ticker *quartz.Ticker
	interval time.Duration

	// flushLever is used to signal the flusher to flush the buffer immediately.
	flushLever  chan struct{}
	flushForced atomic.Bool

	// flushed is used during testing to signal that a flush has completed.
	flushed chan<- int

	// ctx is the context for the batcher. Used to check if shutdown has begun.
	ctx context.Context

	// metrics collects Prometheus metrics for the batcher.
	metrics *MetadataBatcherMetrics
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
func MetadataBatcherWithPubsub(ps pubsub.Pubsub) MetadataBatcherOption {
	return func(b *MetadataBatcher) {
		b.ps = ps
	}
}

// MetadataBatcherWithClock sets the clock to be used for the internal flush ticker.
func MetadataBatcherWithClock(c quartz.Clock) MetadataBatcherOption {
	return func(b *MetadataBatcher) {
		b.clock = c
	}
}

// MetadataBatcherWithBatchSize sets the maximum number of metadata entries to batch.
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

// MetadataBatcherWithMetrics sets the metrics collector.
func MetadataBatcherWithMetrics(metrics *MetadataBatcherMetrics) MetadataBatcherOption {
	return func(b *MetadataBatcher) {
		b.metrics = metrics
	}
}

// NewMetadataBatcher creates a new MetadataBatcher and starts it.
func NewMetadataBatcher(ctx context.Context, opts ...MetadataBatcherOption) (*MetadataBatcher, func(), error) {
	b := &MetadataBatcher{}
	b.log = slog.Logger{}
	b.flushLever = make(chan struct{}, 1) // Buffered so that it doesn't block.

	for _, opt := range opts {
		opt(b)
	}

	if b.store == nil {
		return nil, nil, xerrors.Errorf("no store configured for metadata batcher")
	}

	if b.ps == nil {
		return nil, nil, xerrors.Errorf("no pubsub configured for metadata batcher")
	}

	if b.interval == 0 {
		b.interval = defaultMetadataFlushInterval
	}

	if b.batchSize == 0 {
		b.batchSize = defaultMetadataBatchSize
	}

	if b.clock == nil {
		b.clock = quartz.NewReal()
	}

	b.buf = make(map[uuid.UUID]map[string]metadataValue)
	b.entryCount = 0

	cancelCtx, cancelFunc := context.WithCancel(ctx)
	b.ctx = cancelCtx
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

// Add adds metadata updates for an agent to the batcher. Updates to the same metadata key for the same agent are
// deduplicated, keeping only the value with the most recent collectedAt timestamp. Older updates are silently ignored.
// If the buffer is at capacity, new keys are dropped. Returns an error if the batcher context is done (shutdown in progress).
func (b *MetadataBatcher) Add(agentID uuid.UUID, keys []string, values []string, errors []string, collectedAt []time.Time) error {
	// Check if batcher is shutting down
	select {
	case <-b.ctx.Done():
		return b.ctx.Err()
	default:
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Ensure agent map exists.
	if b.buf[agentID] == nil {
		b.buf[agentID] = make(map[string]metadataValue)
	}

	if !(len(keys) == len(values) && len(values) == len(errors) && len(errors) == len(collectedAt)) {
		return nil
	}

	// Process each key one at a time.
	droppedCount := 0
	for i := range keys {
		// Check if an entry already exists for this key.
		existingValue, exists := b.buf[agentID][keys[i]]

		// Always allow updates to existing keys, even if at capacity.
		if exists {
			// Only overwrite if the new value has a newer timestamp.
			if collectedAt[i].Before(existingValue.collectedAt) {
				continue
			}
			// Update existing key (no capacity change).
			b.buf[agentID][keys[i]] = metadataValue{
				value:       values[i],
				error:       errors[i],
				collectedAt: collectedAt[i],
			}
			continue
		}

		// New key - check capacity before adding.
		if b.entryCount >= b.batchSize {
			droppedCount++
			continue // Skip this new key but continue processing others.
		}

		// Add new key.
		b.entryCount++
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

	// Log dropped keys at the end if any were dropped.
	if droppedCount > 0 {
		b.log.Warn(context.Background(), "metadata buffer at capacity, dropped new keys",
			slog.F("agent_id", agentID),
			slog.F("entry_count", b.entryCount),
			slog.F("batch_size", b.batchSize),
			slog.F("dropped_count", droppedCount),
		)
		if b.metrics != nil {
			b.metrics.droppedKeysTotal.Add(float64(droppedCount))
		}
	}

	return nil
}

// run runs the batcher.
func (b *MetadataBatcher) run(ctx context.Context) {
	// nolint:gocritic // This is only ever used for one thing - updating agent metadata.
	authCtx := dbauthz.AsSystemRestricted(ctx)
	b.ticker = b.clock.NewTicker(b.interval)
	for {
		select {
		case <-b.ticker.C:
			fmt.Println("flushing")
			b.flush(authCtx, false, "scheduled")
			b.ticker.Reset(b.interval)
		case <-b.flushLever:
			// If the flush lever is depressed, flush the buffer immediately.
			b.flush(authCtx, true, "reaching capacity")
		case <-ctx.Done():
			b.log.Debug(ctx, "context done, flushing before exit")

			// We must create a new context here as the parent context is done.
			ctxTimeout, cancel := context.WithTimeout(context.Background(), finalFlushTimeout)
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

	// Record batch size before flattening.
	if b.metrics != nil {
		b.metrics.batchSize.Observe(float64(b.entryCount))
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
		// Record per-agent utilization.
		if b.metrics != nil {
			b.metrics.batchUtilization.Observe(float64(len(metadataMap)))
		}

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
			if b.metrics != nil {
				b.metrics.batchesTotal.WithLabelValues(reason, "canceled").Inc()
			}
			return
		}
		b.log.Error(ctx, "error updating workspace agent metadata", slog.Error(err), slog.F("elapsed", elapsed))
		if b.metrics != nil {
			b.metrics.batchesTotal.WithLabelValues(reason, "false").Inc()
		}
		return
	}

	// Build list of unique agent IDs from the map keys.
	uniqueAgentIDs := make([]uuid.UUID, 0, len(b.buf))
	for agentID := range b.buf {
		uniqueAgentIDs = append(uniqueAgentIDs, agentID)
	}

	// Publish agent IDs in chunks that fit within the pubsub size limit.
	b.publishAgentIDsInChunks(ctx, uniqueAgentIDs)

	// Record successful batch and flush duration.
	if b.metrics != nil {
		b.metrics.batchesTotal.WithLabelValues(reason, "true").Inc()
		b.metrics.flushDuration.WithLabelValues(reason).Observe(time.Since(start).Seconds())
	}

	// Clear the buffer and reset entry count.
	b.buf = make(map[uuid.UUID]map[string]metadataValue)
	b.entryCount = 0
}

// buildAndMarshalChunk builds a chunk of agent IDs that fits within the
// maxPubsubPayloadSize limit and marshals it to JSON. Returns the marshaled
// payload and the number of agent IDs consumed from the input slice.
// This function is separated for testing payload size assumptions.
func buildAndMarshalChunk(agentIDs []uuid.UUID) ([]byte, int, error) {
	if len(agentIDs) == 0 {
		return nil, 0, nil
	}

	// Use size estimation to determine initial chunk size.
	estimatedCapacity := (maxPubsubPayloadSize - pubsubBaseOverhead) / estimatedUUIDJSONSize
	if estimatedCapacity > len(agentIDs) {
		estimatedCapacity = len(agentIDs)
	}

	chunk := make([]uuid.UUID, 0, estimatedCapacity)
	estimatedSize := pubsubBaseOverhead

	for i, agentID := range agentIDs {
		newEstimatedSize := estimatedSize + estimatedUUIDJSONSize
		if newEstimatedSize > maxPubsubPayloadSize && len(chunk) > 0 {
			// Would exceed limit, marshal current chunk.
			break
		}

		chunk = append(chunk, agentID)
		estimatedSize = newEstimatedSize

		// If this is the last agent ID, no need to continue.
		if i == len(agentIDs)-1 {
			break
		}
	}

	payload, err := json.Marshal(WorkspaceAgentMetadataBatchPayload{
		AgentIDs: chunk,
	})
	if err != nil {
		return nil, 0, err
	}

	return payload, len(chunk), nil
}

// publishAgentIDsInChunks publishes agent IDs in chunks that fit within the
// PostgreSQL NOTIFY 8KB payload size limit. Each chunk is published as a
// separate message.
func (b *MetadataBatcher) publishAgentIDsInChunks(ctx context.Context, agentIDs []uuid.UUID) {
	offset := 0
	for offset < len(agentIDs) {
		payload, consumed, err := buildAndMarshalChunk(agentIDs[offset:])
		if err != nil {
			b.log.Error(ctx, "failed to marshal workspace agent metadata chunk", slog.Error(err))
			return
		}

		if consumed == 0 {
			// Should never happen, but protect against infinite loop.
			b.log.Error(ctx, "failed to make progress chunking agent IDs")
			return
		}

		// Verify our size estimate was correct (for debugging/monitoring).
		if len(payload) > maxPubsubPayloadSize {
			b.log.Warn(ctx, "pubsub payload exceeded size limit despite estimation",
				slog.F("payload_size", len(payload)),
				slog.F("limit", maxPubsubPayloadSize),
				slog.F("chunk_size", consumed),
			)
		}

		err = b.ps.Publish(WatchWorkspaceAgentMetadataBatchChannel(), payload)
		if err != nil {
			b.log.Error(ctx, "failed to publish workspace agent metadata batch",
				slog.Error(err),
				slog.F("chunk_size", consumed),
				slog.F("payload_size", len(payload)),
			)
		}

		offset += consumed
	}
}
