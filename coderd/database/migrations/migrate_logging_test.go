package migrations_test

import (
	"context"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/testutil"
)

func TestUpLogging(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.SkipNow()
		return
	}

	good := fstest.MapFS{
		"000001_create_a.up.sql":   {Data: []byte("CREATE TABLE a (id int);")},
		"000001_create_a.down.sql": {Data: []byte("DROP TABLE a;")},
		"000002_create_b.up.sql":   {Data: []byte("CREATE TABLE b (id int);")},
		"000002_create_b.down.sql": {Data: []byte("DROP TABLE b;")},
	}

	messages := func(sink *testutil.FakeSink) []string {
		var out []string
		for _, e := range sink.Entries() {
			out = append(out, e.Message)
		}
		return out
	}
	field := func(e slog.SinkEntry, name string) any {
		for _, f := range e.Fields {
			if f.Name == name {
				return f.Value
			}
		}
		return nil
	}

	t.Run("Committed", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db := testSQLDB(t)

		sink := testutil.NewFakeSink(t)
		require.NoError(t, migrations.UpWithFSAndLogger(ctx, db, good, sink.Logger()))
		require.Equal(t, []string{
			"database migrations required",
			"starting database migration",
			"database migration applied, pending commit",
			"starting database migration",
			"database migration applied, pending commit",
			"committed database migrations",
		}, messages(sink))
		entries := sink.Entries()
		require.Equal(t, true, field(entries[0], "fresh_database"))
		require.Nil(t, field(entries[0], "current_version"))
		require.Equal(t, "create_a", field(entries[1], "name"))
		require.Equal(t, 2, field(entries[5], "count"))

		sink = testutil.NewFakeSink(t)
		require.NoError(t, migrations.UpWithFSAndLogger(ctx, db, good, sink.Logger()))
		require.Equal(t, []string{"database schema is up to date"}, messages(sink))
	})

	t.Run("RolledBack", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db := testSQLDB(t)

		bad := fstest.MapFS{
			"000003_broken.up.sql":   {Data: []byte("SELECT * FROM does_not_exist;")},
			"000003_broken.down.sql": {Data: []byte("")},
		}
		for k, v := range good {
			bad[k] = v
		}

		sink := testutil.NewFakeSink(t)
		err := migrations.UpWithFSAndLogger(ctx, db, bad, sink.Logger(slog.LevelInfo))
		require.ErrorContains(t, err, "migration 3 (broken)")
		require.Empty(t, sink.Entries(func(e slog.SinkEntry) bool { return e.Level >= slog.LevelError }))

		rolledBack := sink.Entries(func(e slog.SinkEntry) bool { return e.Message == "rolled back database migrations" })
		require.Len(t, rolledBack, 1)
		require.Equal(t, 2, field(rolledBack[0], "count"))
		require.Equal(t, true, field(rolledBack[0], "fresh_database"))

		// Nothing was committed, so the schema is still empty.
		var exists bool
		require.NoError(t, db.QueryRowContext(ctx, "SELECT to_regclass('a') IS NOT NULL").Scan(&exists))
		require.False(t, exists)
	})

	t.Run("WaitsForLock", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db := testSQLDB(t)

		// Simulate another instance holding the migration lock.
		holder, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		_, err = holder.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", migrations.LockID)
		require.NoError(t, err)

		sink := testutil.NewFakeSink(t)
		done := make(chan error, 1)
		go func() {
			done <- migrations.UpWithFSAndLogger(ctx, db, good, sink.Logger())
		}()

		hasMessage := func(msg string) bool {
			return len(sink.Entries(func(e slog.SinkEntry) bool { return e.Message == msg })) > 0
		}
		testutil.Eventually(ctx, t, func(context.Context) bool {
			return hasMessage("waiting for database migration lock held by another instance")
		}, testutil.IntervalFast)
		require.False(t, hasMessage("acquired database migration lock"))

		require.NoError(t, holder.Rollback())
		require.NoError(t, testutil.RequireReceive(ctx, t, done))
		require.True(t, hasMessage("acquired database migration lock"))
		require.True(t, hasMessage("committed database migrations"))
	})

	// A failure that is not a SQL error leaves the transaction healthy, so the
	// migrations applied before it are committed rather than rolled back.
	t.Run("CommittedBeforeFailure", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db := testSQLDB(t)

		migs := openFailFS{MapFS: good, fail: "000002_create_b.up.sql"}
		sink := testutil.NewFakeSink(t)
		require.Error(t, migrations.UpWithFSAndLogger(ctx, db, migs, sink.Logger(slog.LevelInfo)))

		require.Empty(t, sink.Entries(func(e slog.SinkEntry) bool { return e.Message == "rolled back database migrations" }))
		committed := sink.Entries(func(e slog.SinkEntry) bool { return e.Message == "committed database migrations before failure" })
		require.Len(t, committed, 1)
		require.Equal(t, 1, field(committed[0], "count"))
		require.Equal(t, 1, field(committed[0], "to_version"))

		var version int
		require.NoError(t, db.QueryRowContext(ctx, "SELECT version FROM schema_migrations").Scan(&version))
		require.Equal(t, 1, version)
	})
}

// openFailFS fails to open one file, simulating a migration source error.
type openFailFS struct {
	fstest.MapFS
	fail string
}

func (f openFailFS) Open(name string) (fs.File, error) {
	if name == f.fail {
		return nil, xerrors.New("open failed")
	}
	return f.MapFS.Open(name)
}
