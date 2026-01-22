package metadatabatcher

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
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

// compositeKey uniquely identifies a metadata entry by agent ID and key name.
type compositeKey struct {
	agentID uuid.UUID
	key     string
}

// value holds a single metadata key-value pair with its error state
// and collection timestamp.
type value struct {
	v           string
	error       string
	collectedAt time.Time
}

// update represents a single metadata update to be batched.
type update struct {
	compositeKey
	value
}

// Batcher holds a buffer of agent metadata updates and periodically
// flushes them to the database and pubsub. This reduces database write
// frequency and pubsub publish rate.
type Batcher struct {
	store database.Store
	ps    pubsub.Pubsub
	log   slog.Logger

	// updateCh is the buffered channel that receives metadata updates from Add() calls.
	updateCh chan update

	// batch holds the current batch being accumulated. For updates with the same composite key the most recent value wins.
	batch           map[compositeKey]value
	currentBatchLen atomic.Int64
	maxBatchSize    int

	clock    quartz.Clock
	timer    *quartz.Timer
	interval time.Duration
	// Used to only log at warn level for dropped keys infrequently, as it could be noisy in failure scenarios.
	warnTicker *quartz.Ticker

	// ctx is the context for the batcher. Used to check if shutdown has begun.
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	// metrics collects Prometheus metrics for the batcher.
	metrics Metrics
}

// Option is a functional option for configuring a Batcher.
type Option func(b *Batcher)

