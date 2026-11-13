package migrations

import (
	"context"
	"database/sql"
	"sync"
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
)

type migrationRef struct {
	version int
	name    string
}

// reporter logs the progress of one migration run. Its own queries run on a
// separate connection and never change what the driver executes.
//
// It is called in order from the goroutine running Up, except by a Lock that
// golang-migrate abandoned after LockTimeout, which can still run after Up
// returns. Heartbeats derive from runCtx, so close stops all of them.
type reporter struct {
	logger     slog.Logger
	appName    string
	clock      quartz.Clock
	interval   time.Duration
	observerDB *sql.DB
	source     source.Driver

	// runCtx is canceled by close, which stops every heartbeat of the run.
	runCtx context.Context
	cancel context.CancelFunc

	mu         sync.Mutex
	backendPID int
	// xid is the ID of the latest migration transaction, as text, kept to
	// check that transaction's outcome with pg_xact_status. It is reset on
	// every Lock, so after the batch transaction starts it identifies that
	// transaction.
	xid         string
	fromVersion int
	names       map[int]string
	total       int
	position    int
	stepStart   time.Time

	stopStep func()

	observerFailed bool
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
		logger:      logger,
		appName:     o.appName,
		clock:       o.clock,
		interval:    o.heartbeatInterval,
		observerDB:  o.observerDB,
		runCtx:      runCtx,
		cancel:      cancel,
		fromVersion: database.NilVersion,
	}
}

// lockStarted is called at the start of every Lock, before any query.
func (r *reporter) lockStarted() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.backendPID = 0
	r.xid = ""
}

// setBackend records the migration connection's backend PID and the
// migration transaction's ID.
func (r *reporter) setBackend(pid int, xid string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.backendPID = pid
	r.xid = xid
}

// lockWaitStarted is called right before Lock requests the migration lock.
// Lock stops the returned heartbeat when the request returns. The heartbeat
// only appears if the wait lasts a full interval.
func (r *reporter) lockWaitStarted(phase string) (since time.Time, stop func()) {
	since = r.clock.Now()

	// An abandoned Lock can get here after close. Logging is skipped, and
	// the heartbeat below starts already stopped.
	if r.runCtx.Err() == nil {
		// Debug level: the lock is usually free, so this should not add a line
		// to every server start. A real wait shows up through the heartbeat.
		r.logger.Debug(r.runCtx, msgAcquiringLock,
			slog.F("phase", phase))
	}

	stop = r.startHeartbeat(func(ctx context.Context, activity []slog.Field) {
		fields := append([]slog.Field{slog.F("elapsed_ms", r.clock.Since(since).Milliseconds())}, activity...)
		r.logger.Info(ctx, msgStillWaitingLock, fields...)
	})
	return since, stop
}

// lockAcquired is called after Lock acquired the migration lock it started
// waiting for at since.
func (r *reporter) lockAcquired(phase string, since time.Time) {
	// An abandoned Lock that gets the lock after close logs nothing.
	if r.runCtx.Err() != nil {
		return
	}
	r.logger.Debug(r.runCtx, msgAcquiredLock,
		slog.F("phase", phase),
		slog.F("waited_ms", r.clock.Since(since).Milliseconds()))
}

// versionRead is called once per run, after the lock is held and the current
// version has been read inside the migration transaction.
// It's not called for a dirty schema.
func (r *reporter) versionRead(version int) {
	ctx := context.Background()
	if version != database.NilVersion && !r.inSource(version) {
		// The database is newer than this binary.
		// golang-migrate fails the run with its own error.
		return
	}
	pending := r.pendingFrom(version)
	names := make(map[int]string, len(pending))
	for _, m := range pending {
		names[m.version] = m.name
	}

	r.mu.Lock()
	r.fromVersion = version
	r.names = names
	r.total = len(pending)
	r.position = 0
	r.mu.Unlock()

	if len(pending) == 0 {
		// Debug level: most starts have nothing to migrate, and they should
		// not add a line to every server start.
		r.logger.Debug(ctx, msgUpToDate, versionField("version", version)...)
		return
	}
	fields := versionField("from_version", version)
	fields = append(fields,
		slog.F("target_version", pending[len(pending)-1].version),
		slog.F("pending", len(pending)))
	r.logger.Info(ctx, msgRequired, fields...)
}

// stepStarted is called after golang-migrate marked version as in progress,
// right before its SQL runs.
func (r *reporter) stepStarted(version int) {
	if version < 0 {
		return
	}
	now := r.clock.Now()

	r.mu.Lock()
	r.position++
	ref := migrationRef{version: version, name: r.names[version]}
	r.stepStart = now
	position, total := r.position, r.total
	from := r.fromVersion
	r.mu.Unlock()

	r.perStep(from, msgApplying, stepFields(ref, position, total)...)

	// The heartbeat stays at info on a fresh install:
	// it only appears for slow steps, and it is the liveness signal.
	stop := r.startHeartbeat(func(ctx context.Context, activity []slog.Field) {
		fields := append([]slog.Field{slog.F("elapsed_ms", r.clock.Since(now).Milliseconds())}, activity...)
		fields = append(fields, stepFields(ref, position, total)...)
		r.logger.Info(ctx, msgStillApplying, fields...)
	})

	r.mu.Lock()
	r.stopStep = stop
	r.mu.Unlock()
}

