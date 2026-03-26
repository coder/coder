package agentconnectionbatcher

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/quartz"
)

const (
	// defaultBatchSize is the maximum number of agent connection updates
	// to batch before forcing a flush. With one entry per agent, this
	// accommodates 500 concurrently connected agents per batch.
	defaultBatchSize = 500

	// defaultChannelBufferMultiplier is the multiplier for the channel
	// buffer size relative to the batch size. A 5x multiplier provides
	// significant headroom for bursts while the batch is being flushed.
	defaultChannelBufferMultiplier = 5

	// defaultFlushInterval is how frequently to flush batched connection
	// updates to the database. 5 seconds provides a good balance between
	// reducing database load and keeping connection state reasonably
	// current.
	defaultFlushInterval = 5 * time.Second

	// finalFlushTimeout is the timeout for the final flush when the
	// batcher is shutting down.
	finalFlushTimeout = 15 * time.Second
)

// Update represents a single agent connection state update to be batched.
type Update struct {
	ID                     uuid.UUID
	FirstConnectedAt       sql.NullTime
	LastConnectedAt        sql.NullTime
	LastConnectedReplicaID uuid.NullUUID
	DisconnectedAt         sql.NullTime
	UpdatedAt              time.Time
}

// Batcher accumulates agent connection updates and periodically flushes
// them to the database in a single batch query. This reduces per-heartbeat
// database write pressure from O(n) queries to O(1).
type Batcher struct {
	store database.Store
	log   slog.Logger

	updateCh     chan Update
	batch        map[uuid.UUID]Update
	maxBatchSize int

	clock    quartz.Clock
	timer    *quartz.Timer
	interval time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// Option is a functional option for configuring a Batcher.
type Option func(b *Batcher)

// WithBatchSize sets the maximum number of updates to accumulate before
// forcing a flush.
func WithBatchSize(size int) Option {
	return func(b *Batcher) {
		b.maxBatchSize = size
	}
}

// WithInterval sets how frequently the batcher flushes to the database.
func WithInterval(d time.Duration) Option {
	return func(b *Batcher) {
		b.interval = d
	}
}

// WithLogger sets the logger for the batcher.
func WithLogger(log slog.Logger) Option {
	return func(b *Batcher) {
		b.log = log
	}
}

// WithClock sets the clock for the batcher, useful for testing.
func WithClock(clock quartz.Clock) Option {
	return func(b *Batcher) {
		b.clock = clock
	}
}

// New creates a new Batcher and starts its background processing loop.
// The provided context controls the lifetime of the batcher.
func New(ctx context.Context, store database.Store, opts ...Option) *Batcher {
	b := &Batcher{
		store: store,
		done:  make(chan struct{}),
		log:   slog.Logger{},
		clock: quartz.NewReal(),
	}

	for _, opt := range opts {
		opt(b)
	}

	if b.interval == 0 {
		b.interval = defaultFlushInterval
	}
	if b.maxBatchSize == 0 {
		b.maxBatchSize = defaultBatchSize
	}

	b.timer = b.clock.NewTimer(b.interval)
	channelSize := b.maxBatchSize * defaultChannelBufferMultiplier
	b.updateCh = make(chan Update, channelSize)
	b.batch = make(map[uuid.UUID]Update)

	b.ctx, b.cancel = context.WithCancel(ctx)
	go func() {
		b.run(b.ctx)
		close(b.done)
	}()

	return b
}

// Close cancels the batcher context and waits for the final flush to
// complete.
func (b *Batcher) Close() {
	b.cancel()
	if b.timer != nil {
		b.timer.Stop()
	}
	<-b.done
}

// Add enqueues an agent connection update for batching. If the
// channel is full, a direct (unbatched) DB write is performed as a
// fallback so that heartbeats are never silently lost.
func (b *Batcher) Add(u Update) {
	select {
	case b.updateCh <- u:
	default:
		b.log.Warn(context.Background(), "connection batcher channel full, falling back to direct write",
			slog.F("agent_id", u.ID),
		)
		b.writeDirect(u)
	}
}

// writeDirect performs a single-item batch write as a fallback when
// the channel is full. This ensures heartbeats are never lost, which
// would cause agents to be erroneously marked as disconnected.
func (b *Batcher) writeDirect(u Update) {
	//nolint:gocritic // System-level fallback for agent heartbeats.
	ctx, cancel := context.WithTimeout(dbauthz.AsSystemRestricted(context.Background()), 10*time.Second)
	defer cancel()
	err := b.store.BatchUpdateWorkspaceAgentConnections(ctx, database.BatchUpdateWorkspaceAgentConnectionsParams{
		ID:                     []uuid.UUID{u.ID},
		FirstConnectedAt:       []time.Time{nullTimeToTime(u.FirstConnectedAt)},
		LastConnectedAt:        []time.Time{nullTimeToTime(u.LastConnectedAt)},
		LastConnectedReplicaID: []uuid.UUID{nullUUIDToUUID(u.LastConnectedReplicaID)},
		DisconnectedAt:         []time.Time{nullTimeToTime(u.DisconnectedAt)},
		UpdatedAt:              []time.Time{u.UpdatedAt},
	})
	if err != nil {
		b.log.Error(context.Background(), "direct heartbeat write failed",
			slog.F("agent_id", u.ID), slog.Error(err))
	}
}

func (b *Batcher) processUpdate(u Update) {
	existing, exists := b.batch[u.ID]
	if exists && u.UpdatedAt.Before(existing.UpdatedAt) {
		return
	}
	b.batch[u.ID] = u
}

func (b *Batcher) run(ctx context.Context) {
	//nolint:gocritic // System-level batch operation for agent connections.
	authCtx := dbauthz.AsSystemRestricted(ctx)
	for {
		select {
		case u := <-b.updateCh:
			b.processUpdate(u)

			if len(b.batch) >= b.maxBatchSize {
				b.flush(authCtx)
				b.timer.Reset(b.interval, "connectionBatcher", "capacityFlush")
			}

		case <-b.timer.C:
			b.flush(authCtx)
			b.timer.Reset(b.interval, "connectionBatcher", "scheduledFlush")

		case <-ctx.Done():
			b.log.Debug(ctx, "context done, flushing before exit")

			ctxTimeout, cancel := context.WithTimeout(context.Background(), finalFlushTimeout)
			defer cancel() //nolint:revive // Returning after this.

			//nolint:gocritic // System-level batch operation for agent connections.
			b.flush(dbauthz.AsSystemRestricted(ctxTimeout))
			return
		}
	}
}

func (b *Batcher) flush(ctx context.Context) {
	count := len(b.batch)
	if count == 0 {
		return
	}

	b.log.Debug(ctx, "flushing connection batch", slog.F("count", count))

	var (
		ids                     = make([]uuid.UUID, 0, count)
		firstConnectedAt       = make([]time.Time, 0, count)
		lastConnectedAt        = make([]time.Time, 0, count)
		lastConnectedReplicaID = make([]uuid.UUID, 0, count)
		disconnectedAt         = make([]time.Time, 0, count)
		updatedAt              = make([]time.Time, 0, count)
	)

	for _, u := range b.batch {
		ids = append(ids, u.ID)
		firstConnectedAt = append(firstConnectedAt, nullTimeToTime(u.FirstConnectedAt))
		lastConnectedAt = append(lastConnectedAt, nullTimeToTime(u.LastConnectedAt))
		lastConnectedReplicaID = append(lastConnectedReplicaID, nullUUIDToUUID(u.LastConnectedReplicaID))
		disconnectedAt = append(disconnectedAt, nullTimeToTime(u.DisconnectedAt))
		updatedAt = append(updatedAt, u.UpdatedAt)
	}

	// Clear batch before the DB call. Losing a batch of heartbeat
	// timestamps is acceptable; the next heartbeat will update them.
	b.batch = make(map[uuid.UUID]Update)

	err := b.store.BatchUpdateWorkspaceAgentConnections(ctx, database.BatchUpdateWorkspaceAgentConnectionsParams{
		ID:                     ids,
		FirstConnectedAt:       firstConnectedAt,
		LastConnectedAt:        lastConnectedAt,
		LastConnectedReplicaID: lastConnectedReplicaID,
		DisconnectedAt:         disconnectedAt,
		UpdatedAt:              updatedAt,
	})
	if err != nil {
		if database.IsQueryCanceledError(err) {
			b.log.Debug(ctx, "query canceled, skipping connection batch update")
			return
		}
		b.log.Error(ctx, "failed to batch update agent connections", slog.Error(err))
		return
	}

	b.log.Debug(ctx, "connection batch flush complete", slog.F("count", count))
}

// nullTimeToTime converts a sql.NullTime to a time.Time. When the
// NullTime is not valid, the zero time is returned which PostgreSQL
// will store as the epoch. The batch query uses unnest over plain
// time arrays, so we cannot pass NULL directly.
func nullTimeToTime(nt sql.NullTime) time.Time {
	if nt.Valid {
		return nt.Time
	}
	return time.Time{}
}

// nullUUIDToUUID converts a uuid.NullUUID to a uuid.UUID. When the
// NullUUID is not valid, uuid.Nil is returned.
func nullUUIDToUUID(nu uuid.NullUUID) uuid.UUID {
	if nu.Valid {
		return nu.UUID
	}
	return uuid.Nil
}
