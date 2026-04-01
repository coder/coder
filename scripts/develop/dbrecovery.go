//go:build !windows

package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/lib/pq"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

const trackingDDL = `
CREATE SCHEMA IF NOT EXISTS _develop;
CREATE TABLE IF NOT EXISTS _develop.applied_migrations (
    version BIGINT PRIMARY KEY,
    filename TEXT NOT NULL,
    up_sql TEXT NOT NULL DEFAULT '',
    down_sql TEXT NOT NULL DEFAULT ''
);
-- Schema migrations for the tracking table itself go here.
ALTER TABLE _develop.applied_migrations ADD COLUMN IF NOT EXISTS up_sql TEXT NOT NULL DEFAULT '';
`

// recoverDB checks for migration conflicts before the server
// starts. It connects to postgres on every run (embedded postgres
// starts fast enough that caching is unnecessary) and compares
// the tracking table against files on disk.
//
// Conflicts:
//   - Tracked file missing from disk → needs --db-rollback or --db-reset.
//   - Tracked file content differs from disk → needs --db-continue or --db-reset.
//   - New files on disk not tracked → normal forward migration, server handles it.
func recoverDB(ctx context.Context, logger slog.Logger, cfg *devConfig) error {
	pgURL := os.Getenv("CODER_PG_CONNECTION_URL")
	isBuiltinPG := pgURL == ""

	if isBuiltinPG {
		pgDir := filepath.Join(cfg.configDir, "postgres")
		if _, err := os.Stat(filepath.Join(pgDir, "data")); err != nil {
			return nil // Fresh install.
		}
		if cfg.dbReset {
			logger.Warn(ctx, "wiping built-in database (--db-reset)")
			if err := os.RemoveAll(pgDir); err != nil {
				return xerrors.Errorf("remove postgres directory: %w", err)
			}
			return nil
		}
		stopPG, err := startTempPostgresSetURL(ctx, logger, cfg, &pgURL)
		if err != nil {
			return xerrors.Errorf(
				"cannot start temporary postgres: %w\n\ntry --db-reset instead", err)
		}
		defer stopPG()
	} else if cfg.dbReset {
		db, err := connectDB(ctx, pgURL)
		if err != nil {
			return xerrors.Errorf("connect for reset: %w", err)
		}
		defer db.Close()
		_, _ = fmt.Fprintf(os.Stderr,
			"\n  WARNING: this will DROP all schemas in the external database.\n"+
				"  Set CODER_DEV_DB_RESET=1 to confirm.\n\n")
		if os.Getenv("CODER_DEV_DB_RESET") != "1" {
			return xerrors.New("refusing to reset external database without CODER_DEV_DB_RESET=1")
		}
		logger.Warn(ctx, "resetting external database (--db-reset)")
		return resetSchema(ctx, db)
	}

	db, err := connectDB(ctx, pgURL)
	if err != nil {
		return xerrors.Errorf("connect: %w", err)
	}
	defer db.Close()

	migrDir := filepath.Join(cfg.projectRoot, "coderd", "database", "migrations")
	return checkAndRecover(ctx, logger, db, migrDir, cfg)
}

