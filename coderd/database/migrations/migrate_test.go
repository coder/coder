package migrations_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/sync/errgroup"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
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

			ctx := testutil.Context(t, testutil.WaitSuperLong)

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

	ctx := testutil.Context(t, testutil.WaitSuperLong)
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
	ctx := testutil.Context(t, testutil.WaitSuperLong)
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

func TestMigration000457ChatAccessRole(t *testing.T) {
	t.Parallel()

	const migrationVersion = 457

	sqlDB := testSQLDB(t)

	// Migrate up to the migration before the one that grants
	// agents-access roles.
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

	ctx := testutil.Context(t, testutil.WaitSuperLong)

	// Define test users.
	userWithChat := uuid.New()         // Has a chat, no agents-access role.
	userAlreadyHasRole := uuid.New()   // Has a chat and already has agents-access.
	userNoChat := uuid.New()           // No chat at all.
	userWithChatAndRoles := uuid.New() // Has a chat and other existing roles.

	now := time.Now().UTC().Truncate(time.Microsecond)

	// We need a chat_provider and chat_model_config for the chats FK.
	providerID := uuid.New()
	modelConfigID := uuid.New()

	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()

	fixtures := []struct {
		query string
		args  []any
	}{
		// Insert test users with varying rbac_roles.
		{
			`INSERT INTO users (id, username, email, hashed_password, created_at, updated_at, status, rbac_roles, login_type)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			[]any{userWithChat, "user-with-chat", "chat@test.com", []byte{}, now, now, "active", pq.StringArray{}, "password"},
		},
		{
			`INSERT INTO users (id, username, email, hashed_password, created_at, updated_at, status, rbac_roles, login_type)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			[]any{userAlreadyHasRole, "user-already-has-role", "already@test.com", []byte{}, now, now, "active", pq.StringArray{"agents-access"}, "password"},
		},
		{
			`INSERT INTO users (id, username, email, hashed_password, created_at, updated_at, status, rbac_roles, login_type)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			[]any{userNoChat, "user-no-chat", "nochat@test.com", []byte{}, now, now, "active", pq.StringArray{}, "password"},
		},
		{
			`INSERT INTO users (id, username, email, hashed_password, created_at, updated_at, status, rbac_roles, login_type)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			[]any{userWithChatAndRoles, "user-with-roles", "roles@test.com", []byte{}, now, now, "active", pq.StringArray{"template-admin"}, "password"},
		},
		// Insert a chat provider and model config for the chats FK.
		{
			`INSERT INTO chat_providers (id, provider, display_name, api_key, enabled, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			[]any{providerID, "openai", "OpenAI", "", true, now, now},
		},
		{
			`INSERT INTO chat_model_configs (id, provider, model, display_name, enabled, context_limit, compression_threshold, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			[]any{modelConfigID, "openai", "gpt-4", "GPT 4", true, 100000, 70, now, now},
		},
		// Insert chats for users A, B, and D (not C).
		{
			`INSERT INTO chats (id, owner_id, last_model_config_id, title, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			[]any{uuid.New(), userWithChat, modelConfigID, "Chat A", now, now},
		},
		{
			`INSERT INTO chats (id, owner_id, last_model_config_id, title, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			[]any{uuid.New(), userAlreadyHasRole, modelConfigID, "Chat B", now, now},
		},
		{
			`INSERT INTO chats (id, owner_id, last_model_config_id, title, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			[]any{uuid.New(), userWithChatAndRoles, modelConfigID, "Chat D", now, now},
		},
	}

	for i, f := range fixtures {
		_, err := tx.ExecContext(ctx, f.query, f.args...)
		require.NoError(t, err, "fixture %d", i)
	}
	require.NoError(t, tx.Commit())

	// Run the migration.
	version, _, err := next()
	require.NoError(t, err)
	require.EqualValues(t, migrationVersion, version)

	// Helper to get rbac_roles for a user.
	getRoles := func(t *testing.T, userID uuid.UUID) []string {
		t.Helper()
		var roles pq.StringArray
		err := sqlDB.QueryRowContext(ctx,
			"SELECT rbac_roles FROM users WHERE id = $1", userID,
		).Scan(&roles)
		require.NoError(t, err)
		return roles
	}

	// Verify: user with chat gets agents-access.
	roles := getRoles(t, userWithChat)
	require.Contains(t, roles, "agents-access",
		"user with chat should get agents-access")

	// Verify: user who already had agents-access has no duplicate.
	roles = getRoles(t, userAlreadyHasRole)
	count := 0
	for _, r := range roles {
		if r == "agents-access" {
			count++
		}
	}
	require.Equal(t, 1, count,
		"user who already had agents-access should not get a duplicate")

	// Verify: user without chat does NOT get agents-access.
	roles = getRoles(t, userNoChat)
	require.NotContains(t, roles, "agents-access",
		"user without chat should not get agents-access")

	// Verify: user with chat and existing roles gets agents-access
	// appended while preserving existing roles.
	roles = getRoles(t, userWithChatAndRoles)
	require.Contains(t, roles, "agents-access",
		"user with chat and other roles should get agents-access")
	require.Contains(t, roles, "template-admin",
		"existing roles should be preserved")
}

func TestMigration000475AgentsAccessOrgRole(t *testing.T) {
	t.Parallel()

	const migrationVersion = 475

	sqlDB := testSQLDB(t)

	// Migrate up to the migration before 000475.
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

	ctx := testutil.Context(t, testutil.WaitSuperLong)

	// Seed: a user with site-level agents-access who is a member of
	// two orgs, plus a second user who is a member of one org and
	// does not have the role.
	userWithRole := uuid.New()
	userWithoutRole := uuid.New()
	org1ID := uuid.New()
	org2ID := uuid.New()

	now := time.Now().UTC().Truncate(time.Microsecond)

	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()

	fixtures := []struct {
		query string
		args  []any
	}{
		{
			`INSERT INTO users (id, username, email, hashed_password, created_at, updated_at, status, rbac_roles, login_type)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			[]any{userWithRole, "user-with-role", "withrole@test.com", []byte{}, now, now, "active", pq.StringArray{"agents-access"}, "password"},
		},
		{
			`INSERT INTO users (id, username, email, hashed_password, created_at, updated_at, status, rbac_roles, login_type)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			[]any{userWithoutRole, "user-without-role", "withoutrole@test.com", []byte{}, now, now, "active", pq.StringArray{}, "password"},
		},
		{
			`INSERT INTO organizations (id, name, display_name, description, icon, created_at, updated_at, is_default)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			[]any{org1ID, "org-1", "Org 1", "", "", now, now, false},
		},
		{
			`INSERT INTO organizations (id, name, display_name, description, icon, created_at, updated_at, is_default)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			[]any{org2ID, "org-2", "Org 2", "", "", now, now, false},
		},
		{
			`INSERT INTO organization_members (organization_id, user_id, created_at, updated_at, roles)
			VALUES ($1, $2, $3, $4, $5)`,
			[]any{org1ID, userWithRole, now, now, pq.StringArray{}},
		},
		{
			`INSERT INTO organization_members (organization_id, user_id, created_at, updated_at, roles)
			VALUES ($1, $2, $3, $4, $5)`,
			[]any{org2ID, userWithRole, now, now, pq.StringArray{}},
		},
		{
			`INSERT INTO organization_members (organization_id, user_id, created_at, updated_at, roles)
			VALUES ($1, $2, $3, $4, $5)`,
			[]any{org1ID, userWithoutRole, now, now, pq.StringArray{}},
		},
	}

	for i, f := range fixtures {
		_, err := tx.ExecContext(ctx, f.query, f.args...)
		require.NoError(t, err, "fixture %d", i)
	}
	require.NoError(t, tx.Commit())

	// Run migration 000475.
	version, _, err := next()
	require.NoError(t, err)
	require.EqualValues(t, migrationVersion, version)

	// Verify: userWithRole no longer has agents-access at site level.
	var siteRoles pq.StringArray
	err = sqlDB.QueryRowContext(ctx,
		"SELECT rbac_roles FROM users WHERE id = $1", userWithRole,
	).Scan(&siteRoles)
	require.NoError(t, err)
	require.NotContains(t, siteRoles, "agents-access",
		"agents-access should be removed from users.rbac_roles")

	// Verify: userWithRole has agents-access in both orgs.
	for _, orgID := range []uuid.UUID{org1ID, org2ID} {
		var orgRoles pq.StringArray
		err = sqlDB.QueryRowContext(ctx,
			"SELECT roles FROM organization_members WHERE user_id = $1 AND organization_id = $2",
			userWithRole, orgID,
		).Scan(&orgRoles)
		require.NoError(t, err)
		require.Contains(t, orgRoles, "agents-access",
			"agents-access should be granted in org %s", orgID)
	}

	// Verify: userWithoutRole did not gain agents-access.
	var orgRoles pq.StringArray
	err = sqlDB.QueryRowContext(ctx,
		"SELECT roles FROM organization_members WHERE user_id = $1 AND organization_id = $2",
		userWithoutRole, org1ID,
	).Scan(&orgRoles)
	require.NoError(t, err)
	require.NotContains(t, orgRoles, "agents-access",
		"agents-access should not be granted to a user who didn't have it")

	// Verify: no DB row exists for agents-access as a custom_role.
	// The role is now a builtin, resolved in Go via RoleByName.
	var customRoleCount int
	err = sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM custom_roles WHERE name = 'agents-access'",
	).Scan(&customRoleCount)
	require.NoError(t, err)
	require.Equal(t, 0, customRoleCount,
		"no custom_roles row should exist for agents-access")

	// Verify: creating a new organization does NOT insert an
	// agents-access custom_role via the trigger. It should only
	// insert organization-member and organization-service-account.
	newOrgID := uuid.New()
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO organizations (id, name, display_name, description, icon, created_at, updated_at, is_default)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		newOrgID, "new-org", "New Org", "", "", now, now, false,
	)
	require.NoError(t, err)

	rows, err := sqlDB.QueryContext(ctx,
		"SELECT name FROM custom_roles WHERE organization_id = $1 AND is_system = true ORDER BY name",
		newOrgID,
	)
	require.NoError(t, err)
	defer rows.Close()

	var gotRoleNames []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		gotRoleNames = append(gotRoleNames, name)
	}
	require.NoError(t, rows.Err())
	require.ElementsMatch(t,
		[]string{"organization-member", "organization-service-account"},
		gotRoleNames,
		"trigger should only create org-member and org-service-account system roles",
	)
}

func TestMigration000504AIProvidersBackfill(t *testing.T) {
	t.Parallel()

	const migrationVersion = 504

	sqlDB := testSQLDB(t)

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

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	now := time.Now().UTC().Truncate(time.Microsecond)
	userID := uuid.New()
	openAIProviderID := uuid.New()
	anthropicProviderID := uuid.New()
	openAIUserKeyID := uuid.New()
	anthropicUserKeyID := uuid.New()
	openAIModelConfigID := uuid.New()
	anthropicModelConfigID := uuid.New()

	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO users (id, username, email, hashed_password, created_at, updated_at, status, rbac_roles, login_type)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		userID, "ai-provider-backfill", "ai-provider-backfill@test.com", []byte{}, now, now, "active", pq.StringArray{}, "password",
	)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO chat_providers (id, provider, display_name, api_key, enabled, base_url, created_at, updated_at)
		VALUES
			($1, 'openai', 'OpenAI', 'sk-provider-openai', TRUE, 'https://api.openai.example.com/v1', $3, $3),
			($2, 'anthropic', '', '', FALSE, '', $3, $3)
	`, openAIProviderID, anthropicProviderID, now)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_chat_provider_keys (id, user_id, chat_provider_id, api_key, created_at, updated_at)
		VALUES
			($1, $3, $4, 'sk-user-openai', $6, $6),
			($2, $3, $5, 'sk-user-anthropic', $6, $6)
	`, openAIUserKeyID, anthropicUserKeyID, userID, openAIProviderID, anthropicProviderID, now)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO chat_model_configs (id, provider, model, display_name, enabled, context_limit, compression_threshold, created_at, updated_at)
		VALUES
			($1, 'openai', 'gpt-4', 'GPT 4', TRUE, 100000, 70, $3, $3),
			($2, 'anthropic', 'claude-3-5-sonnet-latest', 'Claude 3.5 Sonnet', TRUE, 200000, 70, $3, $3)
	`, openAIModelConfigID, anthropicModelConfigID, now)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	var preBackfillCount int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM ai_providers
		WHERE id IN ($1, $2)
	`, openAIProviderID, anthropicProviderID).Scan(&preBackfillCount)
	require.NoError(t, err)
	require.Zero(t, preBackfillCount, "test setup should start before the legacy chat providers are backfilled")

	var preBackfillModelConfigCount int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM chat_model_configs
		WHERE id IN ($1, $2)
			AND ai_provider_id IS NOT NULL
	`, openAIModelConfigID, anthropicModelConfigID).Scan(&preBackfillModelConfigCount)
	require.NoError(t, err)
	require.Zero(t, preBackfillModelConfigCount, "test setup should start before model configs point at AI providers")

	version, more, err := next()
	require.NoError(t, err)
	require.True(t, more)
	require.EqualValues(t, migrationVersion, version)

	assertBackfilledProvider := func(providerID uuid.UUID, providerType, name string, displayName sql.NullString, enabled bool, baseURL string) {
		t.Helper()
		var provider struct {
			Typ         string
			Name        string
			DisplayName sql.NullString
			Enabled     bool
			BaseURL     string
		}
		err = sqlDB.QueryRowContext(ctx, `
			SELECT type, name, display_name, enabled, base_url
			FROM ai_providers
			WHERE id = $1
		`, providerID).Scan(&provider.Typ, &provider.Name, &provider.DisplayName, &provider.Enabled, &provider.BaseURL)
		require.NoError(t, err)
		require.Equal(t, providerType, provider.Typ)
		require.Equal(t, name, provider.Name)
		require.Equal(t, displayName, provider.DisplayName)
		require.Equal(t, enabled, provider.Enabled)
		require.Equal(t, baseURL, provider.BaseURL)
	}
	assertBackfilledProvider(
		openAIProviderID,
		"openai",
		"agents-openai",
		sql.NullString{String: "OpenAI", Valid: true},
		true,
		"https://api.openai.example.com/v1",
	)
	assertBackfilledProvider(
		anthropicProviderID,
		"anthropic",
		"agents-anthropic",
		sql.NullString{},
		false,
		"",
	)

	var providerKeyCount int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM ai_provider_keys
		WHERE provider_id = $1 AND api_key = 'sk-provider-openai'
	`, openAIProviderID).Scan(&providerKeyCount)
	require.NoError(t, err)
	require.Equal(t, 1, providerKeyCount, "non-empty legacy provider API key should be copied")

	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM ai_provider_keys
		WHERE provider_id = $1
	`, anthropicProviderID).Scan(&providerKeyCount)
	require.NoError(t, err)
	require.Zero(t, providerKeyCount, "empty legacy provider API key should not create an AI provider key")

	assertBackfilledUserKey := func(userKeyID, providerID uuid.UUID, apiKey string) {
		t.Helper()
		var userKeyCount int
		err = sqlDB.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM user_ai_provider_keys
			WHERE id = $1 AND user_id = $2 AND ai_provider_id = $3 AND api_key = $4
		`, userKeyID, userID, providerID, apiKey).Scan(&userKeyCount)
		require.NoError(t, err)
		require.Equal(t, 1, userKeyCount)
	}
	assertBackfilledUserKey(openAIUserKeyID, openAIProviderID, "sk-user-openai")
	assertBackfilledUserKey(anthropicUserKeyID, anthropicProviderID, "sk-user-anthropic")

	assertModelConfigProviderID := func(modelConfigID, providerID uuid.UUID) {
		t.Helper()
		var aiProviderID sql.NullString
		err = sqlDB.QueryRowContext(ctx,
			`SELECT ai_provider_id::text FROM chat_model_configs WHERE id = $1`,
			modelConfigID,
		).Scan(&aiProviderID)
		require.NoError(t, err)
		require.Equal(t, sql.NullString{String: providerID.String(), Valid: true}, aiProviderID)
	}
	assertModelConfigProviderID(openAIModelConfigID, openAIProviderID)
	assertModelConfigProviderID(anthropicModelConfigID, anthropicProviderID)

	var legacyProviderCount int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM chat_providers
		WHERE id IN ($1, $2)
	`, openAIProviderID, anthropicProviderID).Scan(&legacyProviderCount)
	require.NoError(t, err)
	require.Equal(t, 2, legacyProviderCount, "backfill should leave legacy rows for the rest of the stack")

	downSQL, err := os.ReadFile("000504_ai_providers_backfill.down.sql")
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, string(downSQL))
	require.NoError(t, err)

	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM ai_providers
		WHERE id IN ($1, $2)
	`, openAIProviderID, anthropicProviderID).Scan(&providerKeyCount)
	require.NoError(t, err)
	require.Zero(t, providerKeyCount, "down migration should remove backfilled AI providers")

	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM ai_provider_keys
		WHERE provider_id IN ($1, $2)
	`, openAIProviderID, anthropicProviderID).Scan(&providerKeyCount)
	require.NoError(t, err)
	require.Zero(t, providerKeyCount, "down migration should remove backfilled provider keys")

	var userKeyCount int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM user_ai_provider_keys
		WHERE id IN ($1, $2)
	`, openAIUserKeyID, anthropicUserKeyID).Scan(&userKeyCount)
	require.NoError(t, err)
	require.Zero(t, userKeyCount, "down migration should remove backfilled user keys")

	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM chat_model_configs
		WHERE id IN ($1, $2)
			AND ai_provider_id IS NOT NULL
	`, openAIModelConfigID, anthropicModelConfigID).Scan(&preBackfillModelConfigCount)
	require.NoError(t, err)
	require.Zero(t, preBackfillModelConfigCount, "down migration should clear model config AI provider references")

	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM chat_providers
		WHERE id IN ($1, $2)
	`, openAIProviderID, anthropicProviderID).Scan(&legacyProviderCount)
	require.NoError(t, err)
	require.Equal(t, 2, legacyProviderCount, "down migration should leave the legacy source rows intact")
}

// TestMigration000504AIProvidersBackfillOverridesNameConflict verifies that a
// pre-existing live ai_providers row whose name collides with the backfill
// (for example, agents-openai) is soft-deleted so the chat_providers-derived
// row inserted by the migration becomes authoritative. This scenario should
// not occur in practice since no other process writes to ai_providers before
// this migration runs, but the migration tolerates it rather than failing.
func TestMigration000504AIProvidersBackfillOverridesNameConflict(t *testing.T) {
	t.Parallel()

	const migrationVersion = 504

	sqlDB := testSQLDB(t)

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

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	now := time.Now().UTC().Truncate(time.Microsecond)
	chatProviderID := uuid.New()
	staleProviderID := uuid.New()

	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()

	// Pre-existing live ai_providers row that collides on name.
	_, err = tx.ExecContext(ctx,
		`INSERT INTO ai_providers (id, type, name, display_name, enabled, base_url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		staleProviderID, "openai", "agents-openai", "Stale OpenAI", true, "https://stale.example.com/v1", now, now,
	)
	require.NoError(t, err)

	// chat_providers row whose backfill will collide with the stale row above.
	_, err = tx.ExecContext(ctx,
		`INSERT INTO chat_providers (id, provider, display_name, api_key, enabled, base_url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		chatProviderID, "openai", "OpenAI", "sk-provider", true, "https://api.openai.example.com/v1", now, now,
	)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	version, more, err := next()
	require.NoError(t, err)
	require.True(t, more)
	require.EqualValues(t, migrationVersion, version)

	// The stale row must be soft-deleted and disabled so the unique name index
	// (which is partial WHERE deleted = FALSE) no longer covers it.
	var stale struct {
		Deleted bool
		Enabled bool
	}
	err = sqlDB.QueryRowContext(ctx,
		`SELECT deleted, enabled FROM ai_providers WHERE id = $1`,
		staleProviderID,
	).Scan(&stale.Deleted, &stale.Enabled)
	require.NoError(t, err)
	require.True(t, stale.Deleted, "pre-existing conflicting ai_providers row should be soft-deleted")
	require.False(t, stale.Enabled, "pre-existing conflicting ai_providers row should be disabled")

	// The new authoritative row must exist with the chat_providers id, the
	// agents-openai name, and the chat_providers base_url.
	var fresh struct {
		Name    string
		BaseURL string
		Deleted bool
		Enabled bool
	}
	err = sqlDB.QueryRowContext(ctx,
		`SELECT name, base_url, deleted, enabled FROM ai_providers WHERE id = $1`,
		chatProviderID,
	).Scan(&fresh.Name, &fresh.BaseURL, &fresh.Deleted, &fresh.Enabled)
	require.NoError(t, err)
	require.Equal(t, "agents-openai", fresh.Name)
	require.Equal(t, "https://api.openai.example.com/v1", fresh.BaseURL)
	require.False(t, fresh.Deleted)
	require.True(t, fresh.Enabled)
}

// TestMigration000504AIProvidersBackfillEnumInSingleTxn reproduces the
// production migration path, where every pending migration runs inside a
// single transaction (see pgTxnDriver). Migration 000499 widens
// ai_provider_type with ALTER TYPE ... ADD VALUE, and 000504 casts existing
// chat_providers rows to that enum. Postgres forbids using an enum value
// added by ADD VALUE within the same transaction, so when a legacy provider
// uses one of the new values (for example openai-compat) the batch fails with
// "unsafe use of new value". The per-step Stepper used by the other tests
// commits each migration separately and cannot surface this.
func TestMigration000504AIProvidersBackfillEnumInSingleTxn(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	ctx := testutil.Context(t, testutil.WaitSuperLong)

	// Apply everything through 498 and commit, so chat_providers exists and is
	// populated before the batch under test runs, matching a deployment that
	// ran an earlier migration batch before this one.
	applyMigrationsInTxn(ctx, t, sqlDB, 1, 498)

	now := time.Now().UTC().Truncate(time.Microsecond)
	providerID := uuid.New()

	// A legacy provider whose type is one of the values added in 000499.
	_, err := sqlDB.ExecContext(ctx, `
		INSERT INTO chat_providers (id, provider, display_name, api_key, enabled, base_url, created_at, updated_at)
		VALUES ($1, 'openai-compat', 'OpenAI Compatible', '', TRUE, 'https://api.example.com/v1', $2, $2)
	`, providerID, now)
	require.NoError(t, err)

	// Apply 000499 through 000504 in a single transaction, as production does.
	applyMigrationsInTxn(ctx, t, sqlDB, 499, 504)

	var typ string
	err = sqlDB.QueryRowContext(ctx,
		`SELECT type FROM ai_providers WHERE id = $1`, providerID,
	).Scan(&typ)
	require.NoError(t, err)
	require.Equal(t, "openai-compat", typ)
}

// applyMigrationsInTxn executes the up SQL for every migration whose version is
// in [from, to] inside a single transaction, mirroring pgTxnDriver. The whole
// batch commits or rolls back together.
func applyMigrationsInTxn(ctx context.Context, t *testing.T, sqlDB *sql.DB, from, to int) {
	t.Helper()

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		var version int
		if _, err := fmt.Sscanf(name, "%06d_", &version); err != nil {
			continue
		}
		if version >= from && version <= to {
			files = append(files, name)
		}
	}
	slices.Sort(files)

	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()

	for _, name := range files {
		query, err := os.ReadFile(name)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, string(query))
		require.NoErrorf(t, err, "apply migration %s", name)
	}
	require.NoError(t, tx.Commit())
}

func TestMigration000542ChatReasoningEffortBackfill(t *testing.T) {
	t.Parallel()

	const priorMigrationVersion = 539

	sqlDB := testSQLDB(t)

	next, err := migrations.Stepper(sqlDB)
	require.NoError(t, err)
	for {
		version, more, err := next()
		require.NoError(t, err)
		if !more || version == priorMigrationVersion {
			break
		}
	}

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	now := time.Now().UTC().Truncate(time.Microsecond)

	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()
	azureID := uuid.New()
	bedrockID := uuid.New()
	emptyID := uuid.New()
	invalidID := uuid.New()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO ai_providers (id, type, name, enabled, base_url, created_at, updated_at)
		VALUES
			($1, 'azure', 'test-azure-reasoning', TRUE, '', $3, $3),
			($2, 'bedrock', 'test-bedrock-reasoning', TRUE, '', $3, $3)
	`, azureID, bedrockID, now)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO chat_model_configs (id, ai_provider_id, model, display_name, enabled, context_limit, compression_threshold, options, created_at, updated_at)
		VALUES
			($3, $1, 'gpt-5.1-azure', 'Azure GPT-5.1', TRUE, 200000, 70, '{"provider_options": {"azure": {"reasoning_effort": " LOW "}}}', $5, $5),
			($4, $2, 'anthropic.claude-opus-4-6', 'Bedrock Claude Opus', TRUE, 200000, 70, '{"provider_options": {"bedrock": {"effort": "minimal"}}}', $5, $5),
			(gen_random_uuid(), $1, 'gpt-5.1-empty-effort', 'Azure Empty Effort', TRUE, 200000, 70, '{"provider_options": {"azure": {"reasoning_effort": ""}}}', $5, $5),
			(gen_random_uuid(), $2, 'anthropic.invalid-effort', 'Bedrock Invalid Effort', TRUE, 200000, 70, '{"provider_options": {"bedrock": {"effort": "extreme"}}}', $5, $5)
	`, azureID, bedrockID, emptyID, invalidID, now)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	migrationSQL, err := os.ReadFile("000542_chat_reasoning_effort.up.sql")
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)

	rows, err := sqlDB.QueryContext(ctx, `
		SELECT ap.type::text, cmc.model, cmc.options->'reasoning_effort'->>'default'
		FROM chat_model_configs cmc
		JOIN ai_providers ap ON ap.id = cmc.ai_provider_id
		WHERE cmc.ai_provider_id IN ($1, $2)
		ORDER BY cmc.model
	`, azureID, bedrockID)
	require.NoError(t, err)
	defer rows.Close()

	got := map[string]sql.NullString{}
	for rows.Next() {
		var provider, model string
		var effort sql.NullString
		require.NoError(t, rows.Scan(&provider, &model, &effort))
		got[provider+":"+model] = effort
	}
	require.NoError(t, rows.Err())
	require.Equal(t, sql.NullString{String: "low", Valid: true}, got["azure:gpt-5.1-azure"])
	require.Equal(t, sql.NullString{}, got["azure:gpt-5.1-empty-effort"])
	require.Equal(t, sql.NullString{String: "minimal", Valid: true}, got["bedrock:anthropic.claude-opus-4-6"])
	require.Equal(t, sql.NullString{}, got["bedrock:anthropic.invalid-effort"])
}

func TestMigration000498SoftDeleteStaleWorkspaceAgents(t *testing.T) {
	t.Parallel()

	const migrationVersion = 498

	sqlDB := testSQLDB(t)

	// Step up to migrationVersion - 1.
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

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Seed the prerequisite tables. Two workspaces share the same EC2-style
	// instance id across several builds; a third workspace has a single
	// build on a different instance (baseline, must not be affected).
	userID := uuid.New()
	orgID := uuid.New()
	templateID := uuid.New()
	templateVersionID := uuid.New()
	fileID := uuid.New()

	wsA := uuid.New()
	wsB := uuid.New()
	wsSingle := uuid.New()
	wsDeleted := uuid.New()

	instanceAB := "i-shared-ab"
	instanceSingle := "i-solo"
	instanceDeleted := "i-deleted"

	// For workspace A: 3 builds on the same instance.
	// For workspace B: 2 builds on the same instance (different workspace,
	// same instance id, exercises the cross-workspace scoping case).
	// For wsSingle: 1 build, should stay non-deleted after the backfill.
	// For wsDeleted: 1 build on a soft-deleted workspace. Agent should be
	// marked deleted even though it's on the latest build.
	type build struct {
		id         uuid.UUID
		jobID      uuid.UUID
		resourceID uuid.UUID
		agentID    uuid.UUID
		buildNum   int32
		wsID       uuid.UUID
		instanceID string
	}

	mkBuild := func(ws uuid.UUID, buildNum int32, instance string) build {
		return build{
			id:         uuid.New(),
			jobID:      uuid.New(),
			resourceID: uuid.New(),
			agentID:    uuid.New(),
			buildNum:   buildNum,
			wsID:       ws,
			instanceID: instance,
		}
	}

	aBuilds := []build{
		mkBuild(wsA, 1, instanceAB),
		mkBuild(wsA, 2, instanceAB),
		mkBuild(wsA, 3, instanceAB),
	}
	bBuilds := []build{
		mkBuild(wsB, 1, instanceAB),
		mkBuild(wsB, 2, instanceAB),
	}
	singleBuilds := []build{
		mkBuild(wsSingle, 1, instanceSingle),
	}
	deletedBuilds := []build{
		mkBuild(wsDeleted, 1, instanceDeleted),
	}
	allBuilds := append(append(append(append([]build{}, aBuilds...), bBuilds...), singleBuilds...), deletedBuilds...)

	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()

	// Minimal user / org / template / template_version / file.
	_, err = tx.ExecContext(ctx,
		`INSERT INTO users (id, username, email, hashed_password, created_at, updated_at, status, rbac_roles, login_type)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		userID, "seed", "seed@test.com", []byte{}, now, now, "active", pq.StringArray{}, "password",
	)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO organizations (id, name, display_name, description, icon, created_at, updated_at, is_default)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		orgID, "seed-org", "Seed Org", "", "", now, now, false,
	)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO files (id, hash, created_at, created_by, mimetype, data) VALUES ($1, $2, $3, $4, $5, $6)`,
		fileID, "hash", now, userID, "application/octet-stream", []byte{},
	)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO templates (id, created_at, updated_at, organization_id, name, provisioner, active_version_id, description, created_by, group_acl, user_acl, display_name)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		templateID, now, now, orgID, "tpl", "echo", templateVersionID, "", userID, "{}", "{}", "",
	)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO template_versions (id, template_id, organization_id, created_at, updated_at, name, readme, job_id, created_by, message)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		templateVersionID, templateID, orgID, now, now, "v", "", uuid.New(), userID, "",
	)
	require.NoError(t, err)

	for _, ws := range []uuid.UUID{wsA, wsB, wsSingle} {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO workspaces (id, created_at, updated_at, owner_id, organization_id, template_id, name, deleted, automatic_updates)
			VALUES ($1, $2, $3, $4, $5, $6, $7, false, 'never')`,
			ws, now, now, userID, orgID, templateID, "ws-"+ws.String()[:8],
		)
		require.NoError(t, err)
	}
	// wsDeleted is a soft-deleted workspace. Its agent is on the latest
	// build but must still be soft-deleted by the migration.
	_, err = tx.ExecContext(ctx,
		`INSERT INTO workspaces (id, created_at, updated_at, owner_id, organization_id, template_id, name, deleted, automatic_updates)
		VALUES ($1, $2, $3, $4, $5, $6, $7, true, 'never')`,
		wsDeleted, now, now, userID, orgID, templateID, "ws-"+wsDeleted.String()[:8],
	)
	require.NoError(t, err)

	// For every build: provisioner_job -> workspace_build -> workspace_resource -> workspace_agent.
	for _, b := range allBuilds {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO provisioner_jobs (id, created_at, updated_at, organization_id, initiator_id, provisioner, storage_method, type, input, file_id)
			VALUES ($1, $2, $3, $4, $5, 'echo', 'file', 'workspace_build', '{}', $6)`,
			b.jobID, now, now, orgID, userID, fileID,
		)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx,
			`INSERT INTO workspace_builds (id, created_at, updated_at, workspace_id, template_version_id, build_number, transition, initiator_id, job_id, reason)
			VALUES ($1, $2, $3, $4, $5, $6, 'start', $7, $8, 'initiator')`,
			b.id, now, now, b.wsID, templateVersionID, b.buildNum, userID, b.jobID,
		)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx,
			`INSERT INTO workspace_resources (id, created_at, job_id, transition, type, name)
			VALUES ($1, $2, $3, 'start', 'aws_instance', 'dev')`,
			b.resourceID, now, b.jobID,
		)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx,
			`INSERT INTO workspace_agents (id, created_at, updated_at, name, resource_id, auth_token, auth_instance_id, architecture, operating_system, deleted)
			VALUES ($1, $2, $3, 'main', $4, $5, $6, 'amd64', 'linux', false)`,
			b.agentID, now, now, b.resourceID, uuid.New(), b.instanceID,
		)
		require.NoError(t, err)
	}

	require.NoError(t, tx.Commit())

	// Sanity check pre-migration: all agents should be deleted=false.
	var preDeletedCount int
	err = sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM workspace_agents WHERE deleted = true`).Scan(&preDeletedCount)
	require.NoError(t, err)
	require.Equal(t, 0, preDeletedCount, "no agents should be deleted pre-migration")

	// Run migration 491.
	version, more, err := next()
	require.NoError(t, err)
	require.True(t, more)
	require.EqualValues(t, migrationVersion, version)

	// Backfill assertions:
	//   wsA: builds 1,2,3 → keep agent for build 3, delete for 1 and 2.
	//   wsB: builds 1,2 → keep agent for build 2, delete for 1.
	//   wsSingle: 1 build → keep.
	//   Per workspace, exactly one agent remains deleted=false.
	check := func(label string, expectDeleted bool, agent uuid.UUID) {
		var deleted bool
		err := sqlDB.QueryRowContext(ctx,
			`SELECT deleted FROM workspace_agents WHERE id = $1`, agent).Scan(&deleted)
		require.NoError(t, err, label)
		require.Equal(t, expectDeleted, deleted, label)
	}
	check("wsA build 1 (old) should be deleted", true, aBuilds[0].agentID)
	check("wsA build 2 (old) should be deleted", true, aBuilds[1].agentID)
	check("wsA build 3 (latest) should be kept", false, aBuilds[2].agentID)
	check("wsB build 1 (old) should be deleted", true, bBuilds[0].agentID)
	check("wsB build 2 (latest) should be kept", false, bBuilds[1].agentID)
	check("wsSingle build 1 (solo latest) should be kept", false, singleBuilds[0].agentID)
	check("wsDeleted: agent on deleted workspace should be soft-deleted even though it's the latest build",
		true, deletedBuilds[0].agentID)

	// The ongoing invariants are enforced by wsbuilder.Builder.Build and
	// provisionerdserver.CompleteJob via SoftDeletePriorWorkspaceAgents and
	// SoftDeleteWorkspaceAgentsByWorkspaceID. Those paths are covered by
	// the querier tests TestSoftDeletePriorWorkspaceAgents and
	// TestSoftDeleteWorkspaceAgentsByWorkspaceID, plus integration tests
	// under coderd/coderd_test.go; not retested here.
}

