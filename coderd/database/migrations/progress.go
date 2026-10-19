package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/lib/pq"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"
)

const (
	defaultHeartbeatInterval = 10 * time.Second
	// observerQueryTimeout bounds every query issued only for reporting, so a
	// struggling database can never make reporting hang or delay migrations.
	observerQueryTimeout = 2 * time.Second
)

// ReporterOptions configures the operator-facing progress and outcome lines
// written by UpWithReporter.
type ReporterOptions struct {
	// Logger receives the lines. By convention it is named "migrations" so
	// operators can filter them from shared logs.
	Logger slog.Logger
	// AppName is set as application_name on the migration connection, so it
	// can be identified in pg_stat_activity by operators and other replicas.
	AppName string
	// Clock drives the heartbeat and timings. Defaults to the real clock.
	Clock quartz.Clock
	// HeartbeatInterval is how often a long lock wait or migration reports
	// that it is still alive. Defaults to 10 seconds.
	HeartbeatInterval time.Duration
	// FS overrides the embedded migrations. Only tests set it.
	FS fs.FS
}

type migrationRef struct {
	version int
	name    string
}

func (m migrationRef) String() string {
	return fmt.Sprintf("%06d_%s", m.version, m.name)
}

type appliedMigration struct {
	migrationRef
	duration time.Duration
}

// reporter tracks one migration run in process and writes progress lines.
// It never changes what the migration driver executes: every query it issues
// runs on a separate pooled connection with a short timeout, against
// PostgreSQL system views or after the migration transaction has ended.
// All methods are safe to call on a nil *reporter, which reports nothing.
type reporter struct {
	logger   slog.Logger
	appName  string
	clock    quartz.Clock
	interval time.Duration
	db       *sql.DB
	source   source.Driver

	mu             sync.Mutex
	backendPID     int
	xid            string // transaction ID of the migration run, as text
	fromVersion    int
	target         int
	total          int
	runStart       time.Time
	waited         time.Duration
	current        *migrationRef
	currentStart   time.Time
	applied        []appliedMigration
	pgErr          *pq.Error
	stmtErr        error
	stopHeartbeat  func()
	observerFailed bool
}

func newReporter(db *sql.DB, opts ReporterOptions) *reporter {
	if opts.Clock == nil {
		opts.Clock = quartz.NewReal()
	}
	if opts.HeartbeatInterval <= 0 {
		opts.HeartbeatInterval = defaultHeartbeatInterval
	}
	return &reporter{
		logger:      opts.Logger,
		appName:     opts.AppName,
		clock:       opts.Clock,
		interval:    opts.HeartbeatInterval,
		db:          db,
		fromVersion: database.NilVersion,
	}
}

// lockWaitStarted is called when another instance holds the migration lock.
// It starts a heartbeat that reports who holds it.
func (r *reporter) lockWaitStarted(backendPID int) (stop func()) {
	if r == nil {
		return func() {}
	}
	r.logger.Info(context.Background(), "database migration waiting for migration lock held by another instance",
		slog.F("backend_pid", backendPID))
	return r.startHeartbeat(backendPID, r.clock.Now(), nil)
}

func (r *reporter) lockAcquired(waited time.Duration) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.waited += waited
	r.mu.Unlock()
	r.logger.Info(context.Background(), "database migration lock acquired",
		slog.F("waited", waited.Truncate(time.Millisecond)))
}

func (r *reporter) setBackend(pid int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.backendPID = pid
	r.mu.Unlock()
}

// inventory is called once per run, right after the lock is acquired and the
// current version has been read inside the migration transaction.
func (r *reporter) inventory(version int, dirty bool, xid string) {
	if r == nil {
		return
	}
	pending := r.pendingFrom(version)

	r.mu.Lock()
	r.fromVersion = version
	r.xid = xid
	r.total = len(pending)
	r.target = version
	if len(pending) > 0 {
		r.target = pending[len(pending)-1].version
	}
	r.runStart = r.clock.Now()
	waited := r.waited
	pid := r.backendPID
	r.mu.Unlock()

	ctx := context.Background()
	switch {
	case dirty:
		r.logger.Error(ctx, "database migration cannot start: schema is marked dirty",
			slog.F("schema_version", version))
	case len(pending) == 0:
		fields := []slog.Field{slog.F("schema_version", version)}
		if waited > 0 {
			// Another instance migrated the schema while this one waited.
			fields = append(fields, slog.F("waited", waited.Truncate(time.Millisecond)))
		}
		r.logger.Info(ctx, "database schema is up to date", fields...)
	default:
		r.logger.Info(ctx, "database migration required",
			slog.F("from_version", displayVersion(version)),
			slog.F("target_version", pending[len(pending)-1].version),
			slog.F("pending", len(pending)),
			slog.F("backend_pid", pid),
			slog.F("transaction_id", xid))
	}
}

