package migrations_test

import (
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/quartz"
)

// TestOutcome covers the outcome line of runs through Up that do not commit.
// TestReporter's cases cover the committed line.
func TestOutcome(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	t.Run("RolledBack", func(t *testing.T) {
		t.Parallel()
		db := testSQLDB(t)
		require.NoError(t, migrations.Up(db, migrations.WithFS(migrationFS(`CREATE TABLE a (id int PRIMARY KEY);`))))

		sink := migrations.NewGoldenSink()
		fsys := migrationFS(
			`CREATE TABLE a (id int PRIMARY KEY);`,
			`CREATE TABLE b (id int);`,
			`INSERT INTO a VALUES (1), (1);`,
		)
		err := migrations.Up(db, testOptions(sink, quartz.NewMock(t), "test/rolled-back", fsys)...)
		require.Error(t, err)

		requireGolden(t, sink, "rolled_back")
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

		sink := migrations.NewGoldenSink()
		fsys := migrationFS(`CREATE TABLE a (id int);`, `CREATE TABLE b (id int);`)
		err = migrations.Up(db, testOptions(sink, quartz.NewMock(t), "test/dirty", fsys)...)
		require.ErrorAs(t, err, new(migrate.ErrDirty))

		// A dirty schema stops the run before the pending migrations are
		// counted, so there is no "migration required" line.
		requireGolden(t, sink, "dirty")
	})
}
