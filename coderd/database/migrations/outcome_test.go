package migrations_test

import (
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/testutil"
)

func TestOutcome(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	t.Run("Committed", func(t *testing.T) {
		t.Parallel()
		db := testSQLDB(t)
		require.NoError(t, migrations.Up(db, migrations.WithFS(migrationFS(`CREATE TABLE a (id int);`))))

		sink := testutil.NewFakeSink(t)
		require.NoError(t, migrations.Up(db,
			migrations.WithFS(migrationFS(`CREATE TABLE a (id int);`, `CREATE TABLE b (id int);`, `CREATE TABLE c (id int);`)),
			migrations.WithLogger(sink.Logger()),
		))

		line := onlyEntry(t, sink, "committed database migrations")
		require.Equal(t, slog.LevelInfo, line.Level)
		require.Equal(t, []string{
			"status", "count", "from_version", "to_version", "transaction_status", "total_duration_ms",
		}, fieldNames(line))
		require.Equal(t, "success", field(t, line, "status"))
		require.Equal(t, 2, field(t, line, "count"))
		require.Equal(t, 1, field(t, line, "from_version"))
		require.Equal(t, 3, field(t, line, "to_version"))
		require.Equal(t, "committed", field(t, line, "transaction_status"))
	})

	t.Run("CommittedFreshInstall", func(t *testing.T) {
		t.Parallel()
		db := testSQLDB(t)

		sink := testutil.NewFakeSink(t)
		require.NoError(t, migrations.Up(db,
			migrations.WithFS(migrationFS(`CREATE TABLE a (id int);`, `CREATE TABLE b (id int);`)),
			migrations.WithLogger(sink.Logger()),
		))

		// There was no version before, so from_version is omitted.
		line := onlyEntry(t, sink, "committed database migrations")
		require.Equal(t, slog.LevelInfo, line.Level)
		require.Equal(t, []string{
			"status", "count", "to_version", "transaction_status", "total_duration_ms",
		}, fieldNames(line))
		require.Equal(t, 2, field(t, line, "to_version"))
	})

	t.Run("RolledBack", func(t *testing.T) {
		t.Parallel()
		db := testSQLDB(t)
		require.NoError(t, migrations.Up(db, migrations.WithFS(migrationFS(`CREATE TABLE a (id int PRIMARY KEY);`))))

		sink := testutil.NewFakeSink(t)
		err := migrations.Up(db,
			migrations.WithFS(migrationFS(
				`CREATE TABLE a (id int PRIMARY KEY);`,
				`CREATE TABLE b (id int);`,
				`INSERT INTO a VALUES (1), (1);`,
			)),
			migrations.WithLogger(sink.Logger()),
		)
		require.Error(t, err)

		line := onlyEntry(t, sink, "migration failed, transaction rolled back, no changes saved")
		require.Equal(t, slog.LevelError, line.Level)
		require.Equal(t, []string{
			"status", "count", "version", "name", "schema_version", "sqlstate",
			"transaction_status", "total_duration_ms", "failed_duration_ms",
		}, fieldNames(line))
		require.Equal(t, "failure", field(t, line, "status"))
		require.Equal(t, 1, field(t, line, "count"))
		require.Equal(t, 3, field(t, line, "version"))
		require.Equal(t, "step_3", field(t, line, "name"))
		require.Equal(t, 1, field(t, line, "schema_version"))
		require.Equal(t, "23505", field(t, line, "sqlstate"))
		require.Equal(t, "aborted", field(t, line, "transaction_status"))

		// The error names the migration and gives PostgreSQL's message and
		// code, but neither the SQL nor PostgreSQL's detail text, which
		// contains the duplicate row's values.
		require.Contains(t, err.Error(),
			`up: migration 3 (step_3): migration failed: duplicate key value violates unique constraint "a_pkey" (sqlstate=23505)`)
		require.NotContains(t, err.Error(), "INSERT")
		require.NotContains(t, err.Error(), "Key (id)")
		require.NotContains(t, err.Error(), "\n")
		var pgErr *pq.Error
		require.ErrorAs(t, err, &pgErr)
	})

	t.Run("Dirty", func(t *testing.T) {
		t.Parallel()
		db := testSQLDB(t)
		require.NoError(t, migrations.Up(db, migrations.WithFS(migrationFS(`CREATE TABLE a (id int);`))))
		_, err := db.Exec(`UPDATE schema_migrations SET dirty = true`)
		require.NoError(t, err)

		sink := testutil.NewFakeSink(t)
		err = migrations.Up(db,
			migrations.WithFS(migrationFS(`CREATE TABLE a (id int);`, `CREATE TABLE b (id int);`)),
			migrations.WithLogger(sink.Logger()),
		)
		require.ErrorAs(t, err, new(migrate.ErrDirty))

		dirty := onlyEntry(t, sink, "database schema is dirty")
		require.Equal(t, slog.LevelError, dirty.Level)
		require.Equal(t, []string{"status", "version", "error"}, fieldNames(dirty))
		require.Equal(t, 1, field(t, dirty, "version"))
		require.Equal(t, "dirty database version 1", field(t, dirty, "error"))

		outcome := onlyEntry(t, sink, "database migration outcome: dirty schema")
		require.Equal(t, slog.LevelError, outcome.Level)
		require.Equal(t, []string{
			"status", "count", "schema_version", "total_duration_ms", "error",
		}, fieldNames(outcome))
		require.Equal(t, 1, field(t, outcome, "schema_version"))

		// A dirty schema stops the run before the pending migrations are
		// counted.
		require.Empty(t, sink.Entries(hasMessage("database migration required")))
	})
}
