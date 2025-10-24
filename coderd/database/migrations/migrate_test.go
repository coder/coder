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

func TestMigration000387MigrateTaskWorkspaces(t *testing.T) {
	t.Parallel()

	// This test verifies the migration of task workspaces to the new tasks data model.
	// Test cases:
	//
	// Task 1 (ws1) - Basic case:
	//   - Single build with has_ai_task=true, prompt, and parameters
	//   - Verifies: all task fields are populated correctly
	//
	// Task 2 (ws2) - No AI Prompt parameter:
	//   - Single build with has_ai_task=true but NO AI Prompt parameter
	//   - Verifies: prompt defaults to empty string (tests LEFT JOIN for optional prompt)
	//
	// Task 3 (ws3) - Latest build is stop:
	//   - Build 1: start with agents/apps and prompt
	//   - Build 2: stop build (references same app via ai_task_sidebar_app_id)
	//   - Verifies: twa uses latest build number with agents/apps from that build's ai_task_sidebar_app_id
	//
	// Antagonists - Should NOT be migrated:
	//   - Regular workspace without has_ai_task flag
	//   - Deleted workspace (w.deleted = true)

	const migrationVersion = 387

	ctx := testutil.Context(t, testutil.WaitLong)
	sqlDB := testSQLDB(t)

	// Migrate up to the migration before the task workspace migration.
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

	now := time.Now().UTC().Truncate(time.Microsecond)
	deletingAt := now.Add(24 * time.Hour).Truncate(time.Microsecond)

	// Define all IDs upfront.
	orgID := uuid.New()
	userID := uuid.New()
	templateID := uuid.New()
	templateVersionID := uuid.New()
	templateJobID := uuid.New()

	// Task workspace 1: basic case with prompt and parameters.
	ws1ID := uuid.New()
	ws1Build1JobID := uuid.New()
	ws1Build1ID := uuid.New()
	ws1Resource1ID := uuid.New()
	ws1Agent1ID := uuid.New()
	ws1App1ID := uuid.New()

	// Task workspace 2: no AI Prompt parameter.
	ws2ID := uuid.New()
	ws2Build1JobID := uuid.New()
	ws2Build1ID := uuid.New()
	ws2Resource1ID := uuid.New()
	ws2Agent1ID := uuid.New()
	ws2App1ID := uuid.New()

	// Task workspace 3: has both start and stop builds.
	ws3ID := uuid.New()
	ws3Build1JobID := uuid.New()
	ws3Build1ID := uuid.New()
	ws3Resource1ID := uuid.New()
	ws3Agent1ID := uuid.New()
	ws3App1ID := uuid.New()
	ws3Build2JobID := uuid.New()
	ws3Build2ID := uuid.New()
	ws3Resource2ID := uuid.New()

	// Antagonist 1: deleted workspace.
	wsAntDeletedID := uuid.New()
	wsAntDeletedBuild1JobID := uuid.New()
	wsAntDeletedBuild1ID := uuid.New()
	wsAntDeletedResource1ID := uuid.New()
	wsAntDeletedAgent1ID := uuid.New()
	wsAntDeletedApp1ID := uuid.New()

	// Antagonist 2: regular workspace without has_ai_task.
	wsAntID := uuid.New()
	wsAntBuild1JobID := uuid.New()
	wsAntBuild1ID := uuid.New()

	// Create all fixtures in a single transaction.
	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()

	// Execute fixture setup as individual statements.
	fixtures := []struct {
		query string
		args  []any
	}{
		// Setup organization, user, and template.
		{
			`INSERT INTO organizations (id, name, display_name, description, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)`,
			[]any{orgID, "test-org", "Test Org", "Test Org", now, now},
		},
		{
			`INSERT INTO users (id, username, email, hashed_password, created_at, updated_at, status, rbac_roles, login_type) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			[]any{userID, "testuser", "test@example.com", []byte{}, now, now, "active", []byte("{}"), "password"},
		},
		{
			`INSERT INTO provisioner_jobs (id, created_at, updated_at, started_at, completed_at, error, organization_id, initiator_id, provisioner, storage_method, file_id, type, input, tags) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			[]any{templateJobID, now, now, now, now, "", orgID, userID, "terraform", "file", uuid.New(), "template_version_import", []byte("{}"), []byte("{}")},
		},
		{
			`INSERT INTO template_versions (id, organization_id, name, readme, created_at, updated_at, job_id, created_by) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			[]any{templateVersionID, orgID, "v1.0", "Test template", now, now, templateJobID, userID},
		},
		{
			`INSERT INTO templates (id, organization_id, name, created_at, updated_at, provisioner, active_version_id, created_by) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			[]any{templateID, orgID, "test-template", now, now, "terraform", templateVersionID, userID},
		},
		{
			`UPDATE template_versions SET template_id = $1 WHERE id = $2`,
			[]any{templateID, templateVersionID},
		},

		// Task workspace 1 is a normal start build.
		{
			`INSERT INTO workspaces (id, created_at, updated_at, owner_id, organization_id, template_id, deleted, name, last_used_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			[]any{ws1ID, now, now, userID, orgID, templateID, false, "task-ws-1", now},
		},
		{
			`INSERT INTO provisioner_jobs (id, created_at, updated_at, started_at, completed_at, error, organization_id, initiator_id, provisioner, storage_method, file_id, type, input, tags) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			[]any{ws1Build1JobID, now, now, now, now, "", orgID, userID, "terraform", "file", uuid.New(), "workspace_build", []byte("{}"), []byte("{}")},
		},
		{
			`INSERT INTO workspace_resources (id, created_at, job_id, transition, type, name, hide, icon, daily_cost, instance_type) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			[]any{ws1Resource1ID, now, ws1Build1JobID, "start", "docker_container", "main", false, "", 0, ""},
		},
		{
			`INSERT INTO workspace_agents (id, created_at, updated_at, name, resource_id, auth_token, architecture, operating_system, directory, connection_timeout_seconds, lifecycle_state, logs_length, logs_overflowed) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
			[]any{ws1Agent1ID, now, now, "agent1", ws1Resource1ID, uuid.New(), "amd64", "linux", "/home/coder", 120, "ready", 0, false},
		},
		{
			`INSERT INTO workspace_apps (id, created_at, agent_id, slug, display_name, icon, command, url, subdomain, external) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			[]any{ws1App1ID, now, ws1Agent1ID, "code-server", "Code Server", "", "", "http://localhost:8080", false, false},
		},
		{
			`INSERT INTO workspace_builds (id, created_at, updated_at, workspace_id, template_version_id, build_number, transition, initiator_id, provisioner_state, job_id, deadline, reason, daily_cost, max_deadline, has_ai_task, ai_task_sidebar_app_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
			[]any{ws1Build1ID, now, now, ws1ID, templateVersionID, 1, "start", userID, []byte{}, ws1Build1JobID, now.Add(8 * time.Hour), "initiator", 0, now.Add(8 * time.Hour), true, ws1App1ID},
		},
		{
			`INSERT INTO workspace_build_parameters (workspace_build_id, name, value) VALUES ($1, $2, $3)`,
			[]any{ws1Build1ID, "AI Prompt", "Build a web server"},
		},
		{
			`INSERT INTO workspace_build_parameters (workspace_build_id, name, value) VALUES ($1, $2, $3)`,
			[]any{ws1Build1ID, "region", "us-east-1"},
		},
		{
			`INSERT INTO workspace_build_parameters (workspace_build_id, name, value) VALUES ($1, $2, $3)`,
			[]any{ws1Build1ID, "instance_type", "t2.micro"},
		},

		// Task workspace 2: no AI Prompt parameter (tests LEFT JOIN).
		{
			`INSERT INTO workspaces (id, created_at, updated_at, owner_id, organization_id, template_id, deleted, name, last_used_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			[]any{ws2ID, now, now, userID, orgID, templateID, false, "task-ws-2-no-prompt", now},
		},
		{
			`INSERT INTO provisioner_jobs (id, created_at, updated_at, started_at, completed_at, error, organization_id, initiator_id, provisioner, storage_method, file_id, type, input, tags) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			[]any{ws2Build1JobID, now, now, now, now, "", orgID, userID, "terraform", "file", uuid.New(), "workspace_build", []byte("{}"), []byte("{}")},
		},
		{
			`INSERT INTO workspace_resources (id, created_at, job_id, transition, type, name, hide, icon, daily_cost, instance_type) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			[]any{ws2Resource1ID, now, ws2Build1JobID, "start", "docker_container", "main", false, "", 0, ""},
		},
		{
			`INSERT INTO workspace_agents (id, created_at, updated_at, name, resource_id, auth_token, architecture, operating_system, directory, connection_timeout_seconds, lifecycle_state, logs_length, logs_overflowed) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
			[]any{ws2Agent1ID, now, now, "agent2", ws2Resource1ID, uuid.New(), "amd64", "linux", "/home/coder", 120, "ready", 0, false},
		},
		{
			`INSERT INTO workspace_apps (id, created_at, agent_id, slug, display_name, icon, command, url, subdomain, external) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			[]any{ws2App1ID, now, ws2Agent1ID, "terminal", "Terminal", "", "", "http://localhost:3000", false, false},
		},
		{
			`INSERT INTO workspace_builds (id, created_at, updated_at, workspace_id, template_version_id, build_number, transition, initiator_id, provisioner_state, job_id, deadline, reason, daily_cost, max_deadline, has_ai_task, ai_task_sidebar_app_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
			[]any{ws2Build1ID, now, now, ws2ID, templateVersionID, 1, "start", userID, []byte{}, ws2Build1JobID, now.Add(8 * time.Hour), "initiator", 0, now.Add(8 * time.Hour), true, ws2App1ID},
		},
		// Note: No AI Prompt parameter for ws2 - this tests the LEFT JOIN for optional prompt.

		// Task workspace 3: has both start and stop builds.
		{
			`INSERT INTO workspaces (id, created_at, updated_at, owner_id, organization_id, template_id, deleted, name, last_used_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			[]any{ws3ID, now, now, userID, orgID, templateID, false, "task-ws-3-stop", now},
		},
		{
			`INSERT INTO provisioner_jobs (id, created_at, updated_at, started_at, completed_at, error, organization_id, initiator_id, provisioner, storage_method, file_id, type, input, tags) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			[]any{ws3Build1JobID, now, now, now, now, "", orgID, userID, "terraform", "file", uuid.New(), "workspace_build", []byte("{}"), []byte("{}")},
		},
		{
			`INSERT INTO workspace_resources (id, created_at, job_id, transition, type, name, hide, icon, daily_cost, instance_type) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			[]any{ws3Resource1ID, now, ws3Build1JobID, "start", "docker_container", "main", false, "", 0, ""},
		},
		{
			`INSERT INTO workspace_agents (id, created_at, updated_at, name, resource_id, auth_token, architecture, operating_system, directory, connection_timeout_seconds, lifecycle_state, logs_length, logs_overflowed) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
			[]any{ws3Agent1ID, now, now, "agent3", ws3Resource1ID, uuid.New(), "amd64", "linux", "/home/coder", 120, "ready", 0, false},
		},
		{
			`INSERT INTO workspace_apps (id, created_at, agent_id, slug, display_name, icon, command, url, subdomain, external) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			[]any{ws3App1ID, now, ws3Agent1ID, "app3", "App3", "", "", "http://localhost:5000", false, false},
		},
		{
			`INSERT INTO workspace_builds (id, created_at, updated_at, workspace_id, template_version_id, build_number, transition, initiator_id, provisioner_state, job_id, deadline, reason, daily_cost, max_deadline, has_ai_task, ai_task_sidebar_app_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
			[]any{ws3Build1ID, now, now, ws3ID, templateVersionID, 1, "start", userID, []byte{}, ws3Build1JobID, now.Add(8 * time.Hour), "initiator", 0, now.Add(8 * time.Hour), true, ws3App1ID},
		},
		{
			`INSERT INTO workspace_build_parameters (workspace_build_id, name, value) VALUES ($1, $2, $3)`,
			[]any{ws3Build1ID, "AI Prompt", "Task with stop build"},
		},
		{
			`INSERT INTO provisioner_jobs (id, created_at, updated_at, started_at, completed_at, error, organization_id, initiator_id, provisioner, storage_method, file_id, type, input, tags) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			[]any{ws3Build2JobID, now, now, now, now, "", orgID, userID, "terraform", "file", uuid.New(), "workspace_build", []byte("{}"), []byte("{}")},
		},
		{
			`INSERT INTO workspace_resources (id, created_at, job_id, transition, type, name, hide, icon, daily_cost, instance_type) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			[]any{ws3Resource2ID, now, ws3Build2JobID, "stop", "docker_container", "main", false, "", 0, ""},
		},
		{
			`INSERT INTO workspace_builds (id, created_at, updated_at, workspace_id, template_version_id, build_number, transition, initiator_id, provisioner_state, job_id, deadline, reason, daily_cost, max_deadline, has_ai_task, ai_task_sidebar_app_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
			[]any{ws3Build2ID, now, now, ws3ID, templateVersionID, 2, "stop", userID, []byte{}, ws3Build2JobID, now.Add(8 * time.Hour), "initiator", 0, now.Add(8 * time.Hour), true, ws3App1ID},
		},

		// Antagonist 1: deleted workspace.
		{
			`INSERT INTO workspaces (id, created_at, updated_at, owner_id, organization_id, template_id, deleted, name, last_used_at, deleting_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			[]any{wsAntDeletedID, now, now, userID, orgID, templateID, true, "deleted-task-workspace", now, deletingAt},
		},
		{
			`INSERT INTO provisioner_jobs (id, created_at, updated_at, started_at, completed_at, error, organization_id, initiator_id, provisioner, storage_method, file_id, type, input, tags) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			[]any{wsAntDeletedBuild1JobID, now, now, now, now, "", orgID, userID, "terraform", "file", uuid.New(), "workspace_build", []byte("{}"), []byte("{}")},
		},
		{
			`INSERT INTO workspace_resources (id, created_at, job_id, transition, type, name, hide, icon, daily_cost, instance_type) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			[]any{wsAntDeletedResource1ID, now, wsAntDeletedBuild1JobID, "start", "docker_container", "main", false, "", 0, ""},
		},
		{
			`INSERT INTO workspace_agents (id, created_at, updated_at, name, resource_id, auth_token, architecture, operating_system, directory, connection_timeout_seconds, lifecycle_state, logs_length, logs_overflowed) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
			[]any{wsAntDeletedAgent1ID, now, now, "agent-deleted", wsAntDeletedResource1ID, uuid.New(), "amd64", "linux", "/home/coder", 120, "ready", 0, false},
		},
		{
			`INSERT INTO workspace_apps (id, created_at, agent_id, slug, display_name, icon, command, url, subdomain, external) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			[]any{wsAntDeletedApp1ID, now, wsAntDeletedAgent1ID, "app-deleted", "AppDeleted", "", "", "http://localhost:6000", false, false},
		},
		{
			`INSERT INTO workspace_builds (id, created_at, updated_at, workspace_id, template_version_id, build_number, transition, initiator_id, provisioner_state, job_id, deadline, reason, daily_cost, max_deadline, has_ai_task, ai_task_sidebar_app_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
			[]any{wsAntDeletedBuild1ID, now, now, wsAntDeletedID, templateVersionID, 1, "start", userID, []byte{}, wsAntDeletedBuild1JobID, now.Add(8 * time.Hour), "initiator", 0, now.Add(8 * time.Hour), true, wsAntDeletedApp1ID},
		},
		{
			`INSERT INTO workspace_build_parameters (workspace_build_id, name, value) VALUES ($1, $2, $3)`,
			[]any{wsAntDeletedBuild1ID, "AI Prompt", "Should not migrate deleted"},
		},

		// Antagonist 2: regular workspace without has_ai_task.
		{
			`INSERT INTO workspaces (id, created_at, updated_at, owner_id, organization_id, template_id, deleted, name, last_used_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			[]any{wsAntID, now, now, userID, orgID, templateID, false, "regular-workspace", now},
		},
		{
			`INSERT INTO provisioner_jobs (id, created_at, updated_at, started_at, completed_at, error, organization_id, initiator_id, provisioner, storage_method, file_id, type, input, tags) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			[]any{wsAntBuild1JobID, now, now, now, now, "", orgID, userID, "terraform", "file", uuid.New(), "workspace_build", []byte("{}"), []byte("{}")},
		},
		{
			`INSERT INTO workspace_builds (id, created_at, updated_at, workspace_id, template_version_id, build_number, transition, initiator_id, provisioner_state, job_id, deadline, reason, daily_cost, max_deadline) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			[]any{wsAntBuild1ID, now, now, wsAntID, templateVersionID, 1, "start", userID, []byte{}, wsAntBuild1JobID, now.Add(8 * time.Hour), "initiator", 0, now.Add(8 * time.Hour)},
		},
	}

	for _, fixture := range fixtures {
		_, err = tx.ExecContext(ctx, fixture.query, fixture.args...)
		require.NoError(t, err)
	}

	err = tx.Commit()
	require.NoError(t, err)

	// Run the migration.
	version, _, err := next()
	require.NoError(t, err)
	require.EqualValues(t, migrationVersion, version)

	// Should have exactly 3 tasks (not antagonists).
	var taskCount int
	err = sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks").Scan(&taskCount)
	require.NoError(t, err)
	require.Equal(t, 3, taskCount, "should have created 3 tasks from workspaces")

	// Verify task 1, normal start build.
	var task1 struct {
		id                 uuid.UUID
		name               string
		workspaceID        uuid.UUID
		templateVersionID  uuid.UUID
		prompt             string
		templateParameters []byte
		createdAt          time.Time
		deletedAt          *time.Time
	}
	err = sqlDB.QueryRowContext(ctx, `
		SELECT id, name, workspace_id, template_version_id, prompt, template_parameters, created_at, deleted_at
		FROM tasks WHERE workspace_id = $1
	`, ws1ID).Scan(&task1.id, &task1.name, &task1.workspaceID, &task1.templateVersionID, &task1.prompt, &task1.templateParameters, &task1.createdAt, &task1.deletedAt)
	require.NoError(t, err)
	require.Equal(t, "task-ws-1", task1.name)
	require.Equal(t, "Build a web server", task1.prompt)
	require.JSONEq(t, `{"region":"us-east-1","instance_type":"t2.micro"}`, string(task1.templateParameters))
	require.Nil(t, task1.deletedAt)

	// Verify task_workspace_apps for task 1.
	var twa1 struct {
		buildNumber int32
		agentID     uuid.UUID
		appID       uuid.UUID
	}
	err = sqlDB.QueryRowContext(ctx, `
		SELECT workspace_build_number, workspace_agent_id, workspace_app_id
		FROM task_workspace_apps WHERE task_id = $1
	`, task1.id).Scan(&twa1.buildNumber, &twa1.agentID, &twa1.appID)
	require.NoError(t, err)
	require.Equal(t, int32(1), twa1.buildNumber)
	require.Equal(t, ws1Agent1ID, twa1.agentID)
	require.Equal(t, ws1App1ID, twa1.appID)

	// Verify task 2, no AI Prompt parameter.
	var task2 struct {
		id                 uuid.UUID
		name               string
		prompt             string
		templateParameters []byte
		deletedAt          *time.Time
	}
	err = sqlDB.QueryRowContext(ctx, `
		SELECT id, name, prompt, template_parameters, deleted_at
		FROM tasks WHERE workspace_id = $1
	`, ws2ID).Scan(&task2.id, &task2.name, &task2.prompt, &task2.templateParameters, &task2.deletedAt)
	require.NoError(t, err)
	require.Equal(t, "task-ws-2-no-prompt", task2.name)
	require.Equal(t, "", task2.prompt, "prompt should be empty string when no AI Prompt parameter")
	require.JSONEq(t, `{}`, string(task2.templateParameters), "no parameters")
	require.Nil(t, task2.deletedAt)

	// Verify task_workspace_apps for task 2.
	var twa2 struct {
		buildNumber int32
		agentID     uuid.UUID
		appID       uuid.UUID
	}
	err = sqlDB.QueryRowContext(ctx, `
		SELECT workspace_build_number, workspace_agent_id, workspace_app_id
		FROM task_workspace_apps WHERE task_id = $1
	`, task2.id).Scan(&twa2.buildNumber, &twa2.agentID, &twa2.appID)
	require.NoError(t, err)
	require.Equal(t, int32(1), twa2.buildNumber)
	require.Equal(t, ws2Agent1ID, twa2.agentID)
	require.Equal(t, ws2App1ID, twa2.appID)

	// Verify task 3, has both start and stop builds.
	var task3 struct {
		id                 uuid.UUID
		name               string
		prompt             string
		templateParameters []byte
		templateVersionID  uuid.UUID
		deletedAt          *time.Time
	}
	err = sqlDB.QueryRowContext(ctx, `
		SELECT id, name, prompt, template_parameters, template_version_id, deleted_at
		FROM tasks WHERE workspace_id = $1
	`, ws3ID).Scan(&task3.id, &task3.name, &task3.prompt, &task3.templateParameters, &task3.templateVersionID, &task3.deletedAt)
	require.NoError(t, err)
	require.Equal(t, "task-ws-3-stop", task3.name)
	require.Equal(t, "Task with stop build", task3.prompt)
	require.JSONEq(t, `{}`, string(task3.templateParameters), "no other parameters")
	require.Equal(t, templateVersionID, task3.templateVersionID)
	require.Nil(t, task3.deletedAt)

	// Verify task_workspace_apps for task 3 uses latest build and its ai_task_sidebar_app_id.
	var twa3 struct {
		buildNumber int32
		agentID     uuid.UUID
		appID       uuid.UUID
	}
	err = sqlDB.QueryRowContext(ctx, `
		SELECT workspace_build_number, workspace_agent_id, workspace_app_id
		FROM task_workspace_apps WHERE task_id = $1
	`, task3.id).Scan(&twa3.buildNumber, &twa3.agentID, &twa3.appID)
	require.NoError(t, err)
	require.Equal(t, int32(2), twa3.buildNumber, "should use latest build number")
	require.Equal(t, ws3Agent1ID, twa3.agentID, "should use agent from latest build's ai_task_sidebar_app_id")
	require.Equal(t, ws3App1ID, twa3.appID, "should use app from latest build's ai_task_sidebar_app_id")

	// Verify antagonists should NOT be migrated.
	var antCount int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM tasks
		WHERE workspace_id IN ($1, $2)
	`, wsAntDeletedID, wsAntID).Scan(&antCount)
	require.NoError(t, err)
	require.Equal(t, 0, antCount, "antagonist workspaces (deleted and regular) should not be migrated")
}
