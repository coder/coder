package migrations_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/golang-migrate/migrate/v4/source/stub"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/sync/errgroup"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func TestMigrate(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.SkipNow()
		return
	}

	t.Run("Once", func(t *testing.T) {
		t.Parallel()

		db := testSQLDB(t)

		err := migrations.Up(db)
		require.NoError(t, err)
	})

	t.Run("Parallel", func(t *testing.T) {
		t.Parallel()

		db := testSQLDB(t)
		eg := errgroup.Group{}

		eg.Go(func() error {
			return migrations.Up(db)
		})
		eg.Go(func() error {
			return migrations.Up(db)
		})

		require.NoError(t, eg.Wait())
	})

	t.Run("Twice", func(t *testing.T) {
		t.Parallel()

		db := testSQLDB(t)

		err := migrations.Up(db)
		require.NoError(t, err)

		err = migrations.Up(db)
		require.NoError(t, err)
	})

	t.Run("UpDownUp", func(t *testing.T) {
		t.Parallel()

		db := testSQLDB(t)

		err := migrations.Up(db)
		require.NoError(t, err)

		err = migrations.Down(db)
		require.NoError(t, err)

		err = migrations.Up(db)
		require.NoError(t, err)
	})
}

func testSQLDB(t testing.TB) *sql.DB {
	t.Helper()

	connection, err := dbtestutil.Open(t)
	require.NoError(t, err)

	db, err := sql.Open("postgres", connection)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// dbtestutil.Open automatically runs migrations, but we want to actually test
	// migration behavior in this package.
	_, err = db.Exec(`DROP SCHEMA public CASCADE`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE SCHEMA public`)
	require.NoError(t, err)

	return db
}

// paralleltest linter doesn't correctly handle table-driven tests (https://github.com/kunwardeep/paralleltest/issues/8)
// nolint:paralleltest
func TestCheckLatestVersion(t *testing.T) {
	t.Parallel()

	type test struct {
		currentVersion   uint
		existingVersions []uint
		expectedResult   string
	}

	tests := []test{
		// successful cases
		{1, []uint{1}, ""},
		{3, []uint{1, 2, 3}, ""},
		{3, []uint{1, 3}, ""},

		// failure cases
		{1, []uint{1, 2}, "current version is 1, but later version 2 exists"},
		{2, []uint{1, 2, 3}, "current version is 2, but later version 3 exists"},
		{4, []uint{1, 2, 3}, "get previous migration: prev for version 4 : file does not exist"},
		{4, []uint{1, 2, 3, 5}, "get previous migration: prev for version 4 : file does not exist"},
	}

	for i, tc := range tests {
		i, tc := i, tc
		t.Run(fmt.Sprintf("entry %d", i), func(t *testing.T) {
			t.Parallel()

			driver, _ := stub.WithInstance(nil, &stub.Config{})
			stub, ok := driver.(*stub.Stub)
			require.True(t, ok)
			for _, version := range tc.existingVersions {
				stub.Migrations.Append(&source.Migration{
					Version:    version,
					Identifier: "",
					Direction:  source.Up,
					Raw:        "",
				})
			}

			err := migrations.CheckLatestVersion(driver, tc.currentVersion)
			var errMessage string
			if err != nil {
				errMessage = err.Error()
			}
			require.Equal(t, tc.expectedResult, errMessage)
		})
	}
}

func setupMigrate(t *testing.T, db *sql.DB, name, path string) (source.Driver, *migrate.Migrate) {
	t.Helper()

	ctx := context.Background()

	conn, err := db.Conn(ctx)
	require.NoError(t, err)

	dbDriver, err := migratepostgres.WithConnection(ctx, conn, &migratepostgres.Config{
		MigrationsTable: "test_migrate_" + name,
	})
	require.NoError(t, err)

	dirFS := os.DirFS(path)
	d, err := iofs.New(dirFS, ".")
	require.NoError(t, err)
	t.Cleanup(func() {
		d.Close()
	})

	m, err := migrate.NewWithInstance(name, d, "", dbDriver)
	require.NoError(t, err)
	t.Cleanup(func() {
		m.Close()
	})

	return d, m
}

type tableStats struct {
	mu sync.Mutex
	s  map[string]int
}

func (s *tableStats) Add(table string, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.s[table] += n
}

func (s *tableStats) Empty() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var m []string
	for table, n := range s.s {
		if n == 0 {
			m = append(m, table)
		}
	}
	return m
}

func TestMigrateUpWithFixtures(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.SkipNow()
		return
	}

	type testCase struct {
		name string
		path string

		// For determining if test case table stats
		// are used to determine test coverage.
		useStats bool
	}
	tests := []testCase{
		{
			name:     "fixtures",
			path:     filepath.Join("testdata", "fixtures"),
			useStats: true,
		},
		// More test cases added via glob below.
	}

	// Folders in testdata/full_dumps represent fixtures for a full
	// deployment of Coder.
	matches, err := filepath.Glob(filepath.Join("testdata", "full_dumps", "*"))
	require.NoError(t, err)
	for _, match := range matches {
		tests = append(tests, testCase{
			name:     filepath.Base(match),
			path:     match,
			useStats: true,
		})
	}

	// These tables are allowed to have zero rows for now,
	// but we should eventually add fixtures for them.
	ignoredTablesForStats := []string{
		"audit_logs",
		"external_auth_links",
		"group_members",
		"licenses",
		"replicas",
		"template_version_parameters",
		"workspace_build_parameters",
		"template_version_variables",
		"dbcrypt_keys", // having zero rows is a valid state for this table
		"template_version_workspace_tags",
		"notification_report_generator_logs",
	}
	s := &tableStats{s: make(map[string]int)}

	// This will run after all subtests have run and fail the test if
	// new tables have been added without covering them with fixtures.
	t.Cleanup(func() {
		emptyTables := s.Empty()
		slices.Sort(emptyTables)
		for _, table := range ignoredTablesForStats {
			i := slices.Index(emptyTables, table)
			if i >= 0 {
				emptyTables = slices.Delete(emptyTables, i, i+1)
			}
		}
		if len(emptyTables) > 0 {
			t.Log("The following tables have zero rows, consider adding fixtures for them or create a full database dump:")
			t.Errorf("tables have zero rows: %v", emptyTables)
			t.Log("See https://github.com/coder/coder/blob/main/docs/about/contributing/backend.md#database-fixtures-for-testing-migrations for more information")
		}
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := testSQLDB(t)

			// This test occasionally timed out in CI, which is understandable
			// considering the amount of migrations and fixtures we have.
			ctx := testutil.Context(t, testutil.WaitSuperLong)

			// Prepare database for stepping up.
			err := migrations.Down(db)
			require.NoError(t, err)

			// Initialize migrations for fixtures.
			fDriver, fMigrate := setupMigrate(t, db, tt.name, tt.path)

			nextStep, err := migrations.Stepper(db)
			require.NoError(t, err)

			var fixtureVer uint
			nextFixtureVer, err := fDriver.First()
			require.NoError(t, err)

			for {
				version, more, err := nextStep()
				require.NoError(t, err)

				if !more {
					// We reached the end of the migrations.
					break
				}

				if nextFixtureVer == version {
					err = fMigrate.Steps(1)
					require.NoError(t, err)
					fixtureVer = version

					nv, _ := fDriver.Next(nextFixtureVer)
					if nv > 0 {
						nextFixtureVer = nv
					}
				}

				t.Logf("migrated to version %d, fixture version %d", version, fixtureVer)
			}

			// Gather number of rows for all existing tables
			// at the end of the migrations and fixtures.
			var tables pq.StringArray
			err = db.QueryRowContext(ctx, `
				SELECT array_agg(tablename)
				FROM pg_catalog.pg_tables
				WHERE
					schemaname != 'information_schema'
					AND schemaname != 'pg_catalog'
					AND tablename NOT LIKE 'test_migrate_%'
			`).Scan(&tables)
			require.NoError(t, err)

			for _, table := range tables {
				var count int
				err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count)
				require.NoError(t, err)

				if tt.useStats {
					s.Add(table, count)
				}
			}

			// Test that migration down is successful after up.
			err = migrations.Down(db)
			require.NoError(t, err, "final migration down should be successful")
		})
	}
}

// TestMigration000362AggregateUsageEvents tests the migration that aggregates
// usage events into daily rows correctly.
func TestMigration000362AggregateUsageEvents(t *testing.T) {
	t.Parallel()

	const migrationVersion = 362

	// Similarly to the other test, this test will probably time out in CI.
	ctx := testutil.Context(t, testutil.WaitSuperLong)

	sqlDB := testSQLDB(t)
	db := database.New(sqlDB)

	// Migrate up to the migration before the one that aggregates usage events.
	next, err := migrations.Stepper(sqlDB)
	require.NoError(t, err)
	for {
		version, more, err := next()
		require.NoError(t, err)
		if !more {
			t.Fatalf("migration %d not found", migrationVersion)
		}
		if version == migrationVersion-1 {
			break
		}
	}

	locSydney, err := time.LoadLocation("Australia/Sydney")
	require.NoError(t, err)

	usageEvents := []struct {
		// The only possible event type is dc_managed_agents_v1 when this
		// migration gets applied.
		eventData []byte
		createdAt time.Time
	}{
		{
			eventData: []byte(`{"count": 41}`),
			createdAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			eventData: []byte(`{"count": 1}`),
			// 2025-01-01 in UTC
			createdAt: time.Date(2025, 1, 2, 8, 38, 57, 0, locSydney),
		},
		{
			eventData: []byte(`{"count": 1}`),
			createdAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
		},
	}
	expectedDailyRows := []struct {
		day       time.Time
		usageData []byte
	}{
		{
			day:       time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			usageData: []byte(`{"count": 42}`),
		},
		{
			day:       time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			usageData: []byte(`{"count": 1}`),
		},
	}

	for _, usageEvent := range usageEvents {
		err := db.InsertUsageEvent(ctx, database.InsertUsageEventParams{
			ID:        uuid.New().String(),
			EventType: "dc_managed_agents_v1",
			EventData: usageEvent.eventData,
			CreatedAt: usageEvent.createdAt,
		})
		require.NoError(t, err)
	}

	// Migrate up to the migration that aggregates usage events.
	version, _, err := next()
	require.NoError(t, err)
	require.EqualValues(t, migrationVersion, version)

	// Get all of the newly created daily rows. This query is not exposed in the
	// querier interface intentionally.
	rows, err := sqlDB.QueryContext(ctx, "SELECT day, event_type, usage_data FROM usage_events_daily ORDER BY day ASC")
	require.NoError(t, err, "perform query")
	defer rows.Close()
	var out []database.UsageEventsDaily
	for rows.Next() {
		var row database.UsageEventsDaily
		err := rows.Scan(&row.Day, &row.EventType, &row.UsageData)
		require.NoError(t, err, "scan row")
		out = append(out, row)
	}

	// Verify that the daily rows match our expectations.
	require.Len(t, out, len(expectedDailyRows))
	for i, row := range out {
		require.Equal(t, "dc_managed_agents_v1", row.EventType)
		// The read row might be `+0000` rather than `UTC` specifically, so just
		// ensure it's within 1 second of the expected time.
		require.WithinDuration(t, expectedDailyRows[i].day, row.Day, time.Second)
		require.JSONEq(t, string(expectedDailyRows[i].usageData), string(row.UsageData))
	}
}