func (r *reporter) migrationStarted(version int) {
	if r == nil || version < 0 {
		return
	}
	ref := migrationRef{version: version, name: r.name(version)}

	r.mu.Lock()
	r.current = &ref
	r.currentStart = r.clock.Now()
	position := len(r.applied) + 1
	total := r.total
	pid := r.backendPID
	r.mu.Unlock()

	r.perMigration("database migration applying migration",
		slog.F("migration", ref.String()), slog.F("position", fmt.Sprintf("%d/%d", position, total)))

	stop := r.startHeartbeat(pid, r.clock.Now(), &ref)
	r.mu.Lock()
	r.stopHeartbeat = stop
	r.mu.Unlock()
}

func (r *reporter) migrationApplied(version int) {
	if r == nil || version < 0 {
		return
	}
	r.stopRunningHeartbeat()

	r.mu.Lock()
	ref := migrationRef{version: version, name: r.name(version)}
	duration := r.clock.Since(r.currentStart)
	r.applied = append(r.applied, appliedMigration{migrationRef: ref, duration: duration})
	position := len(r.applied)
	total := r.total
	r.current = nil
	r.mu.Unlock()

	r.perMigration("database migration applied migration, pending commit",
		slog.F("migration", ref.String()), slog.F("position", fmt.Sprintf("%d/%d", position, total)),
		slog.F("duration", duration.Truncate(time.Millisecond)))
}

func (r *reporter) statementFailed(err error) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stmtErr = err
	var pgErr *pq.Error
	if xerrors.As(err, &pgErr) {
		r.pgErr = pgErr
	}
}

