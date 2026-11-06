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

type appliedMigration struct {
	migrationRef
	duration time.Duration
}

type failedMigration struct {
	migrationRef
	duration time.Duration
	// err is the error of the failed statement, as returned by the driver.
	// It never contains the SQL.
	err error
	// pgErr is PostgreSQL's error, nil when the statement failed for
	// another reason, such as a broken connection.
	pgErr *pq.Error
}

// runResult describes how a whole upgrade ended, across all its pending
// migrations.
type runResult struct {
	StartedAt  time.Time
	FinishedAt time.Time
	// FromVersion, TargetVersion and SchemaVersion are database.NilVersion
	// when there is no version or it is unknown. SchemaVersion is the
	// version saved after the run, as read from PostgreSQL.
	FromVersion   int
	TargetVersion int
	SchemaVersion int
	// Applied lists the migrations that ran in the migration transaction.
	// They are only saved if it committed.
	Applied           []appliedMigration
	Failed            *failedMigration
	TransactionStatus string
	Outcome           runOutcome
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

// finish is called when the upgrade has ended. err is golang-migrate's
// error.
//
// finish stops the heartbeats, asks PostgreSQL how the upgrade ended, logs
// one outcome line, and returns the error for the caller.
// With a nil reporter, when reporting is off, it only returns err, wrapped the same way
// as without reporting.
func (r *reporter) finish(err error) (runResult, error) {
	if r == nil {
		if err != nil {
			return runResult{}, xerrors.Errorf("up: %w", err)
		}
		return runResult{}, nil
	}
	r.close()

	r.mu.Lock()
	res := runResult{
		StartedAt:         r.runStart,
		FinishedAt:        r.clock.Now(),
		FromVersion:       r.fromVersion,
		TargetVersion:     r.targetVersion,
		SchemaVersion:     r.fromVersion,
		Applied:           append([]appliedMigration(nil), r.applied...),
		Failed:            r.failed,
		TransactionStatus: xactUnknown,
	}
	started, total, xid := r.started, r.total, r.xid
	lockWaitStart := r.lockWaitStart
	r.mu.Unlock()

	if started && total > 0 {
		res.TransactionStatus, res.SchemaVersion = r.readOutcome(xid)
	}
	res.Outcome = classify(outcomeInput{
		err:        err,
		started:    started,
		total:      total,
		xactStatus: res.TransactionStatus,
	})
	if res.Outcome == outcomeCommitted && res.SchemaVersion == database.NilVersion {
		// The commit succeeded, so the target version is what was saved.
		res.SchemaVersion = res.TargetVersion
	}

	r.logOutcome(res, err, lockWaitStart)

	if err != nil {
		if res.Failed != nil {
			return res, xerrors.Errorf("up: %w", res.Failed.asError())
		}
		return res, xerrors.Errorf("up: %w", err)
	}

	return res, nil
}

func (r *reporter) logOutcome(res runResult, err error, lockWaitStart time.Time) {
	ctx := context.Background()
	total := slog.F("total_duration_ms", res.FinishedAt.Sub(res.StartedAt).Milliseconds())
	count := slog.F("count", len(res.Applied))
	xactStatus := slog.F("transaction_status", res.TransactionStatus)

	switch res.Outcome {
	case outcomeNoChange:
		// Debug level: most starts have nothing to migrate, and they should
		// not add a line to every server start.
		fields := []slog.Field{slog.F("status", "success"), count}
		fields = append(fields, versionField("schema_version", res.SchemaVersion)...)
		r.logger.Debug(ctx, "database migration outcome: no change", append(fields, total)...)

	case outcomeCommitted:
		fields := []slog.Field{slog.F("status", "success"), count}
		fields = append(fields, versionField("from_version", res.FromVersion)...)
		fields = append(fields, versionField("to_version", res.SchemaVersion)...)
		r.logger.Info(ctx, "committed database migrations", append(fields, xactStatus, total)...)

	case outcomeRolledBack:
		fields := []slog.Field{slog.F("status", "failure"), count}
		if res.Failed != nil {
			fields = append(fields, slog.F("version", res.Failed.version), slog.F("name", res.Failed.name))
		}
		fields = append(fields, versionField("schema_version", res.SchemaVersion)...)
		if res.Failed != nil && res.Failed.pgErr != nil {
			fields = append(fields, slog.F("sqlstate", string(res.Failed.pgErr.Code)))
		}
		fields = append(fields, xactStatus, total)
		if res.Failed != nil {
			fields = append(fields, slog.F("failed_duration_ms", res.Failed.duration.Milliseconds()))
		}
		r.logger.Error(ctx, "migration failed, transaction rolled back, no changes saved", fields...)

	case outcomeCommittedBeforeFailure:
		fields := []slog.Field{slog.F("status", "failure"), count}
		fields = append(fields, versionField("from_version", res.FromVersion)...)
		fields = append(fields, versionField("to_version", res.SchemaVersion)...)
		r.logger.Error(ctx, "committed database migrations before failure", append(fields, xactStatus, total)...)

	case outcomeUnknown:
		fields := []slog.Field{slog.F("status", "failure"), count}
		fields = append(fields, versionField("version", res.FromVersion)...)
		fields = append(fields, xactStatus, total, slog.F("error", safeErrorText(res, err)))
		r.logger.Warn(ctx, "database migration commit outcome unknown", fields...)

	case outcomeLockTimeout:
		// Count from when Coder started waiting for the migration lock, or
		// from the start of the upgrade if it never had to wait.
		waitStart := lockWaitStart
		if waitStart.IsZero() {
			waitStart = res.StartedAt
		}
		r.logger.Error(ctx, "gave up waiting for database migration lock",
			slog.F("status", "failure"), count, slog.F("waited_ms", res.FinishedAt.Sub(waitStart).Milliseconds()))

	case outcomeDirty:
		var dirty migrate.ErrDirty
		_ = errors.As(err, &dirty)
		dirtyErr := slog.F("error", fmt.Sprintf("dirty database version %d", dirty.Version))
		r.logger.Error(ctx, "database schema is dirty",
			slog.F("status", "failure"), slog.F("version", dirty.Version), dirtyErr)
		r.logger.Error(ctx, "database migration outcome: dirty schema",
			slog.F("status", "failure"), count, slog.F("schema_version", dirty.Version), total, dirtyErr)

	case outcomeFailedBeforeStart:
		// The error is reported by the caller, as without reporting.
	}
}

// safeErrorText describes a failure without migration SQL or PostgreSQL's
// detail text, which can contain row data.
func safeErrorText(res runResult, err error) string {
	if res.Failed != nil {
		return res.Failed.asError().Error()
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
// It never waits longer than observerQueryTimeout: canceling a query only
// asks PostgreSQL to abort it, and a query that does not return in time is
// left to finish on its own while the run reports an unknown outcome.
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
			r.observerError(ctx, o.err)
		}
		return o.status, o.version
	case <-ctx.Done():
		r.observerError(context.Background(), ctx.Err())
		return xactUnknown, database.NilVersion
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
	if f.pgErr != nil {
		cause = f.pgErr
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
