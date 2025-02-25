package dbtestutil_test

import (
	"database/sql"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func TestOpen(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("this test requires postgres")
	}

	connect, err := dbtestutil.Open(t)
	require.NoError(t, err)

	db, err := sql.Open("postgres", connect)
	require.NoError(t, err)
	err = db.Ping()
	require.NoError(t, err)
	err = db.Close()
	require.NoError(t, err)
}

func TestOpen_InvalidDBFrom(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("this test requires postgres")
	}

	_, err := dbtestutil.Open(t, dbtestutil.WithDBFrom("__invalid__"))
	require.Error(t, err)
	require.ErrorContains(t, err, "template database")
	require.ErrorContains(t, err, "does not exist")
}

func TestOpen_ValidDBFrom(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("this test requires postgres")
	}

	// first check if we can create a new template db
	dsn, err := dbtestutil.Open(t, dbtestutil.WithDBFrom(""))
	require.NoError(t, err)

	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	err = db.Ping()
	require.NoError(t, err)

	templateDBName := "tpl_" + migrations.GetMigrationsHash()[:32]
	tplDbExistsRes, err := db.Query("SELECT 1 FROM pg_database WHERE datname = $1", templateDBName)
	if err != nil {
		require.NoError(t, err)
	}
	require.True(t, tplDbExistsRes.Next())
	require.NoError(t, tplDbExistsRes.Close())

	// now populate the db with some data and use it as a new template db
	// to verify that dbtestutil.Open respects WithDBFrom
	_, err = db.Exec("CREATE TABLE my_wonderful_table (id serial PRIMARY KEY, name text)")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO my_wonderful_table (name) VALUES ('test')")
	require.NoError(t, err)

	rows, err := db.Query("SELECT current_database()")
	require.NoError(t, err)
	require.True(t, rows.Next())
	var freshTemplateDBName string
	require.NoError(t, rows.Scan(&freshTemplateDBName))
	require.NoError(t, rows.Close())
	require.NoError(t, db.Close())

	for i := 0; i < 10; i++ {
		db, err := sql.Open("postgres", dsn)
		require.NoError(t, err)
		require.NoError(t, db.Ping())
		require.NoError(t, db.Close())
	}

	// now create a new db from the template db
	newDsn, err := dbtestutil.Open(t, dbtestutil.WithDBFrom(freshTemplateDBName))
	require.NoError(t, err)

	newDb, err := sql.Open("postgres", newDsn)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, newDb.Close())
	})

	rows, err = newDb.Query("SELECT 1 FROM my_wonderful_table WHERE name = 'test'")
	require.NoError(t, err)
	require.True(t, rows.Next())
	require.NoError(t, rows.Close())
}
