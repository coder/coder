package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/lib/pq"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

// runOutcome is how a whole upgrade ended: all pending migrations applied
// together in one transaction.
type runOutcome string

const (
	outcomeNoChange               runOutcome = "no_change"
	outcomeCommitted              runOutcome = "committed"
	outcomeRolledBack             runOutcome = "rolled_back"
	outcomeCommittedBeforeFailure runOutcome = "committed_before_failure"
	outcomeUnknown                runOutcome = "unknown"
	outcomeLockTimeout            runOutcome = "lock_timeout"
	outcomeDirty                  runOutcome = "dirty"
	outcomeFailedBeforeStart      runOutcome = "failed_before_start"
)

// Transaction statuses as reported by pg_xact_status, and unknown when the
// status could not be read.
const (
	xactCommitted = "committed"
	xactAborted   = "aborted"
	xactUnknown   = "unknown"
)

type failedMigration struct {
	migrationRef
	duration time.Duration
	// err is the error of the failed statement, as returned by the driver.
	// It never contains the SQL.
	err error
}

// pgError returns PostgreSQL's error, or nil when the statement failed for
// another reason, such as a broken connection.
func (f *failedMigration) pgError() *pq.Error {
	var pgErr *pq.Error
	if errors.As(f.err, &pgErr) {
		return pgErr
	}
	return nil
}

// runResult is what finish learned about how the upgrade ended.
type runResult struct {
	outcome runOutcome
	now     time.Time
	// finalVersion is the schema version saved after the run: the target on
	// a commit, otherwise as read from PostgreSQL, or database.NilVersion
	// when there is none or it is unknown.
	finalVersion int
	xactStatus   string
}

// outcomeInput is what classify decides from.
type outcomeInput struct {
	// err is golang-migrate's error, without ErrNoChange.
	err error
	// started is true once the pending migrations were known.
	started bool
	// total is the number of migrations pending when the upgrade started.
	total int
	// xactStatus is the pg_xact_status of the migration transaction.
	xactStatus string
}

// classify decides how a run ended. A failure after migrations started is
// classified by what PostgreSQL reports for the migration transaction, not
// by the error, which cannot tell whether a commit happened.
func classify(in outcomeInput) runOutcome {
	switch {
	case in.err == nil && in.total == 0:
		return outcomeNoChange
	case in.err == nil:
		// The commit returned without an error, so it happened.
		return outcomeCommitted
	case errors.Is(in.err, migrate.ErrLockTimeout):
		return outcomeLockTimeout
	case errors.As(in.err, new(migrate.ErrDirty)):
		return outcomeDirty
	case !in.started:
		return outcomeFailedBeforeStart
	case in.xactStatus == xactCommitted:
		return outcomeCommittedBeforeFailure
	case in.xactStatus == xactAborted:
		return outcomeRolledBack
	default:
		return outcomeUnknown
	}
}

// finish is called once on Up's goroutine, after m.Up returned and the run
// was canceled. err is golang-migrate's error.
//
// finish asks PostgreSQL how the upgrade ended, logs one outcome line, and
// returns the error for the caller.
func (r *reporter) finish(err error) error {
	if errors.Is(err, migrate.ErrNoChange) {
		// Nothing to migrate is a success.
		err = nil
	}

	res := runResult{
		now:          r.clock.Now(),
		finalVersion: r.fromVersion,
		xactStatus:   xactUnknown,
	}
	switch {
	case err == nil:
		// The commit returned without an error, so it happened and saved
		// the target version. PostgreSQL is only asked when that is
		// unclear.
		res.finalVersion = r.targetVersion
		res.xactStatus = xactCommitted
	case r.started && r.total > 0:
		// xid is only read once started: before that, an abandoned Lock
		// may still write it.
		res.xactStatus, res.finalVersion = r.readOutcome(r.xid)
	}
	res.outcome = classify(outcomeInput{
		err:        err,
		started:    r.started,
		total:      r.total,
		xactStatus: res.xactStatus,
	})

	r.logOutcome(res, err)

	if err != nil {
		if r.failed != nil {
			return xerrors.Errorf("up: %w", r.failed.asError())
		}
		return xerrors.Errorf("up: %w", err)
	}
	return nil
}

