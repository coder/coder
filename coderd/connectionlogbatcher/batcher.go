package connectionlogbatcher

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/quartz"
)

const (
	// defaultBatchSize is the maximum number of connection log entries
	// to batch before forcing a flush.
	defaultBatchSize = 500

	// defaultChannelBufferMultiplier is the multiplier for the channel
	// buffer size relative to the batch size. A 5x multiplier provides
	// significant headroom for bursts while the batch is being flushed.
	defaultChannelBufferMultiplier = 5

	// defaultFlushInterval is how frequently to flush batched connection
	// log entries to the database. 1 second keeps audit logs near
	// real-time.
	defaultFlushInterval = time.Second

	// finalFlushTimeout is the timeout for the final flush when the
	// batcher is shutting down.
	finalFlushTimeout = 15 * time.Second
)

// Batcher accumulates connection log upserts and periodically flushes
// them to the database in a single batch query. This reduces per-event
// database write pressure from O(n) queries to O(1).
type Batcher struct {
	store database.Store
	log   slog.Logger

	itemCh       chan database.UpsertConnectionLogParams
	batch        []database.UpsertConnectionLogParams
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

// WithBatchSize sets the maximum number of entries to accumulate before
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
	b.itemCh = make(chan database.UpsertConnectionLogParams, channelSize)
	b.batch = make([]database.UpsertConnectionLogParams, 0, b.maxBatchSize)

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

// Add enqueues a connection log upsert for batching. If the internal
// channel is full, the entry is dropped and a warning is logged.
func (b *Batcher) Add(item database.UpsertConnectionLogParams) {
	select {
	case b.itemCh <- item:
	default:
		b.log.Warn(context.Background(), "connection log batcher channel full, dropping entry",
			slog.F("connection_id", item.ConnectionID),
		)
	}
}

func (b *Batcher) run(ctx context.Context) {
	//nolint:gocritic // System-level batch operation for connection logs.
	authCtx := dbauthz.AsConnectionLogger(ctx)
	for {
		select {
		case item := <-b.itemCh:
			b.batch = append(b.batch, item)

			if len(b.batch) >= b.maxBatchSize {
				b.flush(authCtx)
				b.timer.Reset(b.interval, "connectionLogBatcher", "capacityFlush")
			}

		case <-b.timer.C:
			b.flush(authCtx)
			b.timer.Reset(b.interval, "connectionLogBatcher", "scheduledFlush")

		case <-ctx.Done():
			b.log.Debug(ctx, "context done, flushing before exit")

			ctxTimeout, cancel := context.WithTimeout(context.Background(), finalFlushTimeout)
			defer cancel() //nolint:revive // Returning after this.

			//nolint:gocritic // System-level batch operation for connection logs.
			b.flush(dbauthz.AsConnectionLogger(ctxTimeout))
			return
		}
	}
}

// conflictKey represents the unique constraint columns used by
// the upsert query. Entries sharing the same key cannot appear
// in a single INSERT … ON CONFLICT DO UPDATE statement.
type conflictKey struct {
	ConnectionID uuid.UUID
	WorkspaceID  uuid.UUID
	AgentName    string
}

func (b *Batcher) flush(ctx context.Context) {
	count := len(b.batch)
	if count == 0 {
		return
	}

	b.log.Debug(ctx, "flushing connection log batch", slog.F("count", count))

	// Deduplicate by conflict key so PostgreSQL never sees the
	// same row twice in one INSERT … ON CONFLICT DO UPDATE.
	// Entries with a NULL connection_id (web events) are exempt
	// because NULL != NULL in SQL unique constraints.
	deduped := make(map[conflictKey]database.UpsertConnectionLogParams, count)
	var nullConnIDEntries []database.UpsertConnectionLogParams

	for _, item := range b.batch {
		if !item.ConnectionID.Valid {
			nullConnIDEntries = append(nullConnIDEntries, item)
			continue
		}
		key := conflictKey{
			ConnectionID: item.ConnectionID.UUID,
			WorkspaceID:  item.WorkspaceID,
			AgentName:    item.AgentName,
		}
		existing, ok := deduped[key]
		if !ok {
			deduped[key] = item
			continue
		}
		// Prefer disconnect over connect (superset of info).
		// If same status, prefer the later event.
		if item.ConnectionStatus == database.ConnectionStatusDisconnected &&
			existing.ConnectionStatus != database.ConnectionStatusDisconnected {
			deduped[key] = item
		} else if item.Time.After(existing.Time) {
			deduped[key] = item
		}
	}

	// Rebuild batch from deduplicated entries.
	items := make([]database.UpsertConnectionLogParams, 0, len(deduped)+len(nullConnIDEntries))
	for _, item := range deduped {
		items = append(items, item)
	}
	items = append(items, nullConnIDEntries...)

	dedupedCount := len(items)
	if dedupedCount < count {
		b.log.Debug(ctx, "deduplicated connection log batch",
			slog.F("original", count),
			slog.F("deduped", dedupedCount),
		)
	}

	var (
		ids              = make([]uuid.UUID, 0, dedupedCount)
		connectTime      = make([]time.Time, 0, dedupedCount)
		organizationID   = make([]uuid.UUID, 0, dedupedCount)
		workspaceOwnerID = make([]uuid.UUID, 0, dedupedCount)
		workspaceID      = make([]uuid.UUID, 0, dedupedCount)
		workspaceName    = make([]string, 0, dedupedCount)
		agentName        = make([]string, 0, dedupedCount)
		connType         = make([]database.ConnectionType, 0, dedupedCount)
		code             = make([]int32, 0, dedupedCount)
		ip               = make([]pqtype.Inet, 0, dedupedCount)
		userAgent        = make([]string, 0, dedupedCount)
		userID           = make([]uuid.UUID, 0, dedupedCount)
		slugOrPort       = make([]string, 0, dedupedCount)
		connectionID     = make([]uuid.UUID, 0, dedupedCount)
		disconnectReason = make([]string, 0, dedupedCount)
		disconnectTime   = make([]time.Time, 0, dedupedCount)
	)

	for _, item := range items {
		ids = append(ids, item.ID)
		connectTime = append(connectTime, item.Time)
		organizationID = append(organizationID, item.OrganizationID)
		workspaceOwnerID = append(workspaceOwnerID, item.WorkspaceOwnerID)
		workspaceID = append(workspaceID, item.WorkspaceID)
		workspaceName = append(workspaceName, item.WorkspaceName)
		agentName = append(agentName, item.AgentName)
		connType = append(connType, item.Type)
		code = append(code, nullInt32ToInt32(item.Code))
		ip = append(ip, item.Ip)
		userAgent = append(userAgent, nullStringToString(item.UserAgent))
		userID = append(userID, nullUUIDToUUID(item.UserID))
		slugOrPort = append(slugOrPort, nullStringToString(item.SlugOrPort))
		connectionID = append(connectionID, nullUUIDToUUID(item.ConnectionID))
		disconnectReason = append(disconnectReason, nullStringToString(item.DisconnectReason))
		// Pre-compute disconnect_time: if status is "disconnected",
		// use the event time; otherwise use zero time (epoch) which
		// the SQL CASE will treat as no disconnect.
		if item.ConnectionStatus == database.ConnectionStatusDisconnected {
			disconnectTime = append(disconnectTime, item.Time)
		} else {
			disconnectTime = append(disconnectTime, time.Time{})
		}
	}

	// Clear batch before the DB call. Losing a batch of connection
	// log entries is acceptable; the next event will be recorded.
	b.batch = make([]database.UpsertConnectionLogParams, 0, b.maxBatchSize)

	err := b.store.BatchUpsertConnectionLogs(ctx, database.BatchUpsertConnectionLogsParams{
		ID:               ids,
		ConnectTime:      connectTime,
		OrganizationID:   organizationID,
		WorkspaceOwnerID: workspaceOwnerID,
		WorkspaceID:      workspaceID,
		WorkspaceName:    workspaceName,
		AgentName:        agentName,
		Type:             connType,
		Code:             code,
		Ip:               ip,
		UserAgent:        userAgent,
		UserID:           userID,
		SlugOrPort:       slugOrPort,
		ConnectionID:     connectionID,
		DisconnectReason: disconnectReason,
		DisconnectTime:   disconnectTime,
	})
	if err != nil {
		if database.IsQueryCanceledError(err) {
			b.log.Debug(ctx, "query canceled, skipping connection log batch update")
			return
		}
		b.log.Error(ctx, "failed to batch upsert connection logs", slog.Error(err))
		return
	}

	b.log.Debug(ctx, "connection log batch flush complete", slog.F("count", count))
}

// nullStringToString converts a sql.NullString to a string. When the
// NullString is not valid, an empty string is returned.
func nullStringToString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// nullInt32ToInt32 converts a sql.NullInt32 to an int32. When the
// NullInt32 is not valid, zero is returned.
func nullInt32ToInt32(ni sql.NullInt32) int32 {
	if ni.Valid {
		return ni.Int32
	}
	return 0
}

// nullUUIDToUUID converts a uuid.NullUUID to a uuid.UUID. When the
// NullUUID is not valid, uuid.Nil is returned.
func nullUUIDToUUID(nu uuid.NullUUID) uuid.UUID {
	if nu.Valid {
		return nu.UUID
	}
	return uuid.Nil
}
