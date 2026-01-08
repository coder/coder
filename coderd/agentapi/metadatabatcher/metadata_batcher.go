package metadatabatcher

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
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

	// defaultChannelBufferMultiplier is the multiplier for the channel buffer size
	// relative to the batch size. A 5x multiplier provides significant headroom
	// for bursts while the batch is being flushed.
	defaultChannelBufferMultiplier = 5

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

	// Channel to publish batch metadata updates to, each update contains a list of all Agent IDs that have an update in
	// the most recent batch
	MetadataBatchPubsubChannel = "workspace_agent_metadata_batch"

	// flush reasons
	flushCapacity = "capacity"
	flushTicker   = "scheduled"
	flushExit     = "shutdown"
)

// WorkspaceAgentMetadataBatchPayload is published to the batched metadata
// channel with agent IDs that have metadata updates. Listeners should
// re-fetch metadata for these agents from the database.
type WorkspaceAgentMetadataBatchPayload struct {
	AgentIDs []uuid.UUID `json:"agent_ids"`
}

// compositeKey uniquely identifies a metadata entry by agent ID and key name.
type compositeKey struct {
	agentID uuid.UUID
	key     string
}

// metadataValue holds a single metadata key-value pair with its error state
// and collection timestamp.
type metadataValue struct {
	value       string
	error       string
	collectedAt time.Time
}

// metadataUpdate represents a single metadata update to be batched.
type metadataUpdate struct {
	agentID     uuid.UUID
	key         string
	value       string
	error       string
	collectedAt time.Time
}

// Batcher holds a buffer of agent metadata updates and periodically
// flushes them to the database and pubsub. This reduces database write
// frequency and pubsub publish rate.
type Batcher struct {
	store database.Store
	ps    pubsub.Pubsub
	log   slog.Logger

	// updateCh is the buffered channel that receives metadata updates from Add() calls.
	updateCh chan metadataUpdate

	// batch holds the current batch being accumulated. Updates with the same
	// composite key are deduplicated, keeping only the most recent value.
	batch     map[compositeKey]metadataValue
	batchSize int

	// clock is used to create tickers and get the current time.
	clock    quartz.Clock
	ticker   *quartz.Ticker
	interval time.Duration

	// ctx is the context for the batcher. Used to check if shutdown has begun.
	ctx context.Context

	// metrics collects Prometheus metrics for the batcher.
	metrics *Metrics
}

// Option is a functional option for configuring a Batcher.
type Option func(b *Batcher)

func WithBatchSize(size int) Option {
	return func(b *Batcher) {
		b.batchSize = size
	}
}

func WithInterval(d time.Duration) Option {
	return func(b *Batcher) {
		b.interval = d
	}
}

func WithLogger(log slog.Logger) Option {
	return func(b *Batcher) {
		b.log = log
	}
}

func WithClock(clock quartz.Clock) Option {
	return func(b *Batcher) {
		b.clock = clock
	}
}

