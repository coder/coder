package connectionlog

import (
	"context"
	"io"
	"sync"
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
	// log entries to the database. Five seconds balances near-real-time
	// audit visibility with write efficiency.
	defaultFlushInterval = 5 * time.Second

	// retryQueueSize is the capacity of the bounded retry channel.
	// Failed batches beyond this limit are dropped.
	retryQueueSize = 10

	// shutdownWriteTimeout bounds how long a final write attempt
	// can take during shutdown when the batcher context is already
	// canceled.
	shutdownWriteTimeout = 10 * time.Second

	// maxRetries is the number of times to retry a failed batch
	// write before dropping it and moving on.
	maxRetries = 3

	// retryInterval is the fixed delay between retry attempts.
	retryInterval = time.Second
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

	// dedupedBatch holds entries keyed by connection ID so that
	// PostgreSQL never sees the same row twice in one INSERT …
	// ON CONFLICT DO UPDATE. Connection IDs are globally unique
	// (each new session gets a fresh UUID). Entries with a NULL
	// connection_id (web events) go into nullConnIDBatch instead
	// because NULL != NULL in SQL unique constraints.
	dedupedBatch    map[uuid.UUID]batchEntry
	nullConnIDBatch []batchEntry
	maxBatchSize    int

	// retryCh is a bounded channel of failed batches awaiting
	// retry. A single retry worker goroutine processes this
	// channel, retrying each batch up to maxRetries times before
	// dropping it. If the channel is full, new failures are
	// dropped immediately.
	retryCh chan database.BatchUpsertConnectionLogsParams

	clock    quartz.Clock
	timer    *quartz.Timer
	interval time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewDBBatcher creates a DBBatcher that batches writes to the database
// and starts its background processing loop. Close must be called to
// flush remaining entries on shutdown.
func NewDBBatcher(ctx context.Context, store database.Store, log slog.Logger, opts ...DBBatcherOption) *DBBatcher {
	b := &DBBatcher{
		store: store,
		log:   log,
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
	b.dedupedBatch = make(map[uuid.UUID]batchEntry, b.maxBatchSize)
	b.retryCh = make(chan database.BatchUpsertConnectionLogsParams, retryQueueSize)

	b.ctx, b.cancel = context.WithCancel(ctx)
	b.wg.Add(2)
	go func() {
		defer b.wg.Done()
		b.run(b.ctx)
	}()
	go func() {
		defer b.wg.Done()
		b.retryLoop()
	}()

	return b
}

// Upsert enqueues a connection log entry for batched writing. It
// blocks if the internal buffer is full, ensuring no logs are dropped.
// It returns an error if the batcher or caller context is canceled.
func (b *DBBatcher) Upsert(ctx context.Context, clog database.UpsertConnectionLogParams) error {
	if b.ctx.Err() != nil {
		return b.ctx.Err()
	}

	select {
	case b.itemCh <- clog:
		return nil
	case <-b.ctx.Done():
		return b.ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close cancels the batcher context, waits for the run loop and
// retry worker to exit.
func (b *DBBatcher) Close() error {
	b.cancel()
	if b.timer != nil {
		b.timer.Stop()
	}
	b.wg.Wait()
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
	entry := batchEntry{
		UpsertConnectionLogParams: item,
	}
	if item.ConnectionStatus == database.ConnectionStatusDisconnected {
		// For standalone disconnect events, use the disconnect
		// time as both connect and disconnect time. This matches
		// the single-row UpsertConnectionLog behavior which uses
		// @time for connect_time regardless of status. The SQL
		// LEAST logic will correct connect_time if the real
		// connect event arrives in a later batch.
		entry.connectTime = item.Time
		entry.disconnectTime = item.Time
	} else {
		entry.connectTime = item.Time
	}

	if !item.ConnectionID.Valid {
		b.nullConnIDBatch = append(b.nullConnIDBatch, entry)
		return
	}
	connID := item.ConnectionID.UUID
	existing, ok := b.dedupedBatch[connID]
	if !ok {
		b.dedupedBatch[connID] = entry
		return
	}
	// When merging entries for the same connection, always preserve
	// the earliest non-zero connect_time and latest disconnect_time
	// so the row records the full session span.
	if !existing.connectTime.IsZero() && existing.connectTime.Before(entry.connectTime) {
		entry.connectTime = existing.connectTime
	}
	if existing.disconnectTime.After(entry.disconnectTime) {
		entry.disconnectTime = existing.disconnectTime
	}

	// Prefer disconnect over connect (superset of info).
	// If same status, prefer the later event.
	if item.ConnectionStatus == database.ConnectionStatusDisconnected &&
		existing.ConnectionStatus != database.ConnectionStatusDisconnected {
		b.dedupedBatch[connID] = entry
	} else if item.Time.After(existing.Time) {
		b.dedupedBatch[connID] = entry
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
			if b.batchLen() > 0 {
				b.shutdownBatch(b.buildParams())
			}
			// Signal the retry worker to skip delays and close
			// the channel so it exits after processing any
			// remaining items.
			// Mark the batcher as closed so that any subsequent
			// Upsert calls fail immediately instead of sending
			// into itemCh after the run loop has exited.
			close(b.retryCh)
			return
		}
	}
}

// batchEntry wraps a connection log event with explicit connect and
// disconnect times. When a connect and disconnect for the same session
// are merged into one entry, connectTime preserves the original
// session start while disconnectTime records when it ended.
type batchEntry struct {
	database.UpsertConnectionLogParams
	connectTime    time.Time
	disconnectTime time.Time
}

// flush builds the batch params, clears the in-memory batch, and
// writes to the database. On failure, the batch is queued for retry
// by the single retry worker goroutine. If the retry queue is full,
// the batch is dropped.
func (b *DBBatcher) flush(ctx context.Context) {
	count := b.batchLen()
	if count == 0 {
		return
	}

	params := b.buildParams()

	// Clear the batch before writing so the run loop can start
	// accumulating new entries.
	b.dedupedBatch = make(map[uuid.UUID]batchEntry, b.maxBatchSize)
	b.nullConnIDBatch = nil

	// Use the batcher's context for normal operation so Close()
	// can cancel hung writes. During shutdown (ctx already canceled),
	// fall back to a bounded timeout.
	writeCtx := b.ctx
	if writeCtx.Err() != nil {
		var cancel context.CancelFunc
		writeCtx, cancel = context.WithTimeout(context.Background(), shutdownWriteTimeout)
		defer cancel()
	}
	//nolint:gocritic // System-level batch operation for connection logs.
	err := b.store.BatchUpsertConnectionLogs(dbauthz.AsConnectionLogger(writeCtx), params)
	if err == nil {
		return
	}

	b.log.Error(ctx, "batch upsert failed, queueing for retry",
		slog.Error(err), slog.F("count", count))

	// Don't retry on shutdown.
	if ctx.Err() != nil {
		return
	}

	select {
	case b.retryCh <- params:
	default:
		b.log.Error(ctx, "retry queue full, dropping batch",
			slog.F("dropped", count))
	}
}

func (b *DBBatcher) buildParams() database.BatchUpsertConnectionLogsParams {
	count := b.batchLen()
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
		codeValid        = make([]bool, 0, count)
		ip               = make([]pqtype.Inet, 0, count)
		userAgent        = make([]string, 0, count)
		userID           = make([]uuid.UUID, 0, count)
		slugOrPort       = make([]string, 0, count)
		connectionID     = make([]uuid.UUID, 0, count)
		disconnectReason = make([]string, 0, count)
		disconnectTime   = make([]time.Time, 0, count)
	)

	appendEntry := func(e batchEntry) {
		ids = append(ids, e.ID)
		connectTime = append(connectTime, e.connectTime)
		organizationID = append(organizationID, e.OrganizationID)
		workspaceOwnerID = append(workspaceOwnerID, e.WorkspaceOwnerID)
		workspaceID = append(workspaceID, e.WorkspaceID)
		workspaceName = append(workspaceName, e.WorkspaceName)
		agentName = append(agentName, e.AgentName)
		connType = append(connType, e.Type)
		code = append(code, e.Code.Int32)
		codeValid = append(codeValid, e.Code.Valid)
		ip = append(ip, e.IP)
		userAgent = append(userAgent, e.UserAgent.String)
		userID = append(userID, e.UserID.UUID)
		slugOrPort = append(slugOrPort, e.SlugOrPort.String)
		connectionID = append(connectionID, e.ConnectionID.UUID)
		disconnectReason = append(disconnectReason, e.DisconnectReason.String)
		disconnectTime = append(disconnectTime, e.disconnectTime)
	}

	for _, entry := range b.dedupedBatch {
		appendEntry(entry)
	}
	for _, entry := range b.nullConnIDBatch {
		appendEntry(entry)
	}

	return database.BatchUpsertConnectionLogsParams{
		ID:               ids,
		ConnectTime:      connectTime,
		OrganizationID:   organizationID,
		WorkspaceOwnerID: workspaceOwnerID,
		WorkspaceID:      workspaceID,
		WorkspaceName:    workspaceName,
		AgentName:        agentName,
		Type:             connType,
		Code:             code,
		CodeValid:        codeValid,
		Ip:               ip,
		UserAgent:        userAgent,
		UserID:           userID,
		SlugOrPort:       slugOrPort,
		ConnectionID:     connectionID,
		DisconnectReason: disconnectReason,
		DisconnectTime:   disconnectTime,
	}
}

// retryLoop is a single background goroutine that processes failed
// batches from retryCh. Each batch is retried up to maxRetries times
// with a fixed delay between attempts. When draining is set (shutdown),
// batches get a single immediate write attempt instead. The loop exits
// when retryCh is closed by the run goroutine.
func (b *DBBatcher) retryLoop() {
	for params := range b.retryCh {
		b.retryBatch(params)
	}
}

// retryBatch retries writing a batch up to maxRetries times with a
// fixed delay between attempts. If the batcher context is canceled
// during a wait, one final attempt is made before returning.
func (b *DBBatcher) retryBatch(params database.BatchUpsertConnectionLogsParams) {
	count := len(params.ID)
	for attempt := range maxRetries {
		t := time.NewTimer(retryInterval)
		select {
		case <-b.ctx.Done():
			b.shutdownBatch(params)
			return
		case <-t.C:
		}

		//nolint:gocritic // System-level batch operation for connection logs.
		err := b.store.BatchUpsertConnectionLogs(dbauthz.AsConnectionLogger(b.ctx), params)
		if err == nil {
			return
		}

		b.log.Warn(b.ctx, "batch retry failed",
			slog.Error(err),
			slog.F("count", count),
			slog.F("attempt", attempt+1),
			slog.F("max_attempts", maxRetries),
		)
	}

	b.log.Error(b.ctx, "batch retries exhausted, dropping batch",
		slog.F("dropped", count))
}

// shutdownBatch makes a single write attempt during shutdown with a
// bounded timeout so it can't hang indefinitely.
func (b *DBBatcher) shutdownBatch(params database.BatchUpsertConnectionLogsParams) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownWriteTimeout)
	defer cancel()
	//nolint:gocritic // System-level batch operation for connection logs.
	err := b.store.BatchUpsertConnectionLogs(dbauthz.AsConnectionLogger(ctx), params)
	if err != nil {
		b.log.Error(b.ctx, "batch write failed on shutdown, dropping batch",
			slog.Error(err), slog.F("dropped", len(params.ID)))
	}
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