// logOutcome logs with context.Background: runCtx is already canceled.
func (r *reporter) logOutcome(res runResult, err error) {
	ctx := context.Background()
	total := slog.F("total_duration_ms", res.now.Sub(r.runStart).Milliseconds())
	count := slog.F("count", r.applied)
	xactStatus := slog.F("transaction_status", res.xactStatus)
	finalVersion := versionField("final_version", res.finalVersion)
	failure := []slog.Field{slog.F("status", "failure"), count}

	switch res.outcome {
	case outcomeNoChange:
		// Debug level: most starts have nothing to migrate, and they should
		// not add a line to every server start.
		fields := []slog.Field{slog.F("status", "success"), count}
		fields = append(fields, finalVersion...)
		r.logger.Debug(ctx, msgOutcomeNoChange, append(fields, total)...)

	case outcomeCommitted:
		fields := []slog.Field{slog.F("status", "success"), count}
		fields = append(fields, versionField("from_version", r.fromVersion)...)
		fields = append(fields, finalVersion...)
		r.logger.Info(ctx, msgOutcomeCommitted, append(fields, xactStatus, total)...)

	case outcomeRolledBack:
		fields := failure
		if r.failed != nil {
			fields = append(fields, slog.F("version", r.failed.version), slog.F("name", r.failed.name))
		}
		fields = append(fields, finalVersion...)
		if r.failed != nil {
			if pgErr := r.failed.pgError(); pgErr != nil {
				fields = append(fields, slog.F("sqlstate", pgErr.Code))
			}
		}
		fields = append(fields, xactStatus, total)
		if r.failed != nil {
			fields = append(fields, slog.F("failed_duration_ms", r.failed.duration.Milliseconds()))
		}
		fields = append(fields, slog.F("error", safeErrorText(r.failed, err)))
		r.logger.Error(ctx, msgOutcomeRolledBack, fields...)

	case outcomeCommittedBeforeFailure:
		fields := failure
		fields = append(fields, versionField("from_version", r.fromVersion)...)
		fields = append(fields, finalVersion...)
		fields = append(fields, xactStatus, total, slog.F("error", safeErrorText(r.failed, err)))
		r.logger.Error(ctx, msgOutcomeCommittedBeforeFailure, fields...)

	case outcomeUnknown:
		fields := failure
		fields = append(fields, finalVersion...)
		fields = append(fields, xactStatus, total, slog.F("error", safeErrorText(r.failed, err)))
		r.logger.Warn(ctx, msgOutcomeUnknown, fields...)

	case outcomeLockTimeout:
		// The batch Lock is the first thing m.Up waits for, so the time
		// since Up called it is the lock wait.
		r.logger.Error(ctx, msgOutcomeLockTimeout,
			append(failure, slog.F("waited_ms", res.now.Sub(r.upStart).Milliseconds()), slog.F("error", safeErrorText(r.failed, err)))...)

	case outcomeDirty:
		var dirty migrate.ErrDirty
		_ = errors.As(err, &dirty)
		r.logger.Error(ctx, msgOutcomeDirty, append(failure,
			slog.F("final_version", dirty.Version), total,
			slog.F("error", fmt.Sprintf("dirty database version %d", dirty.Version)))...)

	case outcomeFailedBeforeStart:
		r.logger.Error(ctx, msgOutcomeFailedBeforeStart,
			append(failure, total, slog.F("error", safeErrorText(r.failed, err)))...)
	}
}

