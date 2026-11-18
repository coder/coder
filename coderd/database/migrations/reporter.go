package migrations

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/lib/pq"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/quartz"
)

const (
	defaultHeartbeatInterval = 10 * time.Second
	// observerQueryTimeout bounds every query issued only for reporting, so a
	// struggling database can never make reporting hang or delay migrations.
	observerQueryTimeout = 2 * time.Second

	// phaseSetup is the short transaction that creates schema_migrations;
	// phaseBatch is the transaction that applies the pending migrations.
	phaseSetup = "setup"
	phaseBatch = "batch"

	stateRunning       = "running"
	stateWaitingOnLock = "waiting_on_lock"
	stateUnknown       = "unknown"
)

// Messages are constant so lines of one kind can be filtered; variable data
// such as the position of a step goes into fields.
const (
	msgAcquiringLock    = "acquiring database migration lock"
	msgAcquiredLock     = "acquired database migration lock"
	msgStillWaitingLock = "still waiting for database migration lock"
	msgUpToDate         = "database schema is up to date"
	msgRequired         = "database migration required"
	msgApplying         = "applying database migration"
	msgStillApplying    = "still applying database migration"
	msgApplied          = "database migration applied, pending commit"
	msgObserverFailed   = "database migration progress details unavailable"

	msgOutcomeNoChange               = "database migration outcome: no change"
	msgOutcomeCommitted              = "committed database migrations"
	msgOutcomeRolledBack             = "migration failed, transaction rolled back, no changes saved"
	msgOutcomeCommittedBeforeFailure = "committed database migrations before failure"
	msgOutcomeUnknown                = "database migration commit outcome unknown"
	msgOutcomeLockTimeout            = "gave up waiting for database migration lock"
	msgOutcomeDirty                  = "database schema is dirty"
	msgOutcomeFailedBeforeStart      = "database migration failed before it started"
)

type migrationRef struct {
	version int
	name    string
}

// reporter logs the progress of one migration run. Its own queries run on a
// separate connection and never change what the driver executes.
//
// The driver calls it one call at a time, so it has no mutex. Only heartbeats
// run concurrently, and they share nothing but observerFailed. A Lock that
// golang-migrate abandoned after LockTimeout can still call it after Up
// returns; cancel stops that Lock's heartbeat too.
type reporter struct {
	logger     slog.Logger
	appName    string
	clock      quartz.Clock
	interval   time.Duration
	observerDB *sql.DB
	source     source.Driver

	// cancel is called once the run has returned. It stops every heartbeat of
	// the run, including one an abandoned Lock starts later, without waiting.
	runCtx context.Context
	cancel context.CancelFunc

	// backendPID is written by Lock and read by stepStarted. The batch Lock
	// runs on golang-migrate's lock goroutine, which hands its result back
	// over a channel before any step starts.
	backendPID int
	// xid is the ID of the latest migration transaction, as text, kept to
	// check that transaction's outcome with pg_xact_status. It is reset on
	// every Lock, so after the batch transaction starts it identifies that
	// transaction.
	xid string
	// runStart is when the run started; upStart is when Up called m.Up, so
	// a lock timeout is measured from it. Both are set on Up's goroutine.
	runStart      time.Time
	upStart       time.Time
	fromVersion   int
	targetVersion int
	// started is set once the version was read inside the migration
	// transaction and the pending migrations are known. From then on the
	// batch Lock has returned, so no abandoned Lock writes xid.
	started     bool
	names       map[int]string
	total       int
	position    int
	stepVersion int
	// applied counts the migrations that ran in the transaction. They are
	// only saved if it commits.
	applied int
	// failed is the migration whose statement failed, if any.
	failed *failedMigration

	// hb is the heartbeat of the lock wait or of the current step. The two
	// never overlap: the lock wait ends inside Lock. Each gets its own
	// heartbeat.
	hb *heartbeat

	observerFailed atomic.Bool
}

// newReporter returns a reporter for one run. Without WithLogger it logs to
// a zero slog.Logger, which discards every line.
func newReporter(db *sql.DB, o upOptions) *reporter {
	var logger slog.Logger
	if o.logger != nil {
		logger = *o.logger
	}
	if o.appName == "" {
		o.appName = "coder-migrate/" + buildinfo.Version()
	}
	if o.clock == nil {
		o.clock = quartz.NewReal()
	}
	if o.heartbeatInterval <= 0 {
		o.heartbeatInterval = defaultHeartbeatInterval
	}
	if o.observerDB == nil {
		o.observerDB = db
	}
	runCtx, cancel := context.WithCancel(context.Background())
	return &reporter{
		logger:        logger,
		appName:       o.appName,
		clock:         o.clock,
		interval:      o.heartbeatInterval,
		observerDB:    o.observerDB,
		runCtx:        runCtx,
		cancel:        cancel,
		runStart:      o.clock.Now(),
		fromVersion:   database.NilVersion,
		targetVersion: database.NilVersion,
	}
}

