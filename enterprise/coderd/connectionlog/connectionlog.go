package connectionlog

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/sqlc-dev/pqtype"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	auditbackends "github.com/coder/coder/v2/enterprise/audit/backends"
	"github.com/coder/quartz"
)

const (
	// defaultBatchSize is the maximum number of connection log entries
	// to batch before forcing a flush.
	defaultBatchSize = 1000

	// defaultFlushInterval is how frequently to flush batched connection
	// log entries to the database. One second keeps audit logs near
	// real-time.
	defaultFlushInterval = time.Second

	// finalFlushTimeout is the timeout for the final flush when the
	// batcher is shutting down.
	finalFlushTimeout = 15 * time.Second
)

// Backend is a destination for connection log events. Backends that
// also implement io.Closer will be closed when the ConnectionLogger
// is closed.
type Backend interface {
	Upsert(ctx context.Context, clog database.UpsertConnectionLogParams) error
}

// ConnectionLogger fans out each connection log event to every
// registered backend.
type ConnectionLogger struct {
	backends []Backend
}

// New creates a ConnectionLogger that dispatches to the given
// backends.
func New(backends ...Backend) *ConnectionLogger {
	return &ConnectionLogger{
		backends: backends,
	}
}

func (c *ConnectionLogger) Upsert(ctx context.Context, clog database.UpsertConnectionLogParams) error {
	var errs error
	for _, backend := range c.backends {
		err := backend.Upsert(ctx, clog)
		if err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	return errs
}

// Close closes all backends that implement io.Closer.
func (c *ConnectionLogger) Close() error {
	var errs error
	for _, backend := range c.backends {
		if closer, ok := backend.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				errs = multierror.Append(errs, err)
			}
		}
	}
	return errs
}

// DBBatcherOption is a functional option for configuring a DBBatcher.
type DBBatcherOption func(b *DBBatcher)

// WithBatchSize sets the maximum number of entries to accumulate
// before forcing a flush.
func WithBatchSize(size int) DBBatcherOption {
	return func(b *DBBatcher) {
		b.maxBatchSize = size
	}
}

// WithFlushInterval sets how frequently the batcher flushes to the
// database.
func WithFlushInterval(d time.Duration) DBBatcherOption {
	return func(b *DBBatcher) {
		b.interval = d
	}
}

// WithClock sets the clock, useful for testing.
func WithClock(clock quartz.Clock) DBBatcherOption {
	return func(b *DBBatcher) {
		b.clock = clock
	}
}

