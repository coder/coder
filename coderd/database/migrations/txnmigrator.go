package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/lib/pq"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

const (
	lockID              = int64(1037453835920848937)
	migrationsTableName = "schema_migrations"

	// heartbeatInterval is how often a lock wait or an in-flight migration
	// reports that it is still alive.
	heartbeatInterval = 30 * time.Second
	// heartbeatQueryTimeout bounds the diagnostic pg_stat_activity query so a
	// struggling database can never make the heartbeat itself hang.
	heartbeatQueryTimeout = 2 * time.Second
)

// pgTxnDriver is a Postgres migration driver that runs all migrations in a
// single transaction. This is done to prevent users from being locked out of
// their deployment if a migration fails, since the schema will simply revert
// back to the previous version.
type pgTxnDriver struct {
	ctx context.Context // context.Background(); migration SQL is never cancelled
	db  *sql.DB
	tx  *sql.Tx

	// logger receives structured progress and outcome events. It is the
	// zero value (a no-op logger) for plain Up/UpWithFS callers.
	logger slog.Logger
	// diagnostics gates the heartbeat's pg_stat_activity query, so plain
	// Up/UpWithFS callers issue no extra queries.
	diagnostics bool
	// source resolves a migration version to its identifier via ReadUp, and
	// is used to count pending migrations ahead of a run.
	source source.Driver

	mu             sync.Mutex // guards the per-run state below
	backendPID     int        // read right after BeginTx, so the heartbeat can query it
	fromVersion    int        // version read at the start of this run; -1 (NilVersion) on an empty database
	total          int        // pending count read at the start of this run
	inFlight       int        // version currently executing, or -1
	inFlightName   string     // name of the version currently executing
	inFlightStart  time.Time
	applied        []Applied // durable only once Unlock commits
	commitErr      error     // result of Commit() in the most recent Unlock
	sqlState       string    // SQLSTATE of the failed statement, captured here because database.Error has no Unwrap
	failureMessage string    // human-readable message for the failed statement, same reason
	stopHeartbeat  func()    // stops the heartbeat started for the in-flight migration, if any
}

// Progress is a snapshot of the currently running migration batch. The
// heartbeat reads it today; a later phase's status page and live metrics
// will read it too.
type Progress struct {
	InFlightVersion int
	InFlightName    string
	Position        int
	Total           int
	StartedAt       time.Time
	BackendPID      int
}

// Progress returns a snapshot of the current run. Safe for concurrent use.
func (d *pgTxnDriver) Progress() Progress {
	d.mu.Lock()
	defer d.mu.Unlock()
	return Progress{
		InFlightVersion: d.inFlight,
		InFlightName:    d.inFlightName,
		Position:        len(d.applied) + 1,
		Total:           d.total,
		StartedAt:       d.inFlightStart,
		BackendPID:      d.backendPID,
	}
}

func (*pgTxnDriver) Open(string) (database.Driver, error) {
	panic("not implemented")
}

func (*pgTxnDriver) Close() error {
	return nil
}

// Lock begins the transaction for this run and acquires the advisory lock
// that keeps concurrent instances from migrating at once. It tries a
// non-blocking acquire first so the common, uncontested case logs nothing;
// a replica that must wait reports when the wait starts and, with its
// duration, when it ends.
func (d *pgTxnDriver) Lock() error {
	d.mu.Lock()
	d.backendPID = 0
	d.fromVersion = 0
	d.total = 0
	d.inFlight = -1
	d.inFlightName = ""
	d.inFlightStart = time.Time{}
	d.applied = nil
	d.commitErr = nil
	d.sqlState = ""
	d.failureMessage = ""
	d.stopHeartbeat = nil
	d.mu.Unlock()

	var err error
	d.tx, err = d.db.BeginTx(d.ctx, nil)
	if err != nil {
		return err
	}

	var pid int
	if err := d.tx.QueryRowContext(d.ctx, "SELECT pg_backend_pid()").Scan(&pid); err == nil {
		d.mu.Lock()
		d.backendPID = pid
		d.mu.Unlock()
	}

	var acquired bool
	const tryQuery = `SELECT pg_try_advisory_xact_lock($1)`
	if err := d.tx.QueryRowContext(d.ctx, tryQuery, lockID).Scan(&acquired); err != nil {
		return xerrors.Errorf("try advisory lock: %w", err)
	}
	if acquired {
		return nil
	}

	d.logger.Info(d.ctx, "waiting for database migration lock held by another instance",
		slog.F("event", EventLockWaitStarted))
	start := time.Now()
	stop := d.startHeartbeat(start, heartbeatWaiting)
	const lockQuery = `SELECT pg_advisory_xact_lock($1)`
	_, err = d.tx.ExecContext(d.ctx, lockQuery, lockID)
	stop()
	if err != nil {
		return xerrors.Errorf("exec select: %w", err)
	}
	d.logger.Info(d.ctx, "acquired database migration lock",
		slog.F("event", EventLockAcquired), slog.F("waited", time.Since(start)))
	return nil
}