// checkAndRecover is the core logic:
//  1. Ensure tracking table exists.
//  2. Read DB version. Refuse if dirty.
//  3. Detect untracked migrations.
//  4. Detect missing files (needs rollback).
//  5. Detect content changes (needs --db-continue).
//  6. Capture current disk state for next time.
func checkAndRecover(ctx context.Context, logger slog.Logger, db *sql.DB, migrDir string, cfg *devConfig) error {
	if _, err := db.ExecContext(ctx, trackingDDL); err != nil {
		return xerrors.Errorf("create tracking table: %w", err)
	}

	dbVersion, dirty, err := currentMigrationVersion(ctx, db)
	if err != nil {
		return xerrors.Errorf("get db version: %w", err)
	}
	if dbVersion < 0 {
		return nil // Fresh DB.
	}
	if dirty {
		return xerrors.Errorf(
			"database is dirty at version %d (a migration failed halfway)\n\n"+
				"  --db-reset  destroy database and start fresh\n", dbVersion)
	}

	maxTracked, err := maxTrackedVersion(ctx, db)
	if err != nil {
		return xerrors.Errorf("get max tracked version: %w", err)
	}
	if dbVersion > maxTracked && maxTracked >= 0 {
		// Gap between tracking and DB version. This happens when
		// the server applied migrations via Up() but develop.sh
		// was interrupted before updateMigrationTracking ran.
		// captureDownSQL at the end of this function backfills
		// from disk.
		logger.Warn(ctx, "migration tracking gap detected, will backfill",
			slog.F("db_version", dbVersion),
			slog.F("max_tracked", maxTracked))
	}

	// Check for missing files (rollback candidates).
	rollbacks, err := findRollbacks(ctx, db, migrDir)
	if err != nil {
		return xerrors.Errorf("find rollbacks: %w", err)
	}

	if len(rollbacks) > 0 {
		if !cfg.dbRollback {
			var details strings.Builder
			for _, rb := range rollbacks {
				_, _ = fmt.Fprintf(&details, "  version %d: %s (missing from disk)\n", rb.version, rb.filename)
			}
			return xerrors.Errorf(
				"database has migrations that no longer exist on disk:\n%s\n"+
					"  --db-rollback   roll back these migrations (preserves data)\n"+
					"  --db-reset      destroy database and start fresh\n",
				details.String())
		}

		if !contiguousFromTop(rollbacks, dbVersion) {
			return xerrors.Errorf(
				"cannot roll back: versions are not contiguous (%s); use --db-reset",
				formatVersions(rollbacks))
		}

		logger.Warn(ctx, "rolling back mismatched migrations",
			slog.F("db_version", dbVersion),
			slog.F("count", len(rollbacks)))

		for _, rb := range rollbacks {
			if err := applyRollback(ctx, db, rb); err != nil {
				return xerrors.Errorf(
					"rollback of version %d (%s) failed: %w\n\nuse --db-reset to start fresh",
					rb.version, rb.filename, err)
			}
			logger.Info(ctx, "rolled back migration",
				slog.F("version", rb.version),
				slog.F("filename", rb.filename))
		}

		dbVersion, _, err = currentMigrationVersion(ctx, db)
		if err != nil {
			return xerrors.Errorf("get db version after rollback: %w", err)
		}
		logger.Info(ctx, "database recovery complete")
	}

	// Check for content changes (same filename, different SQL).
	contentChanges, err := findContentChanges(ctx, db, migrDir)
	if err != nil {
		return xerrors.Errorf("check content changes: %w", err)
	}
	if len(contentChanges) > 0 && !cfg.dbContinue {
		var details strings.Builder
		for _, cc := range contentChanges {
			_, _ = fmt.Fprintf(&details, "\n  version %d: %s\n", cc.version, cc.filename)
			if cc.upChanged {
				_, _ = fmt.Fprintf(&details, "    up.sql differs:\n%s\n", formatDiff("tracked", "disk", cc.trackedUp, cc.diskUp))
			}
			if cc.downChanged {
				_, _ = fmt.Fprintf(&details, "    down.sql differs:\n%s\n", formatDiff("tracked", "disk", cc.trackedDown, cc.diskDown))
			}
		}
		return xerrors.Errorf(
			"migration content changed on disk:%s\n"+
				"  --db-continue   accept changes and update tracking (assumes DB state is compatible)\n"+
				"  --db-reset      destroy database and start fresh\n",
			details.String())
	}
	if len(contentChanges) > 0 && cfg.dbContinue {
		logger.Warn(ctx, "accepting changed migrations (--db-continue)",
			slog.F("count", len(contentChanges)))
	}

	// Capture current disk state.
	if err := captureDownSQL(ctx, db, migrDir, dbVersion); err != nil {
		return xerrors.Errorf("capture migrations: %w", err)
	}

	return nil
}

type rollbackEntry struct {
	version  int
	filename string
	downSQL  string
}

type contentChange struct {
	version               int
	filename              string
	upChanged             bool
	downChanged           bool
	trackedUp, diskUp     string
	trackedDown, diskDown string
}