// lockStarted is called at the start of every Lock, before any query.
func (r *reporter) lockStarted() {
	r.backendPID = 0
	r.xid = ""
}

// setBackend records the migration connection's backend PID and the
// migration transaction's ID.
func (r *reporter) setBackend(pid int, xid string) {
	r.backendPID = pid
	r.xid = xid
}

// lockWaitStarted is called right before Lock requests the migration lock.
// The heartbeat only appears if the wait lasts a full interval.
func (r *reporter) lockWaitStarted(phase string) {
	// An abandoned Lock can get here after cancel. Logging is skipped, and
	// the heartbeat below starts already stopped.
	if r.runCtx.Err() == nil {
		// Debug level: the lock is usually free, so this should not add a line
		// to every server start. A real wait shows up through the heartbeat.
		r.logger.Debug(r.runCtx, msgAcquiringLock,
			slog.F("phase", phase))
	}

	r.startHeartbeat(msgStillWaitingLock, r.backendPID)
}

// lockAcquired is called after Lock acquired the migration lock.
func (r *reporter) lockAcquired(phase string) {
	waited := r.stopHeartbeat()
	// An abandoned Lock that gets the lock after cancel logs nothing.
	if r.runCtx.Err() != nil {
		return
	}
	r.logger.Debug(r.runCtx, msgAcquiredLock,
		slog.F("phase", phase),
		slog.F("waited_ms", waited.Milliseconds()))
}

// versionRead is called once per run, after the lock is held and the current
// version has been read inside the migration transaction.
// It's not called for a dirty schema.
func (r *reporter) versionRead(version int) {
	if version != database.NilVersion {
		if _, ok := r.name(version); !ok {
			// The database is newer than this binary.
			// golang-migrate fails the run with its own error.
			return
		}
	}
	pending := r.pendingFrom(version)
	names := make(map[int]string, len(pending))
	for _, m := range pending {
		names[m.version] = m.name
	}

	r.started = true
	r.fromVersion = version
	r.targetVersion = version
	if len(pending) > 0 {
		r.targetVersion = pending[len(pending)-1].version
	}
	r.names = names
	r.total = len(pending)
	r.position = 0

	if len(pending) == 0 {
		// Debug level: most starts have nothing to migrate, and they should
		// not add a line to every server start.
		r.logger.Debug(r.runCtx, msgUpToDate, versionField("version", version)...)
		return
	}
	fields := versionField("from_version", version)
	fields = append(fields,
		slog.F("target_version", pending[len(pending)-1].version),
		slog.F("pending", len(pending)))
	r.logger.Info(r.runCtx, msgRequired, fields...)
}

// stepStarted is called after golang-migrate marked version as in progress,
// right before its SQL runs.
func (r *reporter) stepStarted(version int) {
	if version < 0 {
		return
	}
	r.position++
	r.stepVersion = version
	fields := r.stepFields(version)
	r.logStep(msgApplying, fields...)
	r.startHeartbeat(msgStillApplying, r.backendPID, fields...)
}

// stepApplied is called after golang-migrate marked version as clean. The
// change is not durable until the batch transaction commits.
func (r *reporter) stepApplied(version int) {
	if version < 0 {
		return
	}
	duration := r.stopHeartbeat()
	r.applied++

	fields := append(r.stepFields(version), slog.F("duration_ms", duration.Milliseconds()))
	r.logStep(msgApplied, fields...)
}

// statementFailed records which migration failed, how long it ran and the
// driver's error. It is called while the step's heartbeat still runs.
func (r *reporter) statementFailed(err error) {
	var duration time.Duration
	if r.hb != nil {
		duration = r.clock.Since(r.hb.since)
	}
	r.failed = &failedMigration{
		migrationRef: migrationRef{version: r.stepVersion, name: r.names[r.stepVersion]},
		duration:     duration,
		err:          err,
	}
}

// logStep logs a step line at info, or at debug when the run started from an
// empty database, so a fresh install that applies hundreds of migrations stays
// quiet.
func (r *reporter) logStep(msg string, fields ...slog.Field) {
	if r.fromVersion == database.NilVersion {
		r.logger.Debug(r.runCtx, msg, fields...)
		return
	}
	r.logger.Info(r.runCtx, msg, fields...)
}

// heartbeat logs the state of one migration backend every interval until it
// is stopped. Everything above mu is set at start and only read.
type heartbeat struct {
	logger slog.Logger
	clock  quartz.Clock
	db     *sql.DB
	pid    int
	msg    string
	fields []slog.Field
	since  time.Time
	cancel context.CancelFunc
	// observerFailed is shared by every heartbeat of the run, so only the
	// first failed check is logged.
	observerFailed *atomic.Bool

	// mu guards stopped and is held while a tick logs, so no line is logged
	// after stop returns.
	mu      sync.Mutex
	stopped bool
}