// Unlock commits the transaction as before. It only records the commit
// result; runUp classifies the outcome once m.Up() returns, since only
// runUp knows whether a non-SQL failure happened after the commit
// succeeded.
//
// golang-migrate calls Unlock() exactly once per Lock(), on every path,
// including a failed run. When a migration's Run() fails, golang-migrate
// never calls the trailing SetVersion(version, false) that would normally
// stop that migration's heartbeat, so Unlock() is also the one place
// guaranteed to run that can still stop a heartbeat left behind by a
// failed migration.
func (d *pgTxnDriver) Unlock() error {
	d.mu.Lock()
	stop := d.stopHeartbeat
	d.stopHeartbeat = nil
	d.mu.Unlock()
	if stop != nil {
		stop()
	}

	err := d.tx.Commit()
	d.tx = nil
	d.mu.Lock()
	d.commitErr = err
	d.mu.Unlock()
	if err != nil {
		return xerrors.Errorf("commit tx on unlock: %w", err)
	}
	return nil
}

func (d *pgTxnDriver) Run(migration io.Reader) error {
	migr, err := io.ReadAll(migration)
	if err != nil {
		return xerrors.Errorf("read migration: %w", err)
	}
	err = d.runStatement(migr)
	if err != nil {
		return xerrors.Errorf("run statement: %w", err)
	}
	return nil
}

func (d *pgTxnDriver) runStatement(statement []byte) error {
	ctx := context.Background()
	query := string(statement)
	if strings.TrimSpace(query) == "" {
		return nil
	}
	if _, err := d.tx.ExecContext(ctx, query); err != nil {
		var pgErr *pq.Error
		if xerrors.As(err, &pgErr) {
			var line uint
			message := fmt.Sprintf("migration failed: %s", pgErr.Message)
			if pgErr.Detail != "" {
				message += ", " + pgErr.Detail
			}
			d.mu.Lock()
			d.sqlState = string(pgErr.Code)
			d.failureMessage = message
			d.mu.Unlock()
			return database.Error{OrigErr: err, Err: message, Query: statement, Line: line}
		}
		d.mu.Lock()
		d.failureMessage = "migration failed"
		d.mu.Unlock()
		return database.Error{OrigErr: err, Err: "migration failed", Query: statement}
	}
	return nil
}

// SetVersion is called by golang-migrate with dirty=true immediately
// before each migration runs, and dirty=false once it succeeds. Those two
// calls are the "starting"/"applied" log points and the start/stop points
// for the per-migration heartbeat.
//
//nolint:revive
func (d *pgTxnDriver) SetVersion(version int, dirty bool) error {
	query := `TRUNCATE ` + migrationsTableName
	if _, err := d.tx.Exec(query); err != nil {
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}

	if version >= 0 {
		query = `INSERT INTO ` + migrationsTableName + ` (version, dirty) VALUES ($1, $2)`
		if _, err := d.tx.Exec(query, version, dirty); err != nil {
			return &database.Error{OrigErr: err, Query: []byte(query)}
		}
	}

	if version < 0 {
		return nil
	}

	name := d.migrationName(version)
	if dirty {
		d.mu.Lock()
		d.inFlight = version
		d.inFlightName = name
		d.inFlightStart = time.Now()
		position := len(d.applied) + 1
		total := d.total
		d.mu.Unlock()

		d.logPerMigration(EventRunStarted, "starting database migration",
			slog.F("version", version), slog.F("name", name),
			slog.F("position", position), slog.F("total", total))

		stop := d.startHeartbeat(time.Now(), heartbeatRunning)
		d.mu.Lock()
		d.stopHeartbeat = stop
		d.mu.Unlock()
		return nil
	}

	d.mu.Lock()
	started := d.inFlightStart
	position := len(d.applied) + 1
	total := d.total
	stop := d.stopHeartbeat
	d.stopHeartbeat = nil
	d.mu.Unlock()
	if stop != nil {
		stop()
	}

	duration := time.Since(started)
	d.mu.Lock()
	d.applied = append(d.applied, Applied{Version: version, Name: name, StartedAt: started, Duration: duration})
	d.inFlight = -1
	d.inFlightName = ""
	d.mu.Unlock()

	d.logPerMigration(EventRunApplied, "database migration applied, pending commit",
		slog.F("version", version), slog.F("name", name),
		slog.F("position", position), slog.F("total", total),
		slog.F("duration", duration))
	return nil
}