// findRollbacks returns tracked migrations whose file no longer
// exists on disk, sorted in descending version order.
func findRollbacks(ctx context.Context, db *sql.DB, migrDir string) ([]rollbackEntry, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT version, filename, down_sql
		FROM _develop.applied_migrations
		ORDER BY version DESC
	`)
	if err != nil {
		return nil, xerrors.Errorf("query tracking table: %w", err)
	}
	defer rows.Close()

	var rollbacks []rollbackEntry
	for rows.Next() {
		var rb rollbackEntry
		if err := rows.Scan(&rb.version, &rb.filename, &rb.downSQL); err != nil {
			return nil, xerrors.Errorf("scan row: %w", err)
		}
		downPath := filepath.Join(migrDir, rb.filename)
		if _, err := os.Stat(downPath); err != nil {
			rollbacks = append(rollbacks, rb)
		}
	}
	return rollbacks, rows.Err()
}

// findContentChanges compares tracked up/down SQL against disk
// for all tracked versions whose files still exist.
func findContentChanges(ctx context.Context, db *sql.DB, migrDir string) ([]contentChange, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT version, filename, up_sql, down_sql
		FROM _develop.applied_migrations
		ORDER BY version
	`)
	if err != nil {
		return nil, xerrors.Errorf("query tracking table: %w", err)
	}
	defer rows.Close()

	var changes []contentChange
	for rows.Next() {
		var version int
		var filename, trackedUp, trackedDown string
		if err := rows.Scan(&version, &filename, &trackedUp, &trackedDown); err != nil {
			return nil, xerrors.Errorf("scan row: %w", err)
		}

		// Only check files that exist on disk (missing files
		// are handled by findRollbacks).
		downPath := filepath.Join(migrDir, filename)
		if _, err := os.Stat(downPath); err != nil {
			continue
		}

		// Derive up filename from down filename.
		upFilename := strings.Replace(filename, ".down.sql", ".up.sql", 1)

		diskDown, err := os.ReadFile(filepath.Join(migrDir, filename))
		if err != nil {
			continue
		}
		diskUp, err := os.ReadFile(filepath.Join(migrDir, upFilename))
		if err != nil {
			continue
		}

		upChanged := trackedUp != "" && trackedUp != string(diskUp)
		downChanged := trackedDown != "" && trackedDown != string(diskDown)

		if upChanged || downChanged {
			changes = append(changes, contentChange{
				version:     version,
				filename:    filename,
				upChanged:   upChanged,
				downChanged: downChanged,
				trackedUp:   trackedUp,
				diskUp:      string(diskUp),
				trackedDown: trackedDown,
				diskDown:    string(diskDown),
			})
		}
	}
	return changes, rows.Err()
}

func maxTrackedVersion(ctx context.Context, db *sql.DB) (int, error) {
	var v sql.NullInt64
	err := db.QueryRowContext(ctx,
		`SELECT MAX(version) FROM _develop.applied_migrations`,
	).Scan(&v)
	if err != nil {
		var pgErr *pq.Error
		if xerrors.As(err, &pgErr) && pgErr.Code.Name() == "undefined_table" {
			return -1, nil
		}
		return -1, xerrors.Errorf("query max tracked version: %w", err)
	}
	if !v.Valid {
		return -1, nil
	}
	return int(v.Int64), nil
}

func contiguousFromTop(rollbacks []rollbackEntry, dbVersion int) bool {
	expected := dbVersion
	for _, rb := range rollbacks {
		if rb.version != expected {
			return false
		}
		expected--
	}
	return true
}

func applyRollback(ctx context.Context, db *sql.DB, rb rollbackEntry) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return xerrors.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, rb.downSQL); err != nil {
		return xerrors.Errorf("execute down SQL: %w", err)
	}

	targetVersion := rb.version - 1
	if _, err := tx.ExecContext(ctx, `TRUNCATE schema_migrations`); err != nil {
		return xerrors.Errorf("truncate schema_migrations: %w", err)
	}
	if targetVersion >= 0 {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version, dirty) VALUES ($1, $2)`,
			targetVersion, false); err != nil {
			return xerrors.Errorf("set version: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM _develop.applied_migrations WHERE version = $1`,
		rb.version); err != nil {
		return xerrors.Errorf("remove tracking entry: %w", err)
	}

	return tx.Commit()
}