func (r *reporter) stopRunningHeartbeat() {
	if r == nil {
		return
	}
	r.mu.Lock()
	stop := r.stopHeartbeat
	r.stopHeartbeat = nil
	r.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// perMigration logs per-migration lines at DEBUG on an empty database, so a
// first install that applies hundreds of migrations is not noisy, and at
// INFO for upgrades.
func (r *reporter) perMigration(msg string, fields ...slog.Field) {
	r.mu.Lock()
	empty := r.fromVersion == database.NilVersion
	r.mu.Unlock()
	if empty {
		r.logger.Debug(context.Background(), msg, fields...)
		return
	}
	r.logger.Info(context.Background(), msg, fields...)
}

// finish logs how the run ended and returns the error the caller should see.
// The outcome is read from PostgreSQL: the commit status of the migration
// transaction and the schema version that is now durable.
func (r *reporter) finish(upErr error) error {
	if r == nil {
		return upErr
	}
	r.stopRunningHeartbeat()

	r.mu.Lock()
	from, target, total := r.fromVersion, r.target, r.total
	xid := r.xid
	current := r.current
	applied := append([]appliedMigration(nil), r.applied...)
	pgErr, stmtErr := r.pgErr, r.stmtErr
	runStart := r.runStart
	r.mu.Unlock()

	ctx := context.Background()
	if current == nil && len(applied) == 0 {
		// No migration started: up to date, dirty, or a failure before the
		// first migration (for example a lock timeout).
		if upErr != nil {
			r.logger.Error(ctx, "database migration failed before applying any migration", slog.Error(upErr))
		}
		return upErr
	}

	if upErr != nil {
		r.logFailure(ctx, upErr, current, stmtErr, pgErr, len(applied), total)
	}

	status, statusErr := r.transactionStatus(xid)
	saved, savedErr := r.savedVersion()
	elapsed := r.clock.Since(runStart).Truncate(time.Millisecond)
	outcomeFields := []slog.Field{
		slog.F("transaction_id", xid),
		slog.F("transaction_status", status),
		slog.F("schema_version", saved),
		slog.F("duration", elapsed),
	}
	if statusErr != nil {
		outcomeFields = append(outcomeFields, slog.F("transaction_status_error", statusErr.Error()))
	}
	if savedErr != nil {
		outcomeFields = append(outcomeFields, slog.F("schema_version_error", savedErr.Error()))
	}

	// outcome is the plain-language result appended to a failure message.
	var outcome string
	switch {
	case status == "committed" && savedErr == nil && saved == target:
		if upErr == nil {
			r.logger.Info(ctx, "database migration committed", append(outcomeFields,
				slog.F("from_version", displayVersion(from)),
				slog.F("applied", len(applied)),
				slog.F("slowest", slowest(applied, 3)))...)
			return nil
		}
		outcome = fmt.Sprintf("transaction committed, schema at %d", saved)
	case status == "committed":
		outcome = fmt.Sprintf("partially committed: schema at %s, expected %d", versionOrUnknown(saved, savedErr), target)
		r.logger.Error(ctx, "database migration partially committed", append(outcomeFields,
			slog.F("from_version", displayVersion(from)), slog.F("target_version", target))...)
	case status == "aborted":
		outcome = fmt.Sprintf("rolled back, schema unchanged at %s", versionOrUnknown(saved, savedErr))
		r.logger.Info(ctx, "database migration rolled back", outcomeFields...)
	case status == "in progress":
		outcome = "migration transaction is still in progress on the database"
		r.logger.Warn(ctx, "database migration transaction still in progress on the database", outcomeFields...)
	default:
		outcome = "outcome unknown"
		r.logger.Warn(ctx, "database migration outcome unknown", outcomeFields...)
	}

	if upErr == nil {
		return xerrors.Errorf("database migration did not complete: %s", outcome)
	}
	if current == nil || stmtErr == nil {
		return &Error{msg: fmt.Sprintf("%s; %s", upErr.Error(), outcome), err: upErr}
	}
	msg := fmt.Sprintf("migration %s (%d/%d) failed: %s", current, len(applied)+1, total, stmtErr.Error())
	if pgErr != nil {
		msg = fmt.Sprintf("migration %s (%d/%d) failed: %s (sqlstate=%s)", current, len(applied)+1, total, pgErr.Message, pgErr.Code)
	}
	return &Error{msg: msg + "; " + outcome, err: upErr}
}

// logFailure logs which migration failed and PostgreSQL's error fields.
func (r *reporter) logFailure(ctx context.Context, upErr error, current *migrationRef, stmtErr error, pgErr *pq.Error, applied, total int) {
	if current == nil || stmtErr == nil {
		r.logger.Error(ctx, "database migration failed", slog.Error(upErr))
		return
	}
	fields := []slog.Field{slog.F("migration", current.String()), slog.F("position", fmt.Sprintf("%d/%d", applied+1, total))}
	if pgErr == nil {
		r.logger.Error(ctx, "database migration failed", append(fields, slog.Error(stmtErr))...)
		return
	}
	fields = append(fields, slog.F("sqlstate", string(pgErr.Code)), slog.F("error", pgErr.Message))
	if pgErr.Detail != "" {
		fields = append(fields, slog.F("detail", pgErr.Detail))
	}
	if pgErr.Hint != "" {
		fields = append(fields, slog.F("hint", pgErr.Hint))
	}
	r.logger.Error(ctx, "database migration failed", fields...)
}

// Error is a migration failure with a concise message. The underlying error
// from golang-migrate, which includes the full migration SQL, is available
// through Unwrap but not printed.
type Error struct {
	msg string
	err error
}

func (e *Error) Error() string { return e.msg }
func (e *Error) Unwrap() error { return e.err }

// transactionStatus asks PostgreSQL whether the migration transaction
// committed: "committed", "aborted", "in progress", or "unknown".
func (r *reporter) transactionStatus(xid string) (string, error) {
	if xid == "" {
		return "unknown", xerrors.New("transaction id was not captured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), observerQueryTimeout)
	defer cancel()
	var status sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT pg_xact_status($1::xid8)`, xid).Scan(&status)
	if err != nil {
		return "unknown", err
	}
	if !status.Valid {
		return "unknown", xerrors.New("transaction too old for pg_xact_status")
	}
	return status.String, nil
}

// savedVersion reads the durable schema version. The migration transaction
// has ended by now, so schema_migrations is no longer locked.
func (r *reporter) savedVersion() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), observerQueryTimeout)
	defer cancel()
	var version int
	err := r.db.QueryRowContext(ctx, `SELECT version FROM `+migrationsTableName+` LIMIT 1`).Scan(&version)
	if xerrors.Is(err, sql.ErrNoRows) {
		return database.NilVersion, nil
	}
	if err != nil {
		return database.NilVersion, err
	}
	return version, nil
}

// startHeartbeat logs every interval until stop is called.
// ref is the running migration, or nil while waiting for the migration lock.
func (r *reporter) startHeartbeat(backendPID int, since time.Time, ref *migrationRef) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	ticker := r.clock.NewTicker(r.interval, "migrations", "heartbeat")
	go func() {
		defer close(done)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.heartbeat(ctx, backendPID, since, ref)
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
}

func (r *reporter) heartbeat(ctx context.Context, backendPID int, since time.Time, ref *migrationRef) {
	fields := []slog.Field{slog.F("elapsed", r.clock.Since(since).Truncate(time.Second))}
	if ref != nil {
		r.mu.Lock()
		position := fmt.Sprintf("%d/%d", len(r.applied)+1, r.total)
		r.mu.Unlock()
		fields = append(fields, slog.F("migration", ref.String()), slog.F("position", position))
	}
	fields = append(fields, r.backendEvidence(ctx, backendPID)...)

	if ref == nil {
		r.logger.Info(ctx, "database migration still waiting for migration lock", fields...)
		return
	}
	r.logger.Info(ctx, "database migration still running", fields...)
}

// backendEvidence describes what the migration connection is doing, using
// only pg_stat_activity and pg_blocking_pids. Coder's own database user can
// read these for its own sessions without extra privileges.
func (r *reporter) backendEvidence(ctx context.Context, backendPID int) []slog.Field {
	ctx, cancel := context.WithTimeout(ctx, observerQueryTimeout)
	defer cancel()

	var (
		waitType, waitEvent string
		blockers            pq.Int64Array
	)
	err := r.db.QueryRowContext(ctx, `
SELECT coalesce(wait_event_type, ''), coalesce(wait_event, ''), pg_blocking_pids(pid)
FROM pg_stat_activity WHERE pid = $1`, backendPID).Scan(&waitType, &waitEvent, &blockers)
	if err != nil {
		r.observerError(ctx, err)
		return []slog.Field{slog.F("state", "unknown")}
	}

	if waitType != "Lock" {
		state := "working"
		if waitType != "" {
			state = fmt.Sprintf("working (%s: %s)", waitType, waitEvent)
		}
		return []slog.Field{slog.F("state", state)}
	}

	fields := []slog.Field{slog.F("state", "waiting_on_lock"), slog.F("wait_event", waitEvent)}
	if len(blockers) > 0 {
		fields = append(fields, slog.F("blocked_by", r.describeSessions(ctx, blockers)))
	}
	return fields
}

// describeSessions returns "pid 184 (app=old-coder-pod, state=idle in
// transaction, xact_age=6m0s)" for each pid. Sessions of other database users
// show only pid and application name.
func (r *reporter) describeSessions(ctx context.Context, pids pq.Int64Array) string {
	rows, err := r.db.QueryContext(ctx, `
SELECT pid, coalesce(application_name, ''), coalesce(state, ''),
       coalesce(extract(epoch FROM now() - xact_start), -1)
FROM pg_stat_activity WHERE pid = ANY($1) ORDER BY pid`, pids)
	if err != nil {
		r.observerError(ctx, err)
		return formatPIDs(pids)
	}
	defer rows.Close()

	var parts []string
	for rows.Next() {
		var (
			pid        int
			app, state string
			ageSeconds float64
		)
		if err := rows.Scan(&pid, &app, &state, &ageSeconds); err != nil {
			r.observerError(ctx, err)
			return formatPIDs(pids)
		}
		attrs := []string{"app=" + orUnknown(app)}
		if state != "" {
			attrs = append(attrs, "state="+state)
		}
		if ageSeconds >= 0 {
			attrs = append(attrs, "xact_age="+(time.Duration(ageSeconds*float64(time.Second))).Truncate(time.Second).String())
		}
		parts = append(parts, fmt.Sprintf("pid %d (%s)", pid, strings.Join(attrs, ", ")))
	}
	if len(parts) == 0 {
		return formatPIDs(pids)
	}
	return strings.Join(parts, "; ")
}

// observerError logs the first reporting failure only; reporting failures
// never affect the migration.
func (r *reporter) observerError(ctx context.Context, err error) {
	r.mu.Lock()
	first := !r.observerFailed
	r.observerFailed = true
	r.mu.Unlock()
	if first {
		r.logger.Warn(ctx, "database migration progress details unavailable", slog.Error(err))
	}
}

// pendingFrom lists the embedded migrations after version, in order.
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
	rc, identifier, err := r.source.ReadUp(uint(version))
	if err != nil {
		return ""
	}
	_ = rc.Close()
	return identifier
}

func slowest(applied []appliedMigration, n int) string {
	sorted := append([]appliedMigration(nil), applied...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].duration > sorted[j].duration })
	if len(sorted) > n {
		sorted = sorted[:n]
	}
	parts := make([]string, 0, len(sorted))
	for _, m := range sorted {
		parts = append(parts, fmt.Sprintf("%s (%s)", m.migrationRef, m.duration.Truncate(time.Millisecond)))
	}
	return strings.Join(parts, ", ")
}

func formatPIDs(pids pq.Int64Array) string {
	parts := make([]string, len(pids))
	for i, p := range pids {
		parts[i] = fmt.Sprintf("pid %d", p)
	}
	return strings.Join(parts, "; ")
}

func displayVersion(v int) string {
	if v == database.NilVersion {
		return "none"
	}
	return fmt.Sprintf("%d", v)
}

func versionOrUnknown(v int, err error) string {
	if err != nil {
		return "unknown"
	}
	return displayVersion(v)
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