func TestMigration000543ChatMessageSearchText(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	require.NoError(t, migrations.Up(sqlDB))

	cases := []struct {
		name    string
		content sql.NullString
		want    sql.NullString
	}{
		{
			name:    "SingleTextPart",
			content: sql.NullString{String: `[{"type":"text","text":"hello world"}]`, Valid: true},
			want:    sql.NullString{String: "hello world", Valid: true},
		},
		{
			name: "TextInterleavedWithNonText",
			content: sql.NullString{String: `[
				{"type":"text","text":"first"},
				{"type":"reasoning","text":"thinking"},
				{"type":"tool-call","toolName":"execute"},
				{"type":"text","text":"second"}
			]`, Valid: true},
			want: sql.NullString{String: "first second", Valid: true},
		},
		{
			name:    "OnlyNonTextParts",
			content: sql.NullString{String: `[{"type":"reasoning","text":"thinking"}]`, Valid: true},
			want:    sql.NullString{},
		},
		{
			name:    "ScalarContent",
			content: sql.NullString{String: `"hello"`, Valid: true},
			want:    sql.NullString{},
		},
		{
			name:    "EmptyArray",
			content: sql.NullString{String: `[]`, Valid: true},
			want:    sql.NullString{},
		},
		{
			name:    "NullInput",
			content: sql.NullString{},
			want:    sql.NullString{},
		},
		{
			name:    "ElementsMissingTypeOrText",
			content: sql.NullString{String: `[{"text":"no type"},{"type":"text"},{"type":"text","text":"kept"}]`, Valid: true},
			want:    sql.NullString{String: "kept", Valid: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitMedium)
			var got sql.NullString
			err := sqlDB.QueryRowContext(ctx,
				`SELECT chat_message_search_text($1::jsonb)`, tc.content,
			).Scan(&got)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// Shared eligibility predicate of the two partial chat_messages search
// indexes. Queries must repeat it verbatim.
const eligibilityPredicate = `deleted = false
	AND visibility IN ('user', 'both')
	AND role IN ('user', 'assistant')`

func TestMigration000543ChatSearchSchemaIndexes(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	require.NoError(t, migrations.Up(sqlDB))

	cases := []struct {
		name    string
		table   string
		partial bool
	}{
		{name: "idx_chat_messages_search_tsv", table: "chat_messages", partial: true},
		{name: "idx_chat_messages_search_tsv_pending", table: "chat_messages", partial: true},
		{name: "idx_chats_title_fts", table: "chats", partial: false},
		{name: "idx_chat_diff_statuses_pr_title_fts", table: "chat_diff_statuses", partial: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitMedium)
			var table string
			var partial bool
			err := sqlDB.QueryRowContext(ctx, `
				SELECT i.tablename, x.indpred IS NOT NULL
				FROM pg_indexes i
				JOIN pg_class c ON c.relname = i.indexname
				JOIN pg_index x ON x.indexrelid = c.oid
				WHERE i.indexname = $1`, tc.name,
			).Scan(&table, &partial)
			require.NoError(t, err, "index %s should exist", tc.name)
			require.Equal(t, tc.table, table, "index %s table", tc.name)
			require.Equal(t, tc.partial, partial, "index %s partial", tc.name)
		})
	}
}

func TestMigration000543ChatSearchSchemaBehavior(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	require.NoError(t, migrations.Up(sqlDB))
	db := database.New(sqlDB)
	ctx := testutil.Context(t, testutil.WaitLong)

	org := dbgen.Organization(t, db, database.Organization{})
	owner := dbgen.User(t, db, database.User{})
	_ = dbgen.ChatProvider(t, db, database.ChatProvider{Provider: "openai", DisplayName: "OpenAI"})
	modelCfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		CreatedBy: uuid.NullUUID{UUID: owner.ID, Valid: true},
		UpdatedBy: uuid.NullUUID{UUID: owner.ID, Valid: true},
		IsDefault: true,
	})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           owner.ID,
		LastModelConfigID: modelCfg.ID,
	})

	newMsg := func(role database.ChatMessageRole, visibility database.ChatMessageVisibility, content string) database.ChatMessage {
		seed := database.ChatMessage{
			ChatID:     chat.ID,
			CreatedBy:  uuid.NullUUID{UUID: owner.ID, Valid: true},
			Role:       role,
			Visibility: visibility,
		}
		if content != "" {
			seed.Content = pqtype.NullRawMessage{RawMessage: []byte(content), Valid: true}
		}
		return dbgen.ChatMessage(t, db, seed)
	}
	textContent := func(text string) string {
		return `[{"type":"text","text":"` + text + `"}]`
	}

	pendingIDs := func(ctx context.Context, limit int) []int64 {
		rows, err := sqlDB.QueryContext(ctx, `
			SELECT id FROM chat_messages
			WHERE search_tsv IS NULL AND `+eligibilityPredicate+`
			ORDER BY id DESC
			LIMIT $1`, limit)
		require.NoError(t, err)
		defer rows.Close()
		var ids []int64
		for rows.Next() {
			var id int64
			require.NoError(t, rows.Scan(&id))
			ids = append(ids, id)
		}
		require.NoError(t, rows.Err())
		return ids
	}

	// Insert regression: RETURNING * must survive the new column, and new
	// rows must start with search_tsv NULL so they enter the pending queue.
	eligibleText := newMsg(database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth, textContent("deploy the search feature"))
	var tsvIsNull bool
	err := sqlDB.QueryRowContext(ctx,
		`SELECT search_tsv IS NULL FROM chat_messages WHERE id = $1`, eligibleText.ID,
	).Scan(&tsvIsNull)
	require.NoError(t, err)
	require.True(t, tsvIsNull, "new rows must have search_tsv NULL")

	eligibleNoText := newMsg(database.ChatMessageRoleAssistant, database.ChatMessageVisibilityUser, `[{"type":"reasoning","text":"thinking"}]`)
	toolMsg := newMsg(database.ChatMessageRoleTool, database.ChatMessageVisibilityBoth, textContent("tool output about deploy"))
	modelOnly := newMsg(database.ChatMessageRoleUser, database.ChatMessageVisibilityModel, textContent("model-only deploy note"))
	deletedMsg := newMsg(database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth, textContent("deleted deploy message"))
	_, err = sqlDB.ExecContext(ctx, `UPDATE chat_messages SET deleted = true WHERE id = $1`, deletedMsg.ID)
	require.NoError(t, err)

	// Only eligible rows appear in the queue, newest first. The tool-role,
	// model-only, and soft-deleted rows are excluded even though their
	// search_tsv is NULL.
	require.Equal(t, []int64{eligibleNoText.ID, eligibleText.ID}, pendingIDs(ctx, 10))

	// Sweep-style UPDATE. The '' sentinel (not NULL) marks no-text rows as
	// swept; NULL means pending, so COALESCE is what drains them from the
	// queue.
	_, err = sqlDB.ExecContext(ctx, `
		UPDATE chat_messages
		SET search_tsv = COALESCE(to_tsvector('simple', chat_message_search_text(content)), ''::tsvector)
		WHERE id = ANY($1)`, pq.Array([]int64{eligibleText.ID, eligibleNoText.ID}))
	require.NoError(t, err)
	require.Empty(t, pendingIDs(ctx, 10), "swept rows must leave the queue, including no-text rows")

	// Soft-deleting an unswept row removes it from the queue without a sweep.
	unswept := newMsg(database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth, textContent("unswept deploy row"))
	require.Equal(t, []int64{unswept.ID}, pendingIDs(ctx, 10))
	_, err = sqlDB.ExecContext(ctx, `UPDATE chat_messages SET deleted = true WHERE id = $1`, unswept.ID)
	require.NoError(t, err)
	require.Empty(t, pendingIDs(ctx, 10))

	// Search contract: populate search_tsv on every row (including
	// ineligible ones) and assert the search-index predicate filters them.
	_, err = sqlDB.ExecContext(ctx, `
		UPDATE chat_messages
		SET search_tsv = COALESCE(to_tsvector('simple', chat_message_search_text(content)), ''::tsvector)
		WHERE chat_id = $1`, chat.ID)
	require.NoError(t, err)

	rows, err := sqlDB.QueryContext(ctx, `
		SELECT id FROM chat_messages
		WHERE search_tsv @@ websearch_to_tsquery('simple', $1)
			AND search_tsv IS NOT NULL
			AND `+eligibilityPredicate+`
		ORDER BY id`, "deploy")
	require.NoError(t, err)
	defer rows.Close()
	var matched []int64
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		matched = append(matched, id)
	}
	require.NoError(t, rows.Err())
	require.Equal(t, []int64{eligibleText.ID}, matched,
		"search must exclude deleted, model-only, and tool-role rows (%d %d %d)",
		toolMsg.ID, modelOnly.ID, deletedMsg.ID)
}