// logPerMigration logs the started/applied lines at DEBUG on an empty
// database, since a first boot applies every migration, and at INFO
// otherwise. The inventory and outcome lines are unaffected. event is
// prepended so it stays the first field regardless of the caller's fields.
func (d *pgTxnDriver) logPerMigration(event Event, msg string, fields ...slog.Field) {
	d.mu.Lock()
	empty := d.fromVersion == database.NilVersion
	d.mu.Unlock()

	fields = append([]slog.Field{slog.F("event", event)}, fields...)

	if empty {
		d.logger.Debug(d.ctx, msg, fields...)
		return
	}
	d.logger.Info(d.ctx, msg, fields...)
}

func (d *pgTxnDriver) Version() (version int, dirty bool, err error) {
	// If the transaction is valid (we hold the exclusive lock), use the txn for
	// the query.
	var q interface {
		QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	} = d.tx
	// If we don't hold the lock just use the database. This only happens in the
	// `Stepper` function and is only used in tests.
	if d.tx == nil {
		q = d.db
	}

	query := `SELECT version, dirty FROM ` + migrationsTableName + ` LIMIT 1`
	err = q.QueryRowContext(context.Background(), query).Scan(&version, &dirty)
	switch {
	case err == sql.ErrNoRows:
		version, dirty, err = database.NilVersion, false, nil

	case err != nil:
		var pgErr *pq.Error
		if xerrors.As(err, &pgErr) && pgErr.Code.Name() == "undefined_table" {
			version, dirty, err = database.NilVersion, false, nil
		} else {
			return 0, false, &database.Error{OrigErr: err, Query: []byte(query)}
		}
	}

	d.logPending(version, dirty)
	return version, dirty, nil
}

// logPending logs the "database migrations required" / "database schema is
// up to date" / "database schema is dirty" line. golang-migrate calls
// Version() exactly once, right after taking the lock, so this is an exact
// count that never waits on another instance's transaction.
func (d *pgTxnDriver) logPending(version int, dirty bool) {
	d.mu.Lock()
	d.fromVersion = version
	d.mu.Unlock()

	if dirty {
		d.logger.Warn(d.ctx, "database schema is dirty",
			slog.F("event", EventInventoryDirty), slog.F("version", version))
		return
	}

	total, target := d.pendingMigrations(version)
	d.mu.Lock()
	d.total = total
	d.mu.Unlock()

	if total == 0 {
		d.logger.Info(d.ctx, "database schema is up to date",
			slog.F("event", EventInventoryUpToDate), slog.F("version", version))
		return
	}
	d.logger.Info(d.ctx, "database migrations required",
		slog.F("event", EventInventoryPending),
		slog.F("current_version", version), slog.F("target_version", target), slog.F("pending", total))
}

// pendingMigrations walks the source driver from version to count the
// remaining up migrations and find the last available version. The count is
// a function of the embedded files and a version, so the pre-upgrade plan
// phase can reuse it.
func (d *pgTxnDriver) pendingMigrations(version int) (count int, target int) {
	if d.source == nil {
		return 0, version
	}

	var next uint
	var err error
	if version < 0 {
		next, err = d.source.First()
	} else {
		next, err = d.source.Next(uint(version))
	}

	target = version
	for err == nil {
		count++
		target = int(next)
		next, err = d.source.Next(next)
	}
	return count, target
}

