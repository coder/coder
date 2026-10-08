package migrations_test

// Scenario tests for UpWithLogger's structured progress and outcome
// logging (RFC: Improve Coder Upgrade Visibility, requirements 1 and 2).
// Each scenario renders real human-readable log output (the same renderer
// coder server uses) to test-output/<scenario>.log, alongside stdout, so
// the rendered lines can be read back and compared against the expected
// operator-facing transcripts.

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"

	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/testutil"
)

// scenarioLogger renders real human-readable log output to both the
// scenario's log file and stdout (so it shows up in `go test -v` too).
func scenarioLogger(t *testing.T, scenario string) slog.Logger {
	t.Helper()
	dir := "test-output"
	require.NoError(t, os.MkdirAll(dir, 0o755))
	f, err := os.Create(filepath.Join(dir, scenario+".log"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })

	w := io.MultiWriter(f, os.Stdout)
	return slog.Make(sloghuman.Sink(w)).Leveled(slog.LevelDebug)
}

func TestUpWithLoggerFreshInstall(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	db := testSQLDB(t)
	logger := scenarioLogger(t, "fresh-install")

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	err := migrations.UpWithLogger(ctx, db, logger.Named("migrations"))
	require.NoError(t, err)
}

func TestUpWithLoggerUpToDate(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	db := testSQLDB(t)
	ctx := testutil.Context(t, testutil.WaitSuperLong)

	// First run brings it fully up to date; not the scenario under test,
	// so log it under its own name for reference.
	setupLogger := scenarioLogger(t, "up-to-date-setup")
	require.NoError(t, migrations.UpWithLogger(ctx, db, setupLogger.Named("migrations")))

	// Second run against the same, now-current database is the scenario:
	// it should log nothing but "database schema is up to date".
	logger := scenarioLogger(t, "up-to-date")
	err := migrations.UpWithLogger(ctx, db, logger.Named("migrations"))
	require.NoError(t, err)
}