// captureDownSQL scans migration files on disk and stores both
// up and down SQL content in the tracking table for versions
// <= dbVersion.
func captureDownSQL(ctx context.Context, db *sql.DB, migrDir string, dbVersion int) error {
	entries, err := os.ReadDir(migrDir)
	if err != nil {
		return xerrors.Errorf("read migrations dir: %w", err)
	}

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".down.sql") || len(name) < 7 {
			continue
		}
		version, err := strconv.Atoi(name[:6])
		if err != nil || version > dbVersion {
			continue
		}

		downContent, err := os.ReadFile(filepath.Join(migrDir, name))
		if err != nil {
			return xerrors.Errorf("read %s: %w", name, err)
		}

		upName := strings.Replace(name, ".down.sql", ".up.sql", 1)
		upContent, err := os.ReadFile(filepath.Join(migrDir, upName))
		if err != nil {
			// Up file might not exist for some migrations.
			upContent = nil
		}

		_, err = db.ExecContext(ctx, `
			INSERT INTO _develop.applied_migrations (version, filename, up_sql, down_sql)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (version) DO UPDATE
			SET filename = EXCLUDED.filename, up_sql = EXCLUDED.up_sql, down_sql = EXCLUDED.down_sql
		`, version, name, string(upContent), string(downContent))
		if err != nil {
			return xerrors.Errorf("upsert version %d: %w", version, err)
		}
	}
	return nil
}

// formatDiff produces a simple line-based diff between two strings.
func formatDiff(labelA, labelB, a, b string) string {
	linesA := strings.Split(a, "\n")
	linesB := strings.Split(b, "\n")

	var out strings.Builder
	maxLines := len(linesA)
	if len(linesB) > maxLines {
		maxLines = len(linesB)
	}

	for i := 0; i < maxLines; i++ {
		var lineA, lineB string
		if i < len(linesA) {
			lineA = linesA[i]
		}
		if i < len(linesB) {
			lineB = linesB[i]
		}
		if lineA != lineB {
			if lineA != "" {
				_, _ = fmt.Fprintf(&out, "      - (%s) %s\n", labelA, lineA)
			}
			if lineB != "" {
				_, _ = fmt.Fprintf(&out, "      + (%s) %s\n", labelB, lineB)
			}
		}
	}
	return out.String()
}

// updateMigrationTracking connects to the running server's
// database and captures current migration state. Called after
// the server health check passes.
func updateMigrationTracking(ctx context.Context, _ slog.Logger, cfg *devConfig) error {
	pgURL := os.Getenv("CODER_PG_CONNECTION_URL")
	if pgURL == "" {
		var err error
		pgURL, err = builtinPostgresURL(cfg)
		if err != nil {
			return xerrors.Errorf("resolve builtin postgres URL: %w", err)
		}
	}

	db, err := connectDB(ctx, pgURL)
	if err != nil {
		return xerrors.Errorf("connect for tracking update: %w", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, trackingDDL); err != nil {
		return xerrors.Errorf("ensure tracking table: %w", err)
	}

	dbVersion, _, err := currentMigrationVersion(ctx, db)
	if err != nil {
		return xerrors.Errorf("get db version: %w", err)
	}
	if dbVersion < 0 {
		return nil
	}

	migrDir := filepath.Join(cfg.projectRoot, "coderd", "database", "migrations")
	return captureDownSQL(ctx, db, migrDir, dbVersion)
}

func builtinPostgresURL(cfg *devConfig) (string, error) {
	pgDir := filepath.Join(cfg.configDir, "postgres")

	portBytes, err := os.ReadFile(filepath.Join(pgDir, "port"))
	if err != nil {
		return "", xerrors.Errorf("read postgres port: %w", err)
	}
	port := strings.TrimSpace(string(portBytes))

	passwordBytes, err := os.ReadFile(filepath.Join(pgDir, "password"))
	if err != nil {
		return "", xerrors.Errorf("read postgres password: %w", err)
	}
	password := strings.TrimSpace(string(passwordBytes))

	return fmt.Sprintf(
		"postgres://coder@localhost:%s/coder?sslmode=disable&password=%s",
		port, url.QueryEscape(password)), nil
}

func resetSchema(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return xerrors.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range []string{
		`DROP SCHEMA IF EXISTS _develop CASCADE`,
		`DROP SCHEMA IF EXISTS public CASCADE`,
		`CREATE SCHEMA IF NOT EXISTS public`,
		`GRANT ALL ON SCHEMA public TO public`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return xerrors.Errorf("exec %q: %w", stmt, err)
		}
	}
	return tx.Commit()
}