// DBBatcher batches connection log upserts and periodically flushes
// them to the database to reduce per-event write pressure.
type DBBatcher struct {
	store database.Store
	log   slog.Logger

	itemCh chan database.UpsertConnectionLogParams

	// dedupedBatch holds entries keyed by their conflict columns so
	// that PostgreSQL never sees the same row twice in one INSERT …
	// ON CONFLICT DO UPDATE. Entries with a NULL connection_id (web
	// events) go into nullConnIDBatch instead because NULL != NULL
	// in SQL unique constraints.
	dedupedBatch    map[conflictKey]database.UpsertConnectionLogParams
	nullConnIDBatch []database.UpsertConnectionLogParams
	maxBatchSize    int

	clock    quartz.Clock
	timer    *quartz.Timer
	interval time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// NewDBBatcher creates a DBBatcher that batches writes to the database
// and starts its background processing loop. Close must be called to
// flush remaining entries on shutdown.
func NewDBBatcher(ctx context.Context, store database.Store, log slog.Logger, opts ...DBBatcherOption) *DBBatcher {
	b := &DBBatcher{
		store: store,
		log:   log,
		done:  make(chan struct{}),
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
	b.itemCh = make(chan database.UpsertConnectionLogParams, b.maxBatchSize)
	b.dedupedBatch = make(map[conflictKey]database.UpsertConnectionLogParams, b.maxBatchSize)

	b.ctx, b.cancel = context.WithCancel(ctx)
	go func() {
		b.run(b.ctx)
		close(b.done)
	}()

	return b
}

// Upsert enqueues a connection log entry for batched writing. It
// blocks if the internal buffer is full, ensuring no logs are dropped.
// It returns immediately if the batcher's context is done.
func (b *DBBatcher) Upsert(ctx context.Context, clog database.UpsertConnectionLogParams) error {
	select {
	case b.itemCh <- clog:
		return nil
	case <-b.ctx.Done():
		return b.ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close cancels the batcher context and waits for the final flush to
// complete.
func (b *DBBatcher) Close() error {
	b.cancel()
	if b.timer != nil {
		b.timer.Stop()
	}
	<-b.done
	return nil
}

// addToBatch inserts an item into the batch, deduplicating by conflict
// key on the fly. For entries with the same key, disconnect events are
// preferred over connect events, and later events are preferred over
// earlier ones.
//
// This is safe because each new connection gets a fresh UUID (see
// agent/agent.go and agent/agentssh), so the only duplicate for the
// same (connection_id, workspace_id, agent_name) is a connect/disconnect
// pair for the same session. A "reconnect" always uses a new ID.
func (b *DBBatcher) addToBatch(item database.UpsertConnectionLogParams) {
	if !item.ConnectionID.Valid {
		b.nullConnIDBatch = append(b.nullConnIDBatch, item)
		return
	}
	key := conflictKey{
		ConnectionID: item.ConnectionID.UUID,
		WorkspaceID:  item.WorkspaceID,
		AgentName:    item.AgentName,
	}
	existing, ok := b.dedupedBatch[key]
	if !ok {
		b.dedupedBatch[key] = item
		return
	}
	// Prefer disconnect over connect (superset of info).
	// If same status, prefer the later event.
	if item.ConnectionStatus == database.ConnectionStatusDisconnected &&
		existing.ConnectionStatus != database.ConnectionStatusDisconnected {
		b.dedupedBatch[key] = item
	} else if item.Time.After(existing.Time) {
		b.dedupedBatch[key] = item
	}
}

// batchLen returns the total number of entries currently buffered.
func (b *DBBatcher) batchLen() int {
	return len(b.dedupedBatch) + len(b.nullConnIDBatch)
}

func (b *DBBatcher) run(ctx context.Context) {
	//nolint:gocritic // System-level batch operation for connection logs.
	authCtx := dbauthz.AsConnectionLogger(ctx)
	for ctx.Err() == nil {
		select {
		case item := <-b.itemCh:
			b.addToBatch(item)

			if b.batchLen() >= b.maxBatchSize {
				b.flush(authCtx)
				b.timer.Reset(b.interval, "connectionLogBatcher", "capacityFlush")
			}

		case <-b.timer.C:
			b.flush(authCtx)
			b.timer.Reset(b.interval, "connectionLogBatcher", "scheduledFlush")

		case <-ctx.Done():
		}
	}

	b.log.Debug(ctx, "context done, flushing before exit")

	// Drain any remaining items from the channel.
	for {
		select {
		case item := <-b.itemCh:
			b.addToBatch(item)
		default:
			ctxTimeout, cancel := context.WithTimeout(context.Background(), finalFlushTimeout)
			defer cancel() //nolint:revive // Returning after this.

			//nolint:gocritic // System-level batch operation for connection logs.
			b.flush(dbauthz.AsConnectionLogger(ctxTimeout))
			return
		}
	}
}

// conflictKey represents the unique constraint columns used by the
// upsert query. Entries sharing the same key cannot appear in a single
// INSERT … ON CONFLICT DO UPDATE statement.
type conflictKey struct {
	ConnectionID uuid.UUID
	WorkspaceID  uuid.UUID
	AgentName    string
}

func (b *DBBatcher) flush(ctx context.Context) {
	count := b.batchLen()
	if count == 0 {
		return
	}

	b.log.Debug(ctx, "flushing connection log batch", slog.F("count", count))

	var (
		ids              = make([]uuid.UUID, 0, count)
		connectTime      = make([]time.Time, 0, count)
		organizationID   = make([]uuid.UUID, 0, count)
		workspaceOwnerID = make([]uuid.UUID, 0, count)
		workspaceID      = make([]uuid.UUID, 0, count)
		workspaceName    = make([]string, 0, count)
		agentName        = make([]string, 0, count)
		connType         = make([]database.ConnectionType, 0, count)
		code             = make([]int32, 0, count)
		ip               = make([]pqtype.Inet, 0, count)
		userAgent        = make([]string, 0, count)
		userID           = make([]uuid.UUID, 0, count)
		slugOrPort       = make([]string, 0, count)
		connectionID     = make([]uuid.UUID, 0, count)
		disconnectReason = make([]string, 0, count)
		disconnectTime   = make([]time.Time, 0, count)
	)

	appendItem := func(item database.UpsertConnectionLogParams) {
		ids = append(ids, item.ID)
		connectTime = append(connectTime, item.Time)
		organizationID = append(organizationID, item.OrganizationID)
		workspaceOwnerID = append(workspaceOwnerID, item.WorkspaceOwnerID)
		workspaceID = append(workspaceID, item.WorkspaceID)
		workspaceName = append(workspaceName, item.WorkspaceName)
		agentName = append(agentName, item.AgentName)
		connType = append(connType, item.Type)
		code = append(code, item.Code.Int32)
		ip = append(ip, item.Ip)
		userAgent = append(userAgent, item.UserAgent.String)
		userID = append(userID, item.UserID.UUID)
		slugOrPort = append(slugOrPort, item.SlugOrPort.String)
		connectionID = append(connectionID, item.ConnectionID.UUID)
		disconnectReason = append(disconnectReason, item.DisconnectReason.String)
		// Pre-compute disconnect_time: if status is "disconnected",
		// use the event time; otherwise use zero time (epoch) which
		// the SQL NULLIF will treat as NULL.
		if item.ConnectionStatus == database.ConnectionStatusDisconnected {
			disconnectTime = append(disconnectTime, item.Time)
		} else {
			disconnectTime = append(disconnectTime, time.Time{})
		}
	}

	for _, item := range b.dedupedBatch {
		appendItem(item)
	}
	for _, item := range b.nullConnIDBatch {
		appendItem(item)
	}

	// Clear batches before the DB call so we can start accumulating
	// new entries immediately.
	clear(b.dedupedBatch)
	b.nullConnIDBatch = b.nullConnIDBatch[:0]

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

type connectionSlogBackend struct {
	exporter *auditbackends.SlogExporter
}

// NewSlogBackend returns a Backend that logs connection events via
// the structured logger.
func NewSlogBackend(logger slog.Logger) Backend {
	return &connectionSlogBackend{
		exporter: auditbackends.NewSlogExporter(logger),
	}
}

func (b *connectionSlogBackend) Upsert(ctx context.Context, clog database.UpsertConnectionLogParams) error {
	return b.exporter.ExportStruct(ctx, clog, "connection_log")
}