// stepApplied is called after golang-migrate marked version as clean. The
// change is not durable until the batch transaction commits.
func (r *reporter) stepApplied(version int) {
	if version < 0 {
		return
	}
	r.stopStepHeartbeat()

	r.mu.Lock()
	ref := migrationRef{version: version, name: r.names[version]}
	duration := r.clock.Since(r.stepStart)
	position, total := r.position, r.total
	from := r.fromVersion
	r.mu.Unlock()

	fields := stepFields(ref, position, total)
	fields = append(fields, slog.F("duration_ms", duration.Milliseconds()))
	r.perStep(from, msgApplied, fields...)
}

// close is called once the run has returned. It stops every heartbeat of the
// run, including one an abandoned Lock starts later. It does not wait for a
// heartbeat check that is in flight. Canceling under r.mu means no heartbeat
// line is written after close returns.
func (r *reporter) close() {
	r.mu.Lock()
	r.cancel()
	r.mu.Unlock()
}

// stopStepHeartbeat stops the step heartbeat, if one is running. Unlock calls
// it for a step that failed before stepApplied.
func (r *reporter) stopStepHeartbeat() {
	r.mu.Lock()
	stop := r.stopStep
	r.stopStep = nil
	r.mu.Unlock()

	if stop != nil {
		stop()
	}
}

// perStep logs per-step lines at debug level when the run started from an empty
// database, so a fresh install that applies hundreds of migrations stays quiet.
func (r *reporter) perStep(fromVersion int, msg string, fields ...slog.Field) {
	if fromVersion == database.NilVersion {
		r.logger.Debug(context.Background(), msg, fields...)
		return
	}
	r.logger.Info(context.Background(), msg, fields...)
}

// startHeartbeat calls emit with the migration connection's state every
// interval until stop is called or the run is closed.
//
// stop only cancels: it never waits for a check that is in flight, because
// canceling a lib/pq query still blocks until the server answers, and that
// would delay the migration. Instead, a tick re-checks its context and emits
// under r.mu, and stop cancels under r.mu, so no heartbeat line is written
// after stop returns. stop must not be called while holding r.mu.
func (r *reporter) startHeartbeat(emit func(ctx context.Context, activity []slog.Field)) (stop func()) {
	ctx, cancel := context.WithCancel(r.runCtx)
	_ = r.clock.TickerFunc(ctx, r.interval, func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		activity, err := r.backendActivity(ctx)

		r.mu.Lock()
		defer r.mu.Unlock()
		if ctx.Err() != nil {
			// Stopped during the check: the step or wait is over, so the
			// result no longer describes anything.
			return ctx.Err()
		}
		if err != nil {
			r.observerErrorLocked(ctx, err)
		}
		emit(ctx, activity)
		return nil
	}, "migrations", "heartbeat")

	return func() {
		r.mu.Lock()
		cancel()
		r.mu.Unlock()
	}
}

// backendActivity returns the state of the migration connection as log
// fields (state, wait_event, blocked_by), read from pg_stat_activity and
// pg_blocking_pids.
func (r *reporter) backendActivity(ctx context.Context) ([]slog.Field, error) {
	r.mu.Lock()
	pid := r.backendPID
	r.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, observerQueryTimeout)
	defer cancel()

	var (
		waitType, waitEvent sql.NullString
		blockers            pq.Int64Array
	)
	err := r.observerDB.QueryRowContext(ctx,
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

// observerErrorLocked logs the first failed reporting query only. Reporting
// failures never affect the migration. The caller holds r.mu.
func (r *reporter) observerErrorLocked(ctx context.Context, err error) {
	if r.observerFailed {
		return
	}
	r.observerFailed = true
	r.logger.Warn(ctx, msgObserverFailed, slog.Error(err))
}

// inSource reports whether version is one of the migrations in the source.
func (r *reporter) inSource(version int) bool {
	if r.source == nil || version < 0 {
		return false
	}
	//nolint:gosec // Guarded by version >= 0 above.
	rc, _, err := r.source.ReadUp(uint(version))
	if err != nil {
		return false
	}
	_ = rc.Close()
	return true
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
		pending = append(pending, migrationRef{version: v, name: r.name(v)})
		next, err = r.source.Next(next)
	}
	return pending
}

// name returns the identifier golang-migrate parses from the file name, for
// example "workspace_agent_session_counts" for
// 000590_workspace_agent_session_counts.up.sql.
func (r *reporter) name(version int) string {
	if r.source == nil || version < 0 {
		return ""
	}
	//nolint:gosec // Guarded by version >= 0 above.
	rc, identifier, err := r.source.ReadUp(uint(version))
	if err != nil {
		return ""
	}
	_ = rc.Close()
	return identifier
}

func stepFields(ref migrationRef, position, total int) []slog.Field {
	return []slog.Field{
		slog.F("version", ref.version),
		slog.F("name", ref.name),
		slog.F("position", position),
		slog.F("total", total),
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