func connectDB(ctx context.Context, pgURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", pgURL)
	if err != nil {
		return nil, xerrors.Errorf("open: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, xerrors.Errorf("ping: %w", err)
	}
	return db, nil
}

func startTempPostgresSetURL(ctx context.Context, logger slog.Logger, cfg *devConfig, pgURL *string) (func(), error) {
	pgDir := filepath.Join(cfg.configDir, "postgres")
	cleanStalePIDFile(filepath.Join(pgDir, "data"))

	passwordBytes, err := os.ReadFile(filepath.Join(pgDir, "password"))
	if err != nil {
		return nil, xerrors.Errorf("read postgres password: %w", err)
	}
	password := strings.TrimSpace(string(passwordBytes))

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, xerrors.Errorf("find ephemeral port: %w", err)
	}
	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return nil, xerrors.New("listener returned non-TCP addr")
	}
	port := tcpAddr.Port
	_ = listener.Close()

	ep := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().
			Version(embeddedpostgres.V13).
			BinariesPath(filepath.Join(pgDir, "bin")).
			CachePath(filepath.Join(pgDir, "cache")).
			DataPath(filepath.Join(pgDir, "data")).
			RuntimePath(filepath.Join(pgDir, "runtime")).
			Port(uint32(port)). //nolint:gosec // port from listener, fits uint32.
			Username("coder").
			Password(password).
			Database("coder").
			Logger(nil),
	)

	logger.Info(ctx, "starting temporary postgres for migration check",
		slog.F("port", port))
	if err := ep.Start(); err != nil {
		return nil, xerrors.Errorf("start embedded postgres: %w", err)
	}

	*pgURL = fmt.Sprintf(
		"postgres://coder@localhost:%d/coder?sslmode=disable&password=%s",
		port, url.QueryEscape(password))

	return func() {
		if err := ep.Stop(); err != nil {
			logger.Warn(ctx, "failed to stop temporary postgres",
				slog.Error(err))
		}
	}, nil
}

func cleanStalePIDFile(dataDir string) {
	pidPath := filepath.Join(dataDir, "postmaster.pid")
	content, err := os.ReadFile(pidPath)
	if err != nil {
		return
	}
	lines := strings.SplitN(string(content), "\n", 2)
	pid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		_ = os.Remove(pidPath)
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		_ = os.Remove(pidPath)
		return
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		_ = os.Remove(pidPath)
	}
}

func currentMigrationVersion(ctx context.Context, db *sql.DB) (int, bool, error) {
	var version int
	var dirty bool
	err := db.QueryRowContext(ctx,
		`SELECT version, dirty FROM schema_migrations LIMIT 1`,
	).Scan(&version, &dirty)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return -1, false, nil
		}
		var pgErr *pq.Error
		if xerrors.As(err, &pgErr) && pgErr.Code.Name() == "undefined_table" {
			return -1, false, nil
		}
		return -1, false, xerrors.Errorf("query schema_migrations: %w", err)
	}
	return version, dirty, nil
}

func formatVersions(rollbacks []rollbackEntry) string {
	var parts []string
	for _, rb := range rollbacks {
		parts = append(parts, strconv.Itoa(rb.version))
	}
	return strings.Join(parts, ", ")
}
