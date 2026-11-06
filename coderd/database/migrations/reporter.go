package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/lib/pq"
	"golang.org/x/xerrors"

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

type migrationRef struct {
	version int
	name    string
}

// reporter tracks one migration run and writes progress lines. The driver
// calls it at fixed moments (lock, version read, step start and end). It
// never changes what the driver executes: its own queries run on a separate
// pooled connection with a short timeout, against PostgreSQL system views.
// All methods are safe to call on a nil *reporter, which reports nothing.
type reporter struct {
	logger     slog.Logger
	appName    string
	clock      quartz.Clock
	interval   time.Duration
	observerDB *sql.DB
	source     source.Driver

	mu         sync.Mutex
	backendPID int
	// xid is the ID of the latest migration transaction, as text, kept to
	// check that transaction's outcome with pg_xact_status. It is reset on
	// every Lock, so after the batch transaction starts it identifies that
	// transaction.
	xid           string
	runStart      time.Time
	fromVersion   int
	targetVersion int
	// started is set once the version was read inside the migration
	// transaction and the pending migrations are known.
	started       bool
	names         map[int]string
	total         int
	position      int
	current       migrationRef
	stepStart     time.Time
	lockWaitStart time.Time
	// applied lists the migrations that ran in the transaction. They are
	// only saved if it commits.
	applied []appliedMigration
	// failed is the migration whose statement failed, if any.
	failed *failedMigration

	stopLockWait func()
	stopStep     func()

	// closed is set once the run has returned.
	closed         bool
	observerFailed bool
}

// newReporter returns nil, which reports nothing,
// unless WithLogger was given.
func newReporter(db *sql.DB, o upOptions) *reporter {
	if o.logger == nil {
		return nil
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
	return &reporter{
		logger:        *o.logger,
		appName:       o.appName,
		clock:         o.clock,
		interval:      o.heartbeatInterval,
		observerDB:    o.observerDB,
		runStart:      o.clock.Now(),
		fromVersion:   database.NilVersion,
		targetVersion: database.NilVersion,
	}
}

// lockStarted is called at the start of every Lock, before any query.
func (r *reporter) lockStarted() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.backendPID = 0
	r.xid = ""
}

// setBackend records the migration connection's backend PID and the
// migration transaction's ID.
func (r *reporter) setBackend(pid int, xid string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.backendPID = pid
	r.xid = xid
}

// lockWaitStarted is called when another session holds the migration lock.
func (r *reporter) lockWaitStarted(phase string) {
	if r == nil {
		return
	}
	since := r.clock.Now()

	r.mu.Lock()
	closed := r.closed
	r.lockWaitStart = since
	r.mu.Unlock()

	if closed {
		return
	}
	r.logger.Info(context.Background(), "waiting for database migration lock",
		slog.F("phase", phase))

	stop := r.startHeartbeat(func(ctx context.Context, activity []slog.Field) {
		fields := append([]slog.Field{slog.F("elapsed_ms", r.clock.Since(since).Milliseconds())}, activity...)
		r.logger.Info(ctx, "still waiting for database migration lock", fields...)
	})

	r.mu.Lock()
	if r.closed {
		// The run returned while the heartbeat started.
		r.mu.Unlock()
		stop()
		return
	}
	r.stopLockWait = stop
	r.mu.Unlock()
}

// stopLockWaitHeartbeat stops the lock wait heartbeat, whether or not the
// lock was acquired.
func (r *reporter) stopLockWaitHeartbeat() {
	if r == nil {
		return
	}

	r.mu.Lock()
	stop := r.stopLockWait
	r.stopLockWait = nil
	r.mu.Unlock()

	if stop != nil {
		stop()
	}
}

// lockAcquired is called after a wait for the migration lock succeeded.
func (r *reporter) lockAcquired(phase string) {
	if r == nil {
		return
	}

	r.mu.Lock()
	waited := r.clock.Since(r.lockWaitStart)
	closed := r.closed
	r.mu.Unlock()

	r.stopLockWaitHeartbeat()
	if closed {
		return
	}
	r.logger.Info(context.Background(), "acquired database migration lock",
		slog.F("phase", phase),
		slog.F("waited_ms", waited.Milliseconds()))
}

// versionRead is called once per run, after the lock is held and the current
// version has been read inside the migration transaction.
// It's not called for a dirty schema.
func (r *reporter) versionRead(version int) {
	if r == nil {
		return
	}
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
	r.started = true
	r.fromVersion = version
	r.targetVersion = version
	if len(pending) > 0 {
		r.targetVersion = pending[len(pending)-1].version
	}
	r.names = names
	r.total = len(pending)
	r.position = 0
	r.mu.Unlock()

	if len(pending) == 0 {
		// Debug level: most starts have nothing to migrate, and they should
		// not add a line to every server start.
		r.logger.Debug(ctx, "database schema is up to date", versionField("version", version)...)
		return
	}
	fields := versionField("from_version", version)
	fields = append(fields,
		slog.F("target_version", pending[len(pending)-1].version),
		slog.F("pending", len(pending)))
	r.logger.Info(ctx, "database migration required", fields...)
}