// NewBatcher creates a new Batcher and starts it.
func NewBatcher(ctx context.Context, reg prometheus.Registerer, store database.Store, ps pubsub.Pubsub, opts ...Option) (*Batcher, func(), error) {
	b := &Batcher{
		store:   store,
		ps:      ps,
		metrics: NewMetrics(),
	}
	b.log = slog.Logger{}
	b.clock = quartz.NewReal()

	for _, opt := range opts {
		opt(b)
	}

	b.metrics.register(reg)

	if b.interval == 0 {
		b.interval = defaultMetadataFlushInterval
	}

	if b.batchSize == 0 {
		b.batchSize = defaultMetadataBatchSize
	}

	if b.ticker == nil {
		b.ticker = b.clock.NewTicker(b.interval)
	}

	// Create buffered channel with 5x batch size capacity
	channelSize := b.batchSize * defaultChannelBufferMultiplier
	b.updateCh = make(chan metadataUpdate, channelSize)

	// Initialize batch map
	b.batch = make(map[compositeKey]metadataValue)

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

// Add adds metadata updates for an agent to the batcher by writing to a
// buffered channel. If the channel is full, updates are dropped. Updates
// to the same metadata key for the same agent are deduplicated in the batch,
// keeping only the value with the most recent collectedAt timestamp.
func (b *Batcher) Add(agentID uuid.UUID, keys []string, values []string, errors []string, collectedAt []time.Time) error {
	if !(len(keys) == len(values) && len(values) == len(errors) && len(errors) == len(collectedAt)) {
		return xerrors.Errorf("invalid Add call, all inputs must have the same number of items; keys: %d, values: %d, errors: %d, collectedAt: %d", len(keys), len(values), len(errors), len(collectedAt))
	}

	// Write each update to the channel. If the channel is full, drop the update.
	droppedCount := 0
	for i := range keys {
		update := metadataUpdate{
			agentID:     agentID,
			key:         keys[i],
			value:       values[i],
			error:       errors[i],
			collectedAt: collectedAt[i],
		}

		select {
		case b.updateCh <- update:
			// Successfully queued
		default:
			// Channel is full, drop this update
			droppedCount++
		}
	}

	// Log dropped keys if any were dropped.
	if droppedCount > 0 {
		b.log.Warn(context.Background(), "metadata channel at capacity, dropped updates",
			slog.F("agent_id", agentID),
			slog.F("channel_size", cap(b.updateCh)),
			slog.F("dropped_count", droppedCount),
		)
		if b.metrics != nil {
			b.metrics.droppedKeysTotal.Add(float64(droppedCount))
		}
	}

	return nil
}

// processUpdate adds a metadata update to the batch with deduplication based on timestamp.
func (b *Batcher) processUpdate(update metadataUpdate) {
	ck := compositeKey{
		agentID: update.agentID,
		key:     update.key,
	}

	// Check if key already exists and only update if new value is newer
	if existing, exists := b.batch[ck]; exists {
		if update.collectedAt.After(existing.collectedAt) {
			b.batch[ck] = metadataValue{
				value:       update.value,
				error:       update.error,
				collectedAt: update.collectedAt,
			}
		}
		// Else: existing value is newer or same, ignore this update
		return
	}

	// New key, add to batch
	b.batch[ck] = metadataValue{
		value:       update.value,
		error:       update.error,
		collectedAt: update.collectedAt,
	}
}

// run runs the batcher loop, reading from the update channel and flushing
// periodically or when the batch reaches capacity.
func (b *Batcher) run(ctx context.Context) {
	flush := func(ctx context.Context, reason string) {
		if err := b.flush(ctx, reason); err != nil {
			// Don't error level log here, database errors here are inconvenient but very much possible.
			//nolint:gocritic
			b.log.Warn(context.Background(), "metadata flush failed",
				slog.F("err_msg", err),
			)
		}
	}

	// nolint:gocritic // This is only ever used for one thing - updating agent metadata.
	authCtx := dbauthz.AsSystemRestricted(ctx)
	for {
		select {
		case update := <-b.updateCh:
			b.processUpdate(update)

			// Check if batch has reached capacity
			if len(b.batch) >= b.batchSize {
				flush(authCtx, flushCapacity)
			}

		case <-b.ticker.C:
			flush(authCtx, flushTicker)

		case <-ctx.Done():
			b.log.Debug(ctx, "context done, flushing before exit")

			// We must create a new context here as the parent context is done.
			ctxTimeout, cancel := context.WithTimeout(context.Background(), finalFlushTimeout)
			defer cancel() //nolint:revive // We're returning, defer is fine.

			// nolint:gocritic // This is only ever used for one thing - updating agent metadata.
			flush(dbauthz.AsSystemRestricted(ctxTimeout), flushExit)
			return
		}
	}
}

// flush flushes the current batch to the database and pubsub.
func (b *Batcher) flush(ctx context.Context, reason string) error {
	count := len(b.batch)

	if count == 0 {
		return nil
	}

	start := time.Now()
	b.log.Debug(ctx, "flushing metadata batch",
		slog.F("reason", reason),
		slog.F("count", count),
	)

	// Convert batch map to parallel arrays for the batch query.
	// Also build map of agent IDs for per-agent metrics and pubsub.
	var (
		agentIDs    = make([]uuid.UUID, 0, count)
		keys        = make([]string, 0, count)
		values      = make([]string, 0, count)
		errors      = make([]string, 0, count)
		collectedAt = make([]time.Time, 0, count)
		agentKeys   = make(map[uuid.UUID]int) // Track keys per agent for metrics
	)

	for ck, mv := range b.batch {
		agentIDs = append(agentIDs, ck.agentID)
		keys = append(keys, ck.key)
		values = append(values, mv.value)
		errors = append(errors, mv.error)
		collectedAt = append(collectedAt, mv.collectedAt)
		agentKeys[ck.agentID]++
	}

	// Record per-agent utilization metrics.
	if b.metrics != nil {
		for _, keyCount := range agentKeys {
			b.metrics.batchUtilization.Observe(float64(keyCount))
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
			// Clear batch since we're not retrying
			b.batch = make(map[compositeKey]metadataValue)
			return err
		}
		b.log.Error(ctx, "error updating workspace agent metadata", slog.Error(err), slog.F("elapsed", elapsed))
		// Clear batch - we don't retry on errors
		b.batch = make(map[compositeKey]metadataValue)
		return err
	}

	// Build list of unique agent IDs for pubsub notification.
	uniqueAgentIDs := make([]uuid.UUID, 0, len(agentKeys))
	for agentID := range agentKeys {
		uniqueAgentIDs = append(uniqueAgentIDs, agentID)
	}

	// Publish agent IDs in chunks that fit within the pubsub size limit.
	b.publishAgentIDsInChunks(ctx, uniqueAgentIDs)

	// Record successful batch size and flush duration after successful send/publish.
	if b.metrics != nil {
		b.metrics.batchSize.Observe(float64(count))
		b.metrics.metadataTotal.Add(float64(count))
		b.metrics.batchesTotal.WithLabelValues(reason).Inc()
		b.metrics.flushDuration.WithLabelValues(reason).Observe(time.Since(start).Seconds())
	}

	// Clear the batch.
	b.batch = make(map[compositeKey]metadataValue)

	elapsed = time.Since(start)
	b.log.Debug(ctx, "flush complete",
		slog.F("count", count),
		slog.F("elapsed", elapsed),
		slog.F("reason", reason),
	)

	return nil
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
func (b *Batcher) publishAgentIDsInChunks(ctx context.Context, agentIDs []uuid.UUID) {
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

		err = b.ps.Publish(MetadataBatchPubsubChannel, payload)
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