// safeErrorText describes a failure without migration SQL or PostgreSQL's
// detail text, which can contain row data.
func safeErrorText(failed *failedMigration, err error) string {
	if failed != nil {
		return failed.asError().Error()
	}
	// golang-migrate's database.Error prints the query it ran. Outside a
	// migration statement that is one of its own short statements, but
	// leave it out all the same.
	var dbErr database.Error
	if errors.As(err, &dbErr) {
		return describeDBError(dbErr)
	}
	var dbErrPtr *database.Error
	if errors.As(err, &dbErrPtr) {
		return describeDBError(*dbErrPtr)
	}
	return err.Error()
}

// describeDBError is database.Error's message without the query.
func describeDBError(e database.Error) string {
	if e.Err == "" {
		return fmt.Sprint(e.OrigErr)
	}
	return fmt.Sprintf("%s: %v", e.Err, e.OrigErr)
}

// readOutcome asks PostgreSQL whether the migration transaction committed,
// using the ID captured when it started, and which schema version is saved.
// It never waits longer than observerQueryTimeout. The queries run in a
// goroutine it does not wait for, because canceling a lib/pq query still
// blocks until the server answers; a late one finishes on its own while the
// run reports an unknown outcome.
func (r *reporter) readOutcome(xid string) (string, int) {
	ctx, cancel := context.WithTimeout(context.Background(), observerQueryTimeout)
	defer cancel()

	type outcome struct {
		status  string
		version int
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		status, err := r.transactionStatus(ctx, xid)
		if err != nil {
			done <- outcome{status: status, version: database.NilVersion, err: err}
			return
		}
		version, err := r.savedVersion(ctx)
		done <- outcome{status: status, version: version, err: err}
	}()

	select {
	case o := <-done:
		if o.err != nil {
			r.observerError(o.err)
		}
		return o.status, o.version
	case <-ctx.Done():
		r.observerError(ctx.Err())
		return xactUnknown, database.NilVersion
	}
}

// observerError logs the first failed reporting query of the run only.
// Reporting failures never affect the migration.
func (r *reporter) observerError(err error) {
	if r.observerFailed.CompareAndSwap(false, true) {
		r.logger.Warn(context.Background(), msgObserverFailed, slog.Error(err))
	}
}

func (r *reporter) transactionStatus(ctx context.Context, xid string) (string, error) {
	if xid == "" {
		return xactUnknown, nil
	}
	var status sql.NullString
	err := r.observerDB.QueryRowContext(ctx, `SELECT pg_xact_status($1::xid8)`, xid).Scan(&status)
	if err != nil {
		return xactUnknown, err
	}
	if !status.Valid {
		// PostgreSQL no longer keeps the status of a transaction this old.
		return xactUnknown, nil
	}
	return status.String, nil
}

// savedVersion reads the schema version saved after the run. The migration
// transaction has ended, so schema_migrations is no longer locked.
func (r *reporter) savedVersion(ctx context.Context) (int, error) {
	var version int
	err := r.observerDB.QueryRowContext(ctx, `SELECT version FROM `+migrationsTableName+` LIMIT 1`).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return database.NilVersion, nil
	}
	if err != nil {
		return database.NilVersion, err
	}
	return version, nil
}

// asError describes the failed migration without its SQL or PostgreSQL's
// detail text.
func (f *failedMigration) asError() *migrationError {
	cause := f.err
	if pgErr := f.pgError(); pgErr != nil {
		cause = pgErr
	}
	return &migrationError{migrationRef: f.migrationRef, cause: cause}
}

// migrationError reports a failed migration without its SQL or PostgreSQL's
// detail text, which can contain row data. It unwraps only to the driver's
// error, so error printers that follow Unwrap cannot reach the SQL either.
type migrationError struct {
	migrationRef
	cause error
}

func (e *migrationError) Error() string {
	var pgErr *pq.Error
	if errors.As(e.cause, &pgErr) {
		return fmt.Sprintf("migration %d (%s): migration failed: %s (sqlstate=%s)",
			e.version, e.name, pgErr.Message, pgErr.Code)
	}
	return fmt.Sprintf("migration %d (%s): migration failed: %v", e.version, e.name, e.cause)
}

func (e *migrationError) Unwrap() error {
	return e.cause
}