// migrationName resolves a version to its authored identifier, e.g.
// "ai_user_daily_spend" for 000536_ai_user_daily_spend.up.sql. golang-migrate
// already parses this from the filename to run the migration, so reading it
// again here for logging costs nothing extra.
func (d *pgTxnDriver) migrationName(version int) string {
	if d.source == nil || version < 0 {
		return ""
	}
	rc, identifier, err := d.source.ReadUp(uint(version))
	if err != nil {
		return ""
	}
	_ = rc.Close()
	return identifier
}

func (*pgTxnDriver) Drop() error {
	panic("not implemented")
}

func (d *pgTxnDriver) ensureVersionTable() error {
	err := d.Lock()
	if err != nil {
		return xerrors.Errorf("acquire migration lock: %w", err)
	}

	const query = `CREATE TABLE IF NOT EXISTS ` + migrationsTableName + ` (version bigint not null primary key, dirty boolean not null)`
	if _, err := d.tx.ExecContext(context.Background(), query); err != nil {
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}

	err = d.Unlock()
	if err != nil {
		return xerrors.Errorf("release migration lock: %w", err)
	}

	return nil
}

type heartbeatPhase int

const (
	heartbeatWaiting heartbeatPhase = iota
	heartbeatRunning
)

// startHeartbeat starts a background goroutine that logs progress every
// heartbeatInterval while a phase is active, and returns a func to stop it.
// It is a no-op when diagnostics is disabled, so plain Up/UpWithFS callers
// issue no extra queries.
func (d *pgTxnDriver) startHeartbeat(since time.Time, phase heartbeatPhase) (stop func()) {
	if !d.diagnostics {
		return func() {}
	}
	done := make(chan struct{})
	var once sync.Once
	go func() {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				d.logHeartbeat(since, phase)
			}
		}
	}()
	return func() { once.Do(func() { close(done) }) }
}

func (d *pgTxnDriver) logHeartbeat(since time.Time, phase heartbeatPhase) {
	elapsed := time.Since(since).Truncate(time.Second)

	d.mu.Lock()
	pid := d.backendPID
	version, name := d.inFlight, d.inFlightName
	position, total := len(d.applied)+1, d.total
	d.mu.Unlock()

	state, waitEvent, blockedBy := d.queryBackendState(pid)

	rest := []slog.Field{slog.F("elapsed", elapsed), slog.F("state", state)}
	if waitEvent != "" {
		rest = append(rest, slog.F("wait_event", waitEvent))
	}
	if blockedBy != "" {
		rest = append(rest, slog.F("blocked_by", blockedBy))
	}

	switch phase {
	case heartbeatWaiting:
		fields := append([]slog.Field{slog.F("event", EventHeartbeat)}, rest...)
		d.logger.Info(d.ctx, "still waiting for database migration lock", fields...)
	case heartbeatRunning:
		fields := append([]slog.Field{
			slog.F("event", EventHeartbeat),
			slog.F("version", version), slog.F("name", name),
			slog.F("position", position), slog.F("total", total),
		}, rest...)
		d.logger.Info(d.ctx, "database migration still running", fields...)
	}
}

// queryBackendState inspects pg_stat_activity for pid on a separate pooled
// connection, so it never competes with the migration's own long-running
// statement. It logs pids and wait events only, never query text, and needs
// no extra database privileges.
func (d *pgTxnDriver) queryBackendState(pid int) (state, waitEvent, blockedBy string) {
	if pid == 0 {
		return "unknown", "", ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), heartbeatQueryTimeout)
	defer cancel()

	const q = `
SELECT state, coalesce(wait_event_type, ''), coalesce(wait_event, ''), pg_blocking_pids(pid)
FROM pg_stat_activity WHERE pid = $1
`
	var (
		dbState       string
		waitEventType string
		we            string
		blockers      pq.Int32Array
	)
	row := d.db.QueryRowContext(ctx, q, pid)
	if err := row.Scan(&dbState, &waitEventType, &we, &blockers); err != nil {
		return "unknown", "", ""
	}
	// Postgres has several wait_event_type categories (Lock, IO, Timeout,
	// Client, IPC, ...); only Lock is an actual lock wait. Everything
	// else, including a deliberately slow statement, is still forward
	// progress from our perspective.
	if waitEventType != "Lock" {
		return "running", "", ""
	}

	pids := make([]string, len(blockers))
	for i, b := range blockers {
		pids[i] = strconv.Itoa(int(b))
	}
	return "waiting_on_lock", we, strings.Join(pids, ",")
}