// stepStarted is called after golang-migrate marked version as in progress,
// right before its SQL runs.
func (r *reporter) stepStarted(version int) {
	if r == nil || version < 0 {
		return
	}
	now := r.clock.Now()

	r.mu.Lock()
	r.position++
	ref := migrationRef{version: version, name: r.names[version]}
	r.current = ref
	r.stepStart = now
	position, total := r.position, r.total
	from := r.fromVersion
	r.mu.Unlock()

	r.perStep(from, fmt.Sprintf("applying update %d of %d", position, total),
		stepFields(ref, position, total)...)

	// The heartbeat stays at info on a fresh install:
	// it only appears for slow steps, and it is the liveness signal.
	msg := fmt.Sprintf("still applying update %d of %d", position, total)
	stop := r.startHeartbeat(func(ctx context.Context, activity []slog.Field) {
		fields := append([]slog.Field{slog.F("elapsed_ms", r.clock.Since(now).Milliseconds())}, activity...)
		fields = append(fields, stepFields(ref, position, total)...)
		r.logger.Info(ctx, msg, fields...)
	})

	r.mu.Lock()
	r.stopStep = stop
	r.mu.Unlock()
}

// stepApplied is called after golang-migrate marked version as clean. The
// change is not durable until the batch transaction commits.
func (r *reporter) stepApplied(version int) {
	if r == nil || version < 0 {
		return
	}
	r.stopStepHeartbeat()

	r.mu.Lock()
	ref := migrationRef{version: version, name: r.names[version]}
	duration := r.clock.Since(r.stepStart)
	r.applied = append(r.applied, appliedMigration{migrationRef: ref, duration: duration})
	position, total := r.position, r.total
	from := r.fromVersion
	r.mu.Unlock()

	fields := stepFields(ref, position, total)
	fields = append(fields, slog.F("duration_ms", duration.Milliseconds()))
	r.perStep(from, "database migration applied, pending commit", fields...)
}

// statementFailed records which migration failed, how long it ran and
// PostgreSQL's error.
func (r *reporter) statementFailed(err error) {
	if r == nil {
		return
	}
	var pgErr *pq.Error
	_ = xerrors.As(err, &pgErr)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.failed = &failedMigration{
		migrationRef: r.current,
		duration:     r.clock.Since(r.stepStart),
		err:          err,
		pgErr:        pgErr,
	}
}

// stopHeartbeats stops every running heartbeat.
// Called before the migration transaction ends.
func (r *reporter) stopHeartbeats() {
	if r == nil {
		return
	}
	r.stopLockWaitHeartbeat()
	r.stopStepHeartbeat()
}

// close is called once the run has returned. It stops every heartbeat.
func (r *reporter) close() {
	if r == nil {
		return
	}

	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()

	r.stopHeartbeats()
}

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
// interval until stop is called. stop returns without waiting for a check
// that is in flight: canceling a query only asks PostgreSQL to abort it, and
// the call keeps blocking until the server answers, which would delay the
// migration. A check that finishes after stop is discarded, so no heartbeat
// line is written after stop returns.
// It must not be called while holding r.mu.
func (r *reporter) startHeartbeat(emit func(ctx context.Context, activity []slog.Field)) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	ticker := r.clock.NewTicker(r.interval, "migrations", "heartbeat")
	var (
		// emitMu makes stopping and writing a heartbeat line exclusive.
		emitMu  sync.Mutex
		stopped bool
	)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				activity, err := r.backendActivity(ctx)
				emitMu.Lock()
				if stopped {
					// The step or wait is over,
					// so the result no longer describes anything.
					emitMu.Unlock()
					return
				}
				if err != nil {
					r.observerError(ctx, err)
				}
				emit(ctx, activity)
				emitMu.Unlock()
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			emitMu.Lock()
			stopped = true
			emitMu.Unlock()
			cancel()
			ticker.Stop()
		})
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
	if waitType.String != "Lock" {
		return []slog.Field{slog.F("state", stateRunning)}, nil
	}
	fields := []slog.Field{slog.F("state", stateWaitingOnLock), slog.F("wait_event", waitEvent.String)}
	if len(blockers) > 0 {
		fields = append(fields, slog.F("blocked_by", formatPIDs(blockers)))
	}
	return fields, nil
}

// observerError logs the first failed reporting query only.
// Reporting failures never affect the migration.
func (r *reporter) observerError(ctx context.Context, err error) {
	r.mu.Lock()
	first := !r.observerFailed
	r.observerFailed = true
	r.mu.Unlock()

	if first {
		r.logger.Warn(ctx, "database migration progress details unavailable", slog.Error(err))
	}
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

// formatPIDs returns bare PIDs: "6652" or "6652,6653".
func formatPIDs(pids pq.Int64Array) string {
	parts := make([]string, len(pids))
	for i, p := range pids {
		parts[i] = strconv.FormatInt(p, 10)
	}
	return strings.Join(parts, ",")
}
