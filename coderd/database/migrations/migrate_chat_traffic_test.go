package migrations_test

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/testutil"
)

// TestUpWithFSChatTrafficDuringUpgrade upgrades databases from earlier
// releases to the latest version with the real migrator while chatd traffic
// uses the chat tables (see chatTrafficCases), as in a rolling deploy.
//
// Neither side may be a deadlock victim. PostgreSQL runs the deadlock check
// in the backend that has waited deadlock_timeout and aborts that backend.
// The migrator takes the chat table locks with waits shorter than that, so
// an application backend never waits long enough to be aborted, and it gets
// no 40P01. A deadlock that aborts the migrator anywhere else makes UpWithFS
// retry, which logs a warning, so the test also requires that nothing was
// logged.
func TestUpWithFSChatTrafficDuringUpgrade(t *testing.T) {
	t.Parallel()

	allMigrations := migrationsFS(t, 0)
	// 585 is v2.37.3 and 605 is release/2.38. The others are the
	// versions just before migrations that alter chats or chat_messages.
	for _, from := range []uint{585, 600, 605, 607, 608} {
		t.Run(strconv.FormatUint(uint64(from), 10), func(t *testing.T) {
			t.Parallel()

			template, templateDB := openMigrationsDB(t)
			require.NoError(t, migrations.UpWithFS(templateDB, migrationsFS(t, from)))
			chatID := insertChatFixture(t, templateDB)
			// CREATE DATABASE ... TEMPLATE needs the template to have no
			// connections.
			require.NoError(t, templateDB.Close())

			for _, tc := range chatTrafficCases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					dsn, err := dbtestutil.Open(t, dbtestutil.WithDBFrom(template))
					require.NoError(t, err)
					appDB := openDB(t, dsn)
					// One connection, so that the PID read here is the
					// backend that runs the migrations.
					migDB := openDB(t, dsn)
					migDB.SetMaxOpenConns(1)
					migDB.SetMaxIdleConns(1)
					sink := testutil.NewFakeSink(t)

					var migPID int
					startMigration := func(t *testing.T, ctx context.Context) (int, <-chan error) {
						t.Helper()
						require.NoError(t, migDB.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&migPID))
						return migPID, async(func() error {
							return migrations.UpWithFS(migDB, allMigrations, migrations.WithLogger(slog.Make(sink)))
						})
					}
					tc.run(t, chatTrafficEnv{db: appDB, chatID: chatID, startMigration: startMigration})

					require.Empty(t, sink.Entries(), "the migrator logged, so it was retried as a deadlock victim")
					require.NoError(t, migrations.EnsureClean(migDB))
					var pid int
					require.NoError(t, migDB.QueryRow(`SELECT pg_backend_pid()`).Scan(&pid))
					require.Equal(t, migPID, pid, "the migrator used another backend than the one the test watched")
				})
			}
		})
	}
}

// TestUpWithFSChatTableLocks checks when UpWithFS takes the chat table locks.
func TestUpWithFSChatTableLocks(t *testing.T) {
	t.Parallel()

	// With nothing pending, a normal restart must not wait for chat
	// traffic.
	t.Run("NothingPending", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		dsn, err := dbtestutil.Open(t)
		require.NoError(t, err)
		db := openDB(t, dsn)

		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		// Conflicts with the ACCESS EXCLUSIVE lock, as a row lock taken
		// with SELECT ... FOR UPDATE does.
		_, err = tx.ExecContext(ctx, `LOCK TABLE chats IN ROW SHARE MODE`)
		require.NoError(t, err)

		migrated := async(func() error { return migrations.UpWithFS(db, migrationsFS(t, 0)) })
		require.NoError(t, testutil.TryReceive(ctx, t, migrated))
	})

	// The view lock must leave chats_expanded unchanged, whatever its
	// options.
	for _, tc := range []struct {
		name    string
		options string
	}{
		{name: "ViewWithoutOptions"},
		{name: "ViewWithOptions", options: "security_barrier=true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitMedium)
			dsn, err := dbtestutil.Open(t)
			require.NoError(t, err)
			db := openDB(t, dsn)
			if tc.options != "" {
				_, err = db.ExecContext(ctx, `ALTER VIEW chats_expanded SET (`+tc.options+`)`)
				require.NoError(t, err)
			}
			viewDef := func() (def string, options []string) {
				var opts sql.NullString
				require.NoError(t, db.QueryRowContext(ctx, `
					SELECT pg_get_viewdef('chats_expanded'::regclass), array_to_string(reloptions, ',')
					FROM pg_class WHERE oid = 'chats_expanded'::regclass`).Scan(&def, &opts))
				if opts.Valid {
					options = strings.Split(opts.String, ",")
				}
				return def, options
			}
			wantDef, wantOptions := viewDef()
			if tc.options != "" {
				require.Equal(t, []string{tc.options}, wantOptions)
			}

			migs := migrationsFS(t, 0)
			migs["999999_noop.up.sql"] = &fstest.MapFile{Data: []byte(`SELECT 1;`)}
			migs["999999_noop.down.sql"] = &fstest.MapFile{Data: []byte(`SELECT 1;`)}
			require.NoError(t, migrations.UpWithFS(db, migs))

			var version int
			require.NoError(t, db.QueryRowContext(ctx, `SELECT version FROM schema_migrations`).Scan(&version))
			require.Equal(t, 999999, version)
			gotDef, gotOptions := viewDef()
			require.Equal(t, wantDef, gotDef)
			require.Equal(t, wantOptions, gotOptions)
		})
	}
}

// migrationsFS returns the migration files of this package up to and
// including version maxVersion, or all of them if maxVersion is 0.
func migrationsFS(t *testing.T, maxVersion uint) fstest.MapFS {
	t.Helper()
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	migs := fstest.MapFS{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}
		prefix, _, ok := strings.Cut(name, "_")
		require.True(t, ok, name)
		version, err := strconv.ParseUint(prefix, 10, 64)
		require.NoError(t, err, name)
		if maxVersion != 0 && uint(version) > maxVersion {
			continue
		}
		data, err := os.ReadFile(name)
		require.NoError(t, err)
		migs[name] = &fstest.MapFile{Data: data}
	}
	require.NotEmpty(t, migs)
	return migs
}

// openMigrationsDB creates an empty database and returns its name and a
// connection pool to it.
func openMigrationsDB(t *testing.T) (string, *sql.DB) {
	t.Helper()
	dsn, err := dbtestutil.Open(t, dbtestutil.WithDBFrom("template1"))
	require.NoError(t, err)
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	return strings.TrimPrefix(u.Path, "/"), openDB(t, dsn)
}

func openDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}
