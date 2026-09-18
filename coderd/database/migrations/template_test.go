package migrations_test

import (
	"database/sql"
	"fmt"
	"os"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/migrations"
)

// The standalone backfill tests share this pristine migration prefix.
const migrationTemplateVersion = 503

var migrationTemplate struct {
	sync.Mutex
	name     string
	cleanups []func()
}

type migrationTestMain struct {
	*testing.M
}

func (m migrationTestMain) Run() int {
	defer func() {
		for _, cleanup := range slices.Backward(migrationTemplate.cleanups) {
			cleanup()
		}
	}()
	return m.M.Run()
}

// Template databases outlive individual tests, but not the test process.
type migrationTemplateTB struct {
	dbtestutil.TBSubset
}

func (migrationTemplateTB) Cleanup(fn func()) {
	migrationTemplate.cleanups = append(migrationTemplate.cleanups, fn)
}

func (migrationTemplateTB) Logf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func migrationTemplateName(t testing.TB) string {
	t.Helper()
	migrationTemplate.Lock()
	defer migrationTemplate.Unlock()
	if migrationTemplate.name != "" {
		return migrationTemplate.name
	}

	params, err := dbtestutil.DefaultBroker.Create(migrationTemplateTB{t}, dbtestutil.WithDBFrom("template1"))
	require.NoError(t, err)
	db, err := sql.Open("postgres", params.DSN())
	require.NoError(t, err)
	defer db.Close()

	next, err := migrations.Stepper(db)
	require.NoError(t, err)
	for {
		version, more, err := next()
		require.NoError(t, err)
		require.True(t, more, "migration template version not found")
		if version == migrationTemplateVersion {
			break
		}
	}
	require.NoError(t, db.Close())
	migrationTemplate.name = params.DBName
	return migrationTemplate.name
}

func testSQLDBAtVersion(t testing.TB, target uint) *sql.DB {
	t.Helper()
	require.GreaterOrEqual(t, target, uint(migrationTemplateVersion))
	db := testSQLDB(t, dbtestutil.WithDBFrom(migrationTemplateName(t)))
	if target > migrationTemplateVersion {
		next, err := migrations.Stepper(db)
		require.NoError(t, err)
		for version := uint(migrationTemplateVersion); version < target; {
			var more bool
			version, more, err = next()
			require.NoError(t, err)
			require.Truef(t, more, "migration %d not found", target)
		}
	}
	var version uint
	var dirty bool
	require.NoError(t, db.QueryRow("SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty))
	require.Equal(t, target, version)
	require.False(t, dirty)
	return db
}

func TestMigrationTemplateIsolation(t *testing.T) {
	t.Parallel()

	first := testSQLDBAtVersion(t, migrationTemplateVersion)
	_, err := first.Exec("CREATE TABLE migration_template_isolation (id int)")
	require.NoError(t, err)

	second := testSQLDBAtVersion(t, migrationTemplateVersion+1)
	var table sql.NullString
	require.NoError(t, second.QueryRow("SELECT to_regclass('migration_template_isolation')::text").Scan(&table))
	require.False(t, table.Valid, "clones must not inherit another test's schema changes")

	var version uint
	require.NoError(t, first.QueryRow("SELECT version FROM schema_migrations").Scan(&version))
	require.EqualValues(t, migrationTemplateVersion, version, "migrating a clone must not advance other clones")
}