// stop never waits for a check in flight: canceling a lib/pq query still
// blocks until the server answers, which would delay the migration.
func (h *heartbeat) stop() {
	h.mu.Lock()
	h.stopped = true
	h.mu.Unlock()
	h.cancel()
}

// tick logs msg with elapsed_ms and the activity of the backend.
func (h *heartbeat) tick(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	activity, err := backendActivity(ctx, h.db, h.pid)

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopped || ctx.Err() != nil {
		// Stopped during the check: the step or wait is over, so the
		// result no longer describes anything.
		return context.Canceled
	}
	if err != nil && h.observerFailed.CompareAndSwap(false, true) {
		// Reporting failures never affect the migration.
		h.logger.Warn(ctx, msgObserverFailed, slog.Error(err))
	}
	line := append([]slog.Field{slog.F("elapsed_ms", h.clock.Since(h.since).Milliseconds())}, activity...)
	h.logger.Info(ctx, h.msg, append(line, h.fields...)...)
	return nil
}

// startHeartbeat replaces the current heartbeat with one for backend pid.
func (r *reporter) startHeartbeat(msg string, pid int, fields ...slog.Field) {
	r.stopHeartbeat()
	ctx, cancel := context.WithCancel(r.runCtx)
	h := &heartbeat{
		logger:         r.logger,
		clock:          r.clock,
		db:             r.observerDB,
		pid:            pid,
		msg:            msg,
		fields:         fields,
		since:          r.clock.Now(),
		cancel:         cancel,
		observerFailed: &r.observerFailed,
	}
	_ = r.clock.TickerFunc(ctx, r.interval, func() error {
		return h.tick(ctx)
	}, "migrations", "heartbeat")
	r.hb = h
}

// stopHeartbeat stops the current heartbeat, if any, and returns how long it ran.
func (r *reporter) stopHeartbeat() time.Duration {
	if r.hb == nil {
		return 0
	}
	r.hb.stop()
	d := r.clock.Since(r.hb.since)
	r.hb = nil
	return d
}

// backendActivity returns the state of backend pid as log fields (state,
// wait_event, blocked_by), read from pg_stat_activity and pg_blocking_pids.
func backendActivity(ctx context.Context, db *sql.DB, pid int) ([]slog.Field, error) {
	ctx, cancel := context.WithTimeout(ctx, observerQueryTimeout)
	defer cancel()

	var (
		waitType, waitEvent sql.NullString
		blockers            pq.Int64Array
	)
	err := db.QueryRowContext(ctx,
		`SELECT wait_event_type, wait_event, pg_blocking_pids(pid) FROM pg_stat_activity WHERE pid = $1`,
		pid).Scan(&waitType, &waitEvent, &blockers)
	if err != nil {
		return []slog.Field{slog.F("state", stateUnknown)}, err
	}
	if !waitType.Valid || waitType.String != "Lock" {
		return []slog.Field{slog.F("state", stateRunning)}, nil
	}
	fields := []slog.Field{slog.F("state", stateWaitingOnLock), slog.F("wait_event", waitEvent.String)}
	if len(blockers) > 0 {
		// Converted so it logs as a list of PIDs, not as pq.Int64Array's
		// PostgreSQL array literal.
		fields = append(fields, slog.F("blocked_by", []int64(blockers)))
	}
	return fields, nil
}

// pendingFrom lists the source's migrations after version, in order.
func (r *reporter) pendingFrom(version int) []migrationRef {
	if r.source == nil {
		return nil
	}
	var (
		next uint
		err  error
	)
	if version < 0 {
		next, err = r.source.First()
	} else {
		//nolint:gosec // Guarded by version >= 0 above.
		next, err = r.source.Next(uint(version))
	}
	var pending []migrationRef
	for err == nil {
		v := int(next) //nolint:gosec // Migration versions are small sequential numbers.
		name, _ := r.name(v)
		pending = append(pending, migrationRef{version: v, name: name})
		next, err = r.source.Next(next)
	}
	return pending
}

// name returns the identifier golang-migrate parses from the file name, for
// example "workspace_agent_session_counts" for
// 000590_workspace_agent_session_counts.up.sql.
// ok is false when the source has no such migration.
func (r *reporter) name(version int) (name string, ok bool) {
	if r.source == nil || version < 0 {
		return "", false
	}
	//nolint:gosec // Guarded by version >= 0 above.
	rc, identifier, err := r.source.ReadUp(uint(version))
	if err != nil {
		return "", false
	}
	_ = rc.Close()
	return identifier, true
}

// stepFields identifies the step for version in the current run.
func (r *reporter) stepFields(version int) []slog.Field {
	return []slog.Field{
		slog.F("version", version),
		slog.F("name", r.names[version]),
		slog.F("position", r.position),
		slog.F("total", r.total),
	}
}

// versionField returns the field, or nothing when there is no version yet.
// A missing version is never logged as -1.
func versionField(name string, version int) []slog.Field {
	if version == database.NilVersion {
		return nil
	}
	return []slog.Field{slog.F(name, version)}
}