func WithBatchSize(size int) Option {
	return func(b *Batcher) {
		b.maxBatchSize = size
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

// NewBatcher creates a new Batcher and starts it. Here ctx controls the lifetime of the batcher, canceling it will
// result in the Batcher exiting it's processing routine (run).
func NewBatcher(ctx context.Context, reg prometheus.Registerer, store database.Store, ps pubsub.Pubsub, opts ...Option) (*Batcher, error) {
	b := &Batcher{
		store:   store,
		ps:      ps,
		metrics: NewMetrics(),
		done:    make(chan struct{}),
		log:     slog.Logger{},
		clock:   quartz.NewReal(),
	}

	for _, opt := range opts {
		opt(b)
	}

	b.metrics.register(reg)

	if b.interval == 0 {
		b.interval = defaultMetadataFlushInterval
	}

	if b.maxBatchSize == 0 {
		b.maxBatchSize = defaultMetadataBatchSize
	}

	// Create warn ticker after options are applied so it uses the correct clock.
	b.warnTicker = b.clock.NewTicker(10 * time.Second)

	if b.timer == nil {
		b.timer = b.clock.NewTimer(b.interval)
	}

	// Create buffered channel with 5x batch size capacity
	channelSize := b.maxBatchSize * defaultChannelBufferMultiplier
	b.updateCh = make(chan update, channelSize)

	// Initialize batch map
	b.batch = make(map[compositeKey]value)

	b.ctx, b.cancel = context.WithCancel(ctx)
	go func() {
		b.run(b.ctx)
		close(b.done)
	}()

	return b, nil
}

func (b *Batcher) Close() {
	b.cancel()
	if b.timer != nil {
		b.timer.Stop()
	}
	// Wait for the run function to end, it may be sending one last batch.
	<-b.done
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
	var u update
	droppedCount := 0
	for i := range keys {
		u.agentID = agentID
		u.key = keys[i]
		u.v = values[i]
		u.error = errors[i]
		u.collectedAt = collectedAt[i]

		select {
		case b.updateCh <- u:
			// Successfully queued
		default:
			// Channel is full, drop this update
			droppedCount++
		}
	}

	// Log dropped keys if any were dropped.
	if droppedCount > 0 {
		msg := "metadata channel at capacity, dropped updates"
		fields := []slog.Field{
			slog.F("agent_id", agentID),
			slog.F("channel_size", cap(b.updateCh)),
			slog.F("dropped_count", droppedCount),
		}
		select {
		case <-b.warnTicker.C:
			b.log.Warn(context.Background(), msg, fields...)
		default:
			b.log.Debug(context.Background(), msg, fields...)
		}

		b.metrics.droppedKeysTotal.Add(float64(droppedCount))
	}

	return nil
}

// processUpdate adds a metadata update to the batch with deduplication based on timestamp.
func (b *Batcher) processUpdate(update update) {
	ck := compositeKey{
		agentID: update.agentID,
		key:     update.key,
	}

	// Check if key already exists and only update if new value is newer.
	existing, exists := b.batch[ck]
	if exists && update.collectedAt.Before(existing.collectedAt) {
		return
	}

	b.batch[ck] = value{
		v:           update.v,
		error:       update.error,
		collectedAt: update.collectedAt,
	}
	if !exists {
		b.currentBatchLen.Add(1)
	}
}

// run runs the batcher loop, reading from the update channel and flushing
// periodically or when the batch reaches capacity.
func (b *Batcher) run(ctx context.Context) {
	// nolint:gocritic // This is only ever used for one thing - updating agent metadata.
	authCtx := dbauthz.AsSystemRestricted(ctx)
	for {
		select {
		case update := <-b.updateCh:
			b.processUpdate(update)

			// Check if batch has reached capacity
			if int(b.currentBatchLen.Load()) >= b.maxBatchSize {
				b.flush(authCtx, flushCapacity)
				// Reset timer so the next scheduled flush is interval duration
				// from now, not from when it was originally scheduled.
				b.timer.Reset(b.interval, "metadataBatcher", "capacityFlush")
			}

		case <-b.timer.C:
			b.flush(authCtx, flushTicker)
			// Reset timer to schedule the next flush.
			b.timer.Reset(b.interval, "metadataBatcher", "scheduledFlush")

		case <-ctx.Done():
			b.log.Debug(ctx, "context done, flushing before exit")

			// We must create a new context here as the parent context is done.
			ctxTimeout, cancel := context.WithTimeout(context.Background(), finalFlushTimeout)
			defer cancel() //nolint:revive // We're returning, defer is fine.

			// nolint:gocritic // This is only ever used for one thing - updating agent metadata.
			b.flush(dbauthz.AsSystemRestricted(ctxTimeout), flushExit)
			return
		}
	}
}

// flush flushes the current batch to the database and pubsub.
func (b *Batcher) flush(ctx context.Context, reason string) {
	count := len(b.batch)

	if count == 0 {
		return
	}

	start := b.clock.Now()
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
		values = append(values, mv.v)
		errors = append(errors, mv.error)
		collectedAt = append(collectedAt, mv.collectedAt)
		agentKeys[ck.agentID]++
	}

	// Batch has been processed into slices for our DB request, so we can clear it.
	// It's safe to clear before we know whether the flush is successful as agent metadata is not critical, and therefore
	// we do not retry failed flushes and losing a batch of metadata is okay.
	b.batch = make(map[compositeKey]value)
	b.currentBatchLen.Store(0)

	// Record per-agent utilization metrics.
	for _, keyCount := range agentKeys {
		b.metrics.batchUtilization.Observe(float64(keyCount))
	}

	// Update the database with all metadata updates in a single query.
	err := b.store.BatchUpdateWorkspaceAgentMetadata(ctx, database.BatchUpdateWorkspaceAgentMetadataParams{
		WorkspaceAgentID: agentIDs,
		Key:              keys,
		Value:            values,
		Error:            errors,
		CollectedAt:      collectedAt,
	})
	elapsed := b.clock.Since(start)

	if err != nil {
		if database.IsQueryCanceledError(err) {
			b.log.Debug(ctx, "query canceled, skipping update of workspace agent metadata", slog.F("elapsed", elapsed))
			return
		}
		b.log.Error(ctx, "error updating workspace agent metadata", slog.Error(err), slog.F("elapsed", elapsed))
		return
	}

	// Build list of unique agent IDs for pubsub notification.
	uniqueAgentIDs := make([]uuid.UUID, 0, len(agentKeys))
	for agentID := range agentKeys {
		uniqueAgentIDs = append(uniqueAgentIDs, agentID)
	}

	// Encode agent IDs into chunks and publish them.
	chunks, err := EncodeAgentIDChunks(uniqueAgentIDs)
	if err != nil {
		b.log.Error(ctx, "Agent ID chunk encoding for pubsub failed",
			slog.Error(err))
	}
	for _, chunk := range chunks {
		if err := b.ps.Publish(MetadataBatchPubsubChannel, chunk); err != nil {
			b.log.Error(ctx, "failed to publish workspace agent metadata batch",
				slog.Error(err),
				slog.F("chunk_size", len(chunk)/UUIDBase64Size),
				slog.F("payload_size", len(chunk)),
			)
			b.metrics.publishErrors.Inc()
		}
	}

	// Record successful batch size and flush duration after successful send/publish.
	b.metrics.batchSize.Observe(float64(count))
	b.metrics.metadataTotal.Add(float64(count))
	b.metrics.batchesTotal.WithLabelValues(reason).Inc()
	b.metrics.flushDuration.WithLabelValues(reason).Observe(time.Since(start).Seconds())

	elapsed = time.Since(start)
	b.log.Debug(ctx, "flush complete",
		slog.F("count", count),
		slog.F("elapsed", elapsed),
		slog.F("reason", reason),
	)
}
