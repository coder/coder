package migrations_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
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

func TestMigration000546ChatHistoryAPIKeyConstraints(t *testing.T) {
	t.Parallel()

	const priorMigrationVersion = 545

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
	constraintNames := []string{
		"chat_messages_api_key_id_fkey",
		"chat_queued_messages_api_key_id_fkey",
	}
	assertConstraintCount := func(t *testing.T, want int) {
		t.Helper()
		for _, name := range constraintNames {
			var got int
			err := sqlDB.QueryRowContext(ctx, `
				SELECT COUNT(*)
				FROM pg_constraint
				WHERE conname = $1
			`, name).Scan(&got)
			require.NoError(t, err)
			require.Equal(t, want, got, name)
		}
	}

	upSQL, err := os.ReadFile("000546_drop_chat_history_api_key_fks.up.sql")
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, string(upSQL))
	require.NoError(t, err)
	assertConstraintCount(t, 0)

	downSQL, err := os.ReadFile("000546_drop_chat_history_api_key_fks.down.sql")
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, string(downSQL))
	require.NoError(t, err)
	assertConstraintCount(t, 1)

	for _, name := range constraintNames {
		var count int
		err := sqlDB.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM pg_constraint
			WHERE conname = $1 AND confdeltype = 'n'
		`, name).Scan(&count)
		require.NoError(t, err)
		require.Equal(t, 1, count, name)
	}
}

func TestMigration000555LegacyNoneLoginToPassword(t *testing.T) {
	t.Parallel()

	const priorMigrationVersion = 554

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

	legacyNoneID := uuid.New()
	serviceAccountID := uuid.New()
	systemID := uuid.New()
	passwordID := uuid.New()

	// A legacy machine user: login_type 'none', not a service account, not a
	// system user. This is the only row the migration should convert.
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO users (id, username, email, hashed_password, created_at, updated_at, status, rbac_roles, login_type, is_service_account, is_system)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		legacyNoneID, "legacy-none", "legacy-none@test.com", []byte{}, now, now, "active", pq.StringArray{}, "none", false, false)
	require.NoError(t, err)

	// A service account must keep login_type 'none' (a CHECK constraint requires
	// service accounts to use 'none' and an empty email).
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO users (id, username, email, hashed_password, created_at, updated_at, status, rbac_roles, login_type, is_service_account, is_system)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		serviceAccountID, "service-account", "", []byte{}, now, now, "active", pq.StringArray{}, "none", true, false)
	require.NoError(t, err)

	// A system user must be left untouched.
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO users (id, username, email, hashed_password, created_at, updated_at, status, rbac_roles, login_type, is_service_account, is_system)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		systemID, "system-user", "system@test.com", []byte{}, now, now, "active", pq.StringArray{}, "none", false, true)
	require.NoError(t, err)

	// An existing password user must be left untouched.
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO users (id, username, email, hashed_password, created_at, updated_at, status, rbac_roles, login_type, is_service_account, is_system)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		passwordID, "password-user", "password@test.com", []byte("hashed"), now, now, "active", pq.StringArray{}, "password", false, false)
	require.NoError(t, err)

	migrationSQL, err := os.ReadFile("000555_legacy_none_login_to_password.up.sql")
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)

	getUser := func(t *testing.T, id uuid.UUID) (loginType, email string) {
		t.Helper()
		err := sqlDB.QueryRowContext(ctx,
			`SELECT login_type::text, email FROM users WHERE id = $1`, id).Scan(&loginType, &email)
		require.NoError(t, err)
		return loginType, email
	}

	// The legacy machine user is converted to password auth with its email
	// preserved.
	gotLoginType, gotEmail := getUser(t, legacyNoneID)
	require.Equal(t, "password", gotLoginType)
	require.Equal(t, "legacy-none@test.com", gotEmail)

	// Service accounts, system users, and existing password users are unchanged.
	gotLoginType, _ = getUser(t, serviceAccountID)
	require.Equal(t, "none", gotLoginType)
	gotLoginType, _ = getUser(t, systemID)
	require.Equal(t, "none", gotLoginType)
	gotLoginType, _ = getUser(t, passwordID)
	require.Equal(t, "password", gotLoginType)
}

// TestMigration000558AuditOAuth2ProviderSettingsEnumInSingleTxn reproduces
// the production upgrade path, where every pending migration in a deploy
// runs inside a single transaction (see pgTxnDriver). 000558 adds
// 'oauth2_provider_settings' to the resource_type enum via ALTER TYPE ...
// ADD VALUE. Postgres forbids using an enum value added by ADD VALUE within
// the same transaction that added it, so this confirms the audit write path
// (a separate transaction, exactly like commitAudit() performs it for a real
// PUT to the DCR settings endpoint) can use the new value immediately after
// the migration transaction commits, and that pre-existing audit data from
// before the upgrade survives untouched.
func TestMigration000558AuditOAuth2ProviderSettingsEnumInSingleTxn(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	ctx := testutil.Context(t, testutil.WaitSuperLong)

	// Apply everything through 557 and commit, simulating a deployment
	// that was already running the previous release, with real
	// pre-existing audit data, before the upgrade that adds 558.
	applyMigrationsInTxn(ctx, t, sqlDB, 1, 557)

	now := time.Now().UTC().Truncate(time.Microsecond)
	preUpgradeLogID := uuid.New()
	_, err := sqlDB.ExecContext(ctx, `
		INSERT INTO audit_logs (
			id, time, user_id, organization_id, resource_type, resource_id,
			resource_target, action, diff, status_code, additional_fields,
			request_id, resource_icon
		) VALUES (
			$1, $2, $3, $4, 'oauth2_provider_app', $5, 'pre-upgrade-app',
			'write', '{}', 200, '{}', $6, ''
		)`,
		preUpgradeLogID, now, uuid.New(), uuid.New(), uuid.New(), uuid.New(),
	)
	require.NoError(t, err)

	// Apply 558 in the same single transaction production uses for the
	// whole pending batch.
	applyMigrationsInTxn(ctx, t, sqlDB, 558, 558)

	// Pre-existing audit data survives the upgrade untouched.
	var resourceTarget string
	err = sqlDB.QueryRowContext(ctx,
		`SELECT resource_target FROM audit_logs WHERE id = $1`, preUpgradeLogID,
	).Scan(&resourceTarget)
	require.NoError(t, err)
	require.Equal(t, "pre-upgrade-app", resourceTarget)

	// The new enum value is usable immediately after the migration
	// transaction commits, in a separate transaction, exactly like a real
	// PUT to the DCR settings endpoint would write it.
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO audit_logs (
			id, time, user_id, organization_id, resource_type, resource_id,
			resource_target, action, diff, status_code, additional_fields,
			request_id, resource_icon
		) VALUES (
			$1, $2, $3, $4, 'oauth2_provider_settings', $5, '', 'write',
			'{}', 200, '{}', $6, ''
		)`,
		uuid.New(), now, uuid.New(), uuid.New(), uuid.New(), uuid.New(),
	)
	require.NoError(t, err)
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

func TestMigration000556UserSecretsEnabled(t *testing.T) {
	t.Parallel()

	const migrationVersion = 556

	sqlDB := testSQLDB(t)

	// Migrate up to the migration before the one that adds the enabled
	// column.
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

	userID := uuid.New()
	envSecretID := uuid.New()
	fileSecretID := uuid.New()
	bothEmptySecretID := uuid.New()

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
			[]any{userID, "user-secrets-enabled", "user-secrets-enabled@test.com", []byte{}, now, now, "active", pq.StringArray{}, "password"},
		},
		// env-only secret: should remain enabled after migration.
		{
			`INSERT INTO user_secrets (id, user_id, name, description, value, env_name, file_path, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			[]any{envSecretID, userID, "env-secret", "", "v1", "ENV_SECRET", "", now, now},
		},
		// file-only secret: should remain enabled after migration.
		{
			`INSERT INTO user_secrets (id, user_id, name, description, value, env_name, file_path, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			[]any{fileSecretID, userID, "file-secret", "", "v2", "", "/tmp/file-secret", now, now},
		},
		// Both env_name and file_path empty: silently skipped today by
		// the agent manifest layer. Should be flipped to enabled=false
		// by the migration so the behavior is preserved exactly under
		// the new "always inject when enabled" rule.
		{
			`INSERT INTO user_secrets (id, user_id, name, description, value, env_name, file_path, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			[]any{bothEmptySecretID, userID, "both-empty", "", "v3", "", "", now, now},
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

	getEnabled := func(t *testing.T, id uuid.UUID) bool {
		t.Helper()
		var enabled bool
		err := sqlDB.QueryRowContext(ctx,
			"SELECT enabled FROM user_secrets WHERE id = $1", id,
		).Scan(&enabled)
		require.NoError(t, err)
		return enabled
	}

	require.True(t, getEnabled(t, envSecretID),
		"env-only secret should remain enabled")
	require.True(t, getEnabled(t, fileSecretID),
		"file-only secret should remain enabled")
	require.False(t, getEnabled(t, bothEmptySecretID),
		"secret with both targets empty should be flipped to disabled "+
			"to preserve the previous implicit-skip behavior")
}

func TestMigration000562ChatModelConfigOrganization(t *testing.T) {
	t.Parallel()

	const migrationVersion = 562
	// The migration immediately before the org-scoping migration on this
	// branch. 561 is reserved for B1 (PR 27617), so the chain here is
	// 560 -> 562; the explicit predecessor is load-bearing across the gap.
	const previousMigrationVersion = 560

	sqlDB := testSQLDB(t)

	// Migrate up to the migration before the org-scoping migration.
	next, err := migrations.Stepper(sqlDB)
	require.NoError(t, err)
	for {
		version, more, err := next()
		require.NoError(t, err)
		if !more {
			t.Fatalf("migration %d not found", previousMigrationVersion)
		}
		if version == previousMigrationVersion {
			break
		}
	}

	ctx := testutil.Context(t, testutil.WaitSuperLong)

	providerID := uuid.New()
	defaultConfigID := uuid.New()
	plainConfigID := uuid.New()
	deletedConfigID := uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)

	fixtures := []struct {
		query string
		args  []any
	}{
		{
			`INSERT INTO ai_providers (id, type, name, enabled, base_url, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			[]any{providerID, "openai", "openai-552", true, "https://api.openai.com/v1", now, now},
		},
		// The deployment's single live default config.
		{
			`INSERT INTO chat_model_configs (id, model, display_name, enabled, is_default, context_limit, compression_threshold, ai_provider_id, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			[]any{defaultConfigID, "gpt-5.2", "Default 552", true, true, 200000, 70, providerID, now, now},
		},
		// A live non-default config.
		{
			`INSERT INTO chat_model_configs (id, model, display_name, enabled, is_default, context_limit, compression_threshold, ai_provider_id, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			[]any{plainConfigID, "gpt-5.2-mini", "Plain 552", true, false, 128000, 70, providerID, now, now},
		},
		// A soft-deleted config: backfilled and ACL-seeded like any row,
		// while staying outside the partial default index.
		{
			`INSERT INTO chat_model_configs (id, model, display_name, enabled, is_default, deleted, deleted_at, context_limit, compression_threshold, ai_provider_id, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			[]any{deletedConfigID, "gpt-4-legacy", "Deleted 552", false, false, true, now, 128000, 70, providerID, now, now},
		},
	}

	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()
	for i, f := range fixtures {
		_, err := tx.ExecContext(ctx, f.query, f.args...)
		require.NoError(t, err, "fixture %d", i)
	}
	require.NoError(t, tx.Commit())

	// Run the migration.
	version, _, err := next()
	require.NoError(t, err)
	require.EqualValues(t, migrationVersion, version)

	var defaultOrgID uuid.UUID
	err = sqlDB.QueryRowContext(ctx,
		"SELECT id FROM organizations WHERE is_default = true",
	).Scan(&defaultOrgID)
	require.NoError(t, err)

	// Every row, including the soft-deleted one, is backfilled to the
	// default org.
	var backfilled int
	err = sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM chat_model_configs WHERE organization_id = $1", defaultOrgID,
	).Scan(&backfilled)
	require.NoError(t, err)
	require.Equal(t, 3, backfilled, "all fixture rows should be backfilled to the default org")

	var notNullViolation bool
	err = sqlDB.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM chat_model_configs WHERE organization_id IS NULL)",
	).Scan(&notNullViolation)
	require.NoError(t, err)
	require.False(t, notNullViolation, "no row may keep a NULL organization_id")

	// Every row's group_acl is seeded with the everyone-in-org read
	// entry keyed by the org ID (the Everyone group shares the org ID).
	seededACL := map[string]any{
		defaultOrgID.String(): map[string]any{"permissions": []string{"read"}},
	}
	for _, id := range []uuid.UUID{defaultConfigID, plainConfigID, deletedConfigID} {
		var groupACL []byte
		err = sqlDB.QueryRowContext(ctx,
			"SELECT group_acl FROM chat_model_configs WHERE id = $1", id,
		).Scan(&groupACL)
		require.NoError(t, err)
		require.JSONEq(t, string(mustJSON(t, seededACL)), string(groupACL),
			"group_acl should carry the everyone-in-org read entry")
	}

	// The single-default index is now keyed per organization: the index
	// definition references organization_id and keeps its predicate.
	var indexDef string
	err = sqlDB.QueryRowContext(ctx,
		"SELECT indexdef FROM pg_indexes WHERE indexname = 'idx_chat_model_configs_single_default'",
	).Scan(&indexDef)
	require.NoError(t, err)
	require.Contains(t, indexDef, "organization_id")
	require.Contains(t, indexDef, "is_default = true")
	require.Contains(t, indexDef, "deleted = false")

	// The org lookup index exists.
	var orgIndexExists bool
	err = sqlDB.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname = 'idx_chat_model_configs_organization_id')",
	).Scan(&orgIndexExists)
	require.NoError(t, err)
	require.True(t, orgIndexExists)

	// The default org can host only one live default: a second insert
	// violates the per-org partial unique index.
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO chat_model_configs (id, model, display_name, enabled, is_default, context_limit, compression_threshold, ai_provider_id, organization_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		uuid.New(), "gpt-5.2-alt", "Second Default 552", true, true, 200000, 70, providerID, defaultOrgID, now, now,
	)
	require.Error(t, err, "a second live default in the same org must be rejected")
	require.Contains(t, err.Error(), "idx_chat_model_configs_single_default")

	// A second org can host its own live default.
	secondOrgID := uuid.New()
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO organizations (id, name, description, display_name, default_org_member_roles, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		secondOrgID, "second-org-552", "", "", pq.StringArray{}, now, now,
	)
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO chat_model_configs (id, model, display_name, enabled, is_default, context_limit, compression_threshold, ai_provider_id, organization_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		uuid.New(), "gpt-5.2-org2", "Second Org Default 552", true, true, 200000, 70, providerID, secondOrgID, now, now,
	)
	require.NoError(t, err, "each org can host its own live default")

	// The ACL object CHECKs reject non-object values.
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO chat_model_configs (id, model, display_name, enabled, context_limit, compression_threshold, ai_provider_id, organization_id, group_acl, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, '[]'::jsonb, $9, $10)`,
		uuid.New(), "bad-acl", "Bad ACL 552", true, 128000, 70, providerID, secondOrgID, now, now,
	)
	require.Error(t, err, "non-object group_acl must be rejected")
}

// mustJSON marshals a value for JSONEq comparisons, failing the test on
// error.
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return raw
}

func TestMigration000564ChatModelConfigOrgExplosion(t *testing.T) {
	t.Parallel()

	const migrationVersion = 564

	sqlDB := testSQLDB(t)

	// stepperUpToLatest runs the stepper to completion: a Stepper cannot
	// stop early, it closes only when the steps are exhausted, and each
	// call commits the driver's transaction. The assertion only requires
	// that target was APPLIED, not that it is the latest: a stacked PR may
	// add a later migration, so encoding "target is latest" would redden
	// any such child on its merge ref.
	stepperUpToLatest := func(target uint) {
		t.Helper()
		next, err := migrations.Stepper(sqlDB)
		require.NoError(t, err)
		last := uint(0)
		for {
			version, more, err := next()
			require.NoError(t, err)
			if !more {
				break
			}
			last = version
		}
		require.GreaterOrEqual(t, last, target, "migration %d must be applied", target)
	}

	// Migrate fully up first. The Stepper cannot stop at an intermediate
	// version (it runs to completion and each run is one transaction), so
	// fixtures are seeded post-schema and the explosion is exercised as
	// DOWN (restore pre-explosion state) then UP (re-explode from it).
	stepperUpToLatest(migrationVersion)

	ctx := testutil.Context(t, testutil.WaitSuperLong)

	now := time.Now().UTC().Truncate(time.Microsecond)
	providerID := uuid.New()
	user1ID := uuid.New()
	user2ID := uuid.New()
	orgBID := uuid.New() // live org with chats
	orgCID := uuid.New() // live zero-member org: receives the full live set
	orgDID := uuid.New() // soft-deleted org: receives nothing, chats untouched
	c1ID := uuid.New()   // live default config
	c2ID := uuid.New()   // live plain config
	c3ID := uuid.New()   // soft-deleted, referenced in orgB only
	c4ID := uuid.New()   // soft-deleted, unreferenced: never copied
	c5ID := uuid.New()   // live plain config
	// aclLateConfigID simulates a config created between 000562 and 000564 by
	// the pre-cutover handlers, which did not seed the everyone ACL entry.
	aclLateConfigID := uuid.New()
	chatBID := uuid.New()  // chat in orgB pinned to live c1
	chatB3ID := uuid.New() // chat in orgB pinned to deleted c3
	chatDID := uuid.New()  // chat in soft-deleted orgD pinned to live c1

	execFixture := func(query string, args ...any) {
		t.Helper()
		_, err := sqlDB.ExecContext(ctx, query, args...)
		require.NoError(t, err)
	}

	for i, id := range []uuid.UUID{user1ID, user2ID} {
		execFixture(
			`INSERT INTO users (id, username, email, hashed_password, created_at, updated_at, status, rbac_roles, login_type)
			VALUES ($1, $2, $3, $4, $5, $6, 'active', '{}', 'password')`,
			id, fmt.Sprintf("m3user%d", i+1), fmt.Sprintf("m3user%d@coder.com", i+1), []byte{}, now, now,
		)
	}

	execFixture(
		`INSERT INTO ai_providers (id, type, name, enabled, base_url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		providerID, "openai", "openai-564", true, "https://api.openai.com/v1", now, now,
	)

	// Three non-default orgs: live B, live zero-member C, soft-deleted D.
	for _, o := range []struct {
		id      uuid.UUID
		name    string
		deleted bool
	}{
		{orgBID, "org-b-564", false},
		{orgCID, "org-c-564", false},
		{orgDID, "org-d-564", true},
	} {
		execFixture(
			`INSERT INTO organizations (id, name, description, display_name, default_org_member_roles, created_at, updated_at, deleted)
			VALUES ($1, $2, '', '', '{}', $3, $3, $4)`,
			o.id, o.name, now, o.deleted,
		)
	}

	var defaultOrgID uuid.UUID
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT id FROM organizations WHERE is_default = true").Scan(&defaultOrgID))

	// Model configs in the default org (pre-564 state: every config lives
	// there, backfilled by 000562 with the everyone ACL entry).
	insertConfig := func(id uuid.UUID, model string, isDefault, deleted bool, acl string) {
		t.Helper()
		execFixture(
			`INSERT INTO chat_model_configs (id, model, display_name, enabled, is_default, deleted, deleted_at,
				context_limit, compression_threshold, ai_provider_id, organization_id, group_acl, created_at, updated_at)
			VALUES ($1, $2, $3, true, $4, $5, (CASE WHEN $5 THEN $6::timestamptz ELSE NULL END),
				200000, 70, $7, $8, $9::jsonb, $6, $6)`,
			id, model, model+" display", isDefault, deleted, now, providerID, defaultOrgID, acl,
		)
	}
	everyoneACL := `{"` + defaultOrgID.String() + `": {"permissions": ["read"]}}`
	insertConfig(c1ID, "gpt-5.2", true, false, everyoneACL)
	insertConfig(c2ID, "gpt-5.2-mini", false, false, everyoneACL)
	insertConfig(c3ID, "gpt-4-legacy", false, true, everyoneACL)
	insertConfig(c4ID, "gpt-4-ancient", false, true, everyoneACL)
	insertConfig(c5ID, "gpt-5.2-nano", false, false, everyoneACL)
	insertConfig(aclLateConfigID, "gpt-5.2-late", false, false, `{}`)

	// Chats: orgB pinned to live c1, orgB second chat pinned to deleted c3,
	// soft-deleted orgD pinned to live c1.
	for _, ch := range []struct {
		id      uuid.UUID
		orgID   uuid.UUID
		ownerID uuid.UUID
		cfgID   uuid.UUID
	}{
		{chatBID, orgBID, user1ID, c1ID},
		{chatB3ID, orgBID, user2ID, c3ID},
		{chatDID, orgDID, user1ID, c1ID},
	} {
		execFixture(
			`INSERT INTO chats (id, owner_id, organization_id, last_model_config_id, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $5)`,
			ch.id, ch.ownerID, ch.orgID, ch.cfgID, now,
		)
	}

	// Messages in each chat referencing the chat's pinned config.
	for _, m := range []struct {
		chatID uuid.UUID
		cfgID  uuid.UUID
	}{
		{chatBID, c1ID},
		{chatB3ID, c3ID},
		{chatDID, c1ID},
	} {
		execFixture(
			`INSERT INTO chat_messages (chat_id, model_config_id, role, content, content_version)
			VALUES ($1, $2, 'user', '[]'::jsonb, 2)`,
			m.chatID, m.cfgID,
		)
	}

	// Queued messages (FK-less) referencing configs via their chat's org.
	execFixture(
		`INSERT INTO chat_queued_messages (chat_id, model_config_id, content, created_by)
		VALUES ($1, $2, '[]'::jsonb, $3)`,
		chatBID, c1ID, user1ID,
	)
	execFixture(
		`INSERT INTO chat_queued_messages (chat_id, model_config_id, content, created_by)
		VALUES ($1, $2, '[]'::jsonb, $3)`,
		chatDID, c1ID, user1ID,
	)

	// Debug runs (FK-less, attribution).
	execFixture(
		`INSERT INTO chat_debug_runs (id, chat_id, model_config_id, kind, status)
		VALUES ($1, $2, $3, 'turn', 'finished')`,
		uuid.New(), chatBID, c1ID,
	)
	execFixture(
		`INSERT INTO chat_debug_runs (id, chat_id, model_config_id, kind, status)
		VALUES ($1, $2, $3, 'turn', 'finished')`,
		uuid.New(), chatDID, c1ID,
	)

	// Compaction-threshold keys: c1 (live, users 1+2), c3 (deleted but
	// referenced in orgB, user 1), c4 (deleted unreferenced, user 1: must
	// never fan out), plus a non-threshold key that must stay untouched.
	thresholdKey := func(id uuid.UUID) string {
		return "chat_compaction_threshold_pct:" + id.String()
	}
	for _, tc := range []struct {
		userID uuid.UUID
		key    string
		value  string
	}{
		{user1ID, thresholdKey(c1ID), "80"},
		{user2ID, thresholdKey(c1ID), "75"},
		{user1ID, thresholdKey(c3ID), "60"},
		{user1ID, thresholdKey(c4ID), "55"},
		// Hostile keys: a malformed and an empty suffix. The up leaves
		// them alone; the down must not abort on their uuid cast.
		{user1ID, "chat_compaction_threshold_pct:not-a-uuid", "50"},
		{user1ID, "chat_compaction_threshold_pct:", "45"},
		{user1ID, "chat_personal_model_override:root", "chat_default"},
	} {
		execFixture(
			`INSERT INTO user_configs (user_id, key, value) VALUES ($1, $2, $3)`,
			tc.userID, tc.key, tc.value,
		)
	}

	// The fixtures were seeded post-schema in pre-explosion shape
	// (default-org configs, references pointing at originals). The Stepper
	// cannot stop mid-way, so the explosion of the seeded data is driven by
	// rewinding the version row to the predecessor and stepping to latest:
	// only 564's up re-applies.
	_, err := sqlDB.ExecContext(ctx,
		"TRUNCATE schema_migrations; INSERT INTO schema_migrations (version, dirty) VALUES (563, false)")
	require.NoError(t, err)
	stepperUpToLatest(migrationVersion)

	// copyID resolves the copy of orig in org by natural attributes (the
	// migration persists no mapping; this is also what the down relies on).
	copyID := func(origID, orgID uuid.UUID) (uuid.UUID, bool) {
		t.Helper()
		var id uuid.UUID
		err := sqlDB.QueryRowContext(ctx,
			`SELECT cp.id FROM chat_model_configs cp
			JOIN chat_model_configs orig ON orig.id = $1
			WHERE cp.organization_id = $2
			  AND cp.model = orig.model
			  AND cp.ai_provider_id IS NOT DISTINCT FROM orig.ai_provider_id
			  AND cp.id <> orig.id`, origID, orgID).Scan(&id)
		if err == sql.ErrNoRows {
			return uuid.Nil, false
		}
		require.NoError(t, err)
		return id, true
	}

	// --- Per-org row counts ---
	// Default org keeps its 6 originals; orgB gets 4 live copies (c1, c2,
	// c5, late) plus the referenced-deleted c3 copy; orgC (zero-member) gets
	// the full live set (4) and nothing deleted; orgD gets nothing.
	assertCount := func(orgID uuid.UUID, want int, msg string) {
		t.Helper()
		var got int
		require.NoError(t, sqlDB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM chat_model_configs WHERE organization_id = $1", orgID).Scan(&got))
		require.Equal(t, want, got, msg)
	}
	assertCount(defaultOrgID, 6, "default org keeps only its originals")
	assertCount(orgBID, 5, "orgB: 4 live fan-out + referenced-deleted c3")
	assertCount(orgCID, 4, "orgC: full live set, no deleted copies")
	assertCount(orgDID, 0, "soft-deleted org receives no copies")

	// The deleted copy preserves deleted/deleted_at.
	c3CopyB, ok := copyID(c3ID, orgBID)
	require.True(t, ok, "orgB received the referenced-deleted c3 copy")
	var deletedAtNull bool
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT deleted_at IS NULL FROM chat_model_configs WHERE id = $1", c3CopyB).Scan(&deletedAtNull))
	require.False(t, deletedAtNull, "deleted copy preserves deleted_at")

	// c4 is copied nowhere.
	for _, orgID := range []uuid.UUID{orgBID, orgCID, orgDID} {
		_, ok := copyID(c4ID, orgID)
		require.False(t, ok, "unreferenced deleted c4 must not be copied")
	}

	// --- Exactly one live default per org that received copies ---
	for _, orgID := range []uuid.UUID{defaultOrgID, orgBID, orgCID} {
		var defaults int
		require.NoError(t, sqlDB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM chat_model_configs WHERE organization_id = $1 AND is_default AND NOT deleted", orgID).Scan(&defaults))
		require.Equal(t, 1, defaults, "exactly one live default per org")
	}

	// --- Remaps ---
	// Chats in live orgB remap to same-org copies; the chat in soft-deleted
	// orgD keeps the original reference.
	c1CopyB, ok := copyID(c1ID, orgBID)
	require.True(t, ok)
	assertChatPinned := func(chatID, want uuid.UUID, msg string) {
		t.Helper()
		var got uuid.UUID
		require.NoError(t, sqlDB.QueryRowContext(ctx,
			"SELECT last_model_config_id FROM chats WHERE id = $1", chatID).Scan(&got))
		require.Equal(t, want, got, msg)
	}
	assertChatPinned(chatBID, c1CopyB, "orgB chat remapped to same-org live copy")
	assertChatPinned(chatB3ID, c3CopyB, "orgB chat on deleted model remapped to the deleted copy")
	assertChatPinned(chatDID, c1ID, "soft-deleted org chat keeps original reference")

	var msgCfg uuid.UUID
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT model_config_id FROM chat_messages WHERE chat_id = $1", chatBID).Scan(&msgCfg))
	require.Equal(t, c1CopyB, msgCfg, "orgB message remapped")
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT model_config_id FROM chat_messages WHERE chat_id = $1", chatB3ID).Scan(&msgCfg))
	require.Equal(t, c3CopyB, msgCfg, "orgB deleted-model message remapped")
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT model_config_id FROM chat_messages WHERE chat_id = $1", chatDID).Scan(&msgCfg))
	require.Equal(t, c1ID, msgCfg, "orgD message untouched")

	// Queued messages (FK-less): orgB remapped, orgD untouched.
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT model_config_id FROM chat_queued_messages WHERE chat_id = $1", chatBID).Scan(&msgCfg))
	require.Equal(t, c1CopyB, msgCfg, "orgB queued message remapped")
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT model_config_id FROM chat_queued_messages WHERE chat_id = $1", chatDID).Scan(&msgCfg))
	require.Equal(t, c1ID, msgCfg, "orgD queued message untouched")

	// Debug runs (FK-less): orgB remapped, orgD untouched.
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT model_config_id FROM chat_debug_runs WHERE chat_id = $1", chatBID).Scan(&msgCfg))
	require.Equal(t, c1CopyB, msgCfg, "orgB debug run remapped")
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT model_config_id FROM chat_debug_runs WHERE chat_id = $1", chatDID).Scan(&msgCfg))
	require.Equal(t, c1ID, msgCfg, "orgD debug run untouched")

	// No reference in a live non-default org still points at a default-org
	// config.
	var dangling int
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM chats c
		JOIN chat_model_configs cmc ON cmc.id = c.last_model_config_id
		JOIN organizations def ON def.id = cmc.organization_id AND def.is_default
		JOIN organizations co ON co.id = c.organization_id
		WHERE NOT co.is_default AND NOT co.deleted`).Scan(&dangling))
	require.Zero(t, dangling, "no live-org chat references a default-org config")

	// --- ACL re-key and backfill ---
	// Copies carry the everyone entry keyed by their own org.
	for _, orgID := range []uuid.UUID{orgBID, orgCID} {
		var missingACL int
		require.NoError(t, sqlDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM chat_model_configs
			WHERE organization_id = $1 AND NOT (group_acl ? $2)`,
			orgID, orgID.String()).Scan(&missingACL))
		require.Zero(t, missingACL, "every copy carries the everyone entry keyed by its org")
	}
	// Copies never carry the default org's ACL key.
	var leakedACL int
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM chat_model_configs
		WHERE organization_id <> $1 AND (group_acl ? $2)`,
		defaultOrgID, defaultOrgID.String()).Scan(&leakedACL))
	require.Zero(t, leakedACL, "copies must not inherit the default org's ACL key")
	// The pre-existing row with an empty group_acl was backfilled.
	var aclRaw []byte
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT group_acl FROM chat_model_configs WHERE id = $1", aclLateConfigID).Scan(&aclRaw))
	require.JSONEq(t, string(mustJSON(t, map[string]any{
		defaultOrgID.String(): map[string]any{"permissions": []string{"read"}},
	})), string(aclRaw), "row missing the everyone entry is backfilled")

	// --- Threshold fan-out ---
	// c1 keys fanned out to the orgB and orgC copies for both users (follows
	// copies, not membership); the c3 key fanned out to the orgB copy only;
	// c4 produced nothing.
	c1CopyC, ok := copyID(c1ID, orgCID)
	require.True(t, ok)
	for _, tc := range []struct {
		userID uuid.UUID
		cfgID  uuid.UUID
	}{
		{user1ID, c1CopyB},
		{user1ID, c1CopyC},
		{user2ID, c1CopyB},
		{user2ID, c1CopyC},
		{user1ID, c3CopyB},
	} {
		var exists bool
		require.NoError(t, sqlDB.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM user_configs WHERE user_id = $1 AND key = $2)",
			tc.userID, thresholdKey(tc.cfgID)).Scan(&exists))
		require.True(t, exists, "threshold key fanned out to copy %s for user %s", tc.cfgID, tc.userID)
	}
	// c3 produced no orgC key (its only copy is in orgB).
	var c3CCount int
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_configs uc
		JOIN chat_model_configs cp ON cp.id = substring(uc.key FROM 'chat_compaction_threshold_pct:(.*)')::uuid
		WHERE uc.key LIKE 'chat_compaction_threshold_pct:%'
		  AND substring(uc.key FROM 'chat_compaction_threshold_pct:(.*)') ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'
		  AND cp.organization_id = $1 AND cp.model = 'gpt-4-legacy'`,
		orgCID).Scan(&c3CCount))
	require.Zero(t, c3CCount, "deleted config with no orgC copy produces no orgC key")
	// c4 (deleted, unreferenced): the only ancient-model threshold key is the
	// seeded original.
	var c4Keys []string
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT key FROM user_configs WHERE key LIKE 'chat_compaction_threshold_pct:%'
		AND substring(key FROM 'chat_compaction_threshold_pct:(.*)') ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'
		AND substring(key FROM 'chat_compaction_threshold_pct:(.*)')::uuid = $1`, c4ID)
	require.NoError(t, err)
	for rows.Next() {
		var k string
		require.NoError(t, rows.Scan(&k))
		c4Keys = append(c4Keys, k)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Equal(t, []string{thresholdKey(c4ID)}, c4Keys, "unreferenced deleted c4 fans out zero keys")
	// Seeded original keys survive; the non-threshold key is untouched.
	for _, key := range []string{thresholdKey(c1ID), thresholdKey(c3ID)} {
		var exists bool
		require.NoError(t, sqlDB.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM user_configs WHERE user_id = $1 AND key = $2)",
			user1ID, key).Scan(&exists))
		require.True(t, exists, "original threshold key survives")
	}
	// The seeded sentinel row is re-keyed by 000566 (org-scoped personal
	// overrides), which runs after this migration in the stack: the
	// deployment-level key is fanned to the default org's suffixed key and
	// the original is deleted. Assert the post-stack shape here so this
	// test stays green on merge refs that include the child migration.
	var overrideValue string
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT value FROM user_configs uc
		JOIN organizations o ON o.is_default
		WHERE uc.user_id = $1 AND uc.key = 'chat_personal_model_override:' || o.id::text || ':root'`,
		user1ID).Scan(&overrideValue))
	require.Equal(t, "chat_default", overrideValue, "sentinel re-keyed to the default org by 000566")
	var legacyOverride bool
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM user_configs WHERE user_id = $1 AND key = 'chat_personal_model_override:root')",
		user1ID).Scan(&legacyOverride))
	require.False(t, legacyOverride, "000566 deleted the deployment-level key")

	// --- Down round-trip on the exploded state. The framework's Stepper
	// cannot commit after exactly one down step (its driver commits only
	// when the stepper exhausts), so the down file is executed directly:
	// migrations are plain SQL and this is the same text golang-migrate
	// would run.
	downSQL, err := fs.ReadFile(migrations.MigrationFS(), "000564_chat_model_config_org_explosion.down.sql")
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, string(downSQL))
	require.NoError(t, err)

	// Copies are gone; references restored to the default-org originals.
	assertCount(defaultOrgID, 6, "down: default org keeps its originals")
	assertCount(orgBID, 0, "down: copies deleted from orgB")
	assertCount(orgCID, 0, "down: copies deleted from orgC")
	assertChatPinned(chatBID, c1ID, "down: orgB chat restored to original c1")
	assertChatPinned(chatB3ID, c3ID, "down: orgB chat restored to original c3")
	assertChatPinned(chatDID, c1ID, "down: orgD chat unchanged")
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT model_config_id FROM chat_messages WHERE chat_id = $1", chatBID).Scan(&msgCfg))
	require.Equal(t, c1ID, msgCfg, "down: orgB message restored")
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT model_config_id FROM chat_queued_messages WHERE chat_id = $1", chatBID).Scan(&msgCfg))
	require.Equal(t, c1ID, msgCfg, "down: orgB queued message restored")
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT model_config_id FROM chat_debug_runs WHERE chat_id = $1", chatBID).Scan(&msgCfg))
	require.Equal(t, c1ID, msgCfg, "down: orgB debug run restored")

	// Fanned-out threshold keys are gone; exactly the seeded valid key set
	// remains. The two hostile keys (malformed/empty suffix) are pruned as
	// dangling: they cannot name an existing config, and the down must not
	// abort on their uuid cast.
	var thresholdCount int
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM user_configs WHERE key LIKE 'chat_compaction_threshold_pct:%'").Scan(&thresholdCount))
	require.Equal(t, 4, thresholdCount, "down: only the 4 seeded valid threshold keys remain")

	// Step forward again: the down executed outside the driver, so rewind
	// the version row and re-apply the up. Shape re-asserted at count
	// level.
	_, err = sqlDB.ExecContext(ctx,
		"TRUNCATE schema_migrations; INSERT INTO schema_migrations (version, dirty) VALUES (563, false)")
	require.NoError(t, err)
	stepperUpToLatest(migrationVersion)
	assertCount(orgBID, 5, "re-up: orgB copies recreated")
	assertCount(orgCID, 4, "re-up: orgC copies recreated")
	assertChatPinned(chatDID, c1ID, "re-up: orgD still untouched")
}

func TestMigration000566ChatModelOverrideOrgScope(t *testing.T) {
	t.Parallel()

	const migrationVersion = 566

	sqlDB := testSQLDB(t)

	// stepperUpToLatest runs the stepper to completion and asserts the
	// target was APPLIED, never that it is the latest: stacked PRs add
	// later migrations and a latest-equality assertion reddens every
	// child merge ref.
	stepperUpToLatest := func(target uint) {
		t.Helper()
		next, err := migrations.Stepper(sqlDB)
		require.NoError(t, err)
		last := uint(0)
		for {
			version, more, err := next()
			require.NoError(t, err)
			if !more {
				break
			}
			last = version
		}
		require.GreaterOrEqual(t, last, target, "migration %d must be applied", target)
	}

	// Migrate fully up first (the Stepper cannot stop mid-way), seed the
	// pre-566 shape, then rewind the version row so only 566's up
	// re-applies against the seeded data.
	stepperUpToLatest(migrationVersion)

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	now := time.Now().UTC().Truncate(time.Microsecond)

	providerID := uuid.New()
	user1ID := uuid.New()
	user2ID := uuid.New()
	c1ID := uuid.New() // live default-org config
	c3ID := uuid.New() // soft-deleted default-org config

	execFixture := func(query string, args ...any) {
		t.Helper()
		_, err := sqlDB.ExecContext(ctx, query, args...)
		require.NoError(t, err)
	}

	execFixture(
		`INSERT INTO users (id, username, email, hashed_password, created_at, updated_at, status, rbac_roles, login_type)
		VALUES ($1, 'm5user1', 'm5user1@coder.com', $2, $3, $3, 'active', '{}', 'password')`,
		user1ID, []byte{}, now,
	)
	execFixture(
		`INSERT INTO users (id, username, email, hashed_password, created_at, updated_at, status, rbac_roles, login_type)
		VALUES ($1, 'm5user2', 'm5user2@coder.com', $2, $3, $3, 'active', '{}', 'password')`,
		user2ID, []byte{}, now,
	)
	execFixture(
		`INSERT INTO ai_providers (id, type, name, enabled, base_url, created_at, updated_at)
		VALUES ($1, 'openai', 'openai-566', true, 'https://api.openai.com/v1', $2, $2)`,
		providerID, now,
	)

	var defaultOrgID uuid.UUID
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT id FROM organizations WHERE is_default = true").Scan(&defaultOrgID))

	insertConfig := func(id uuid.UUID, model string, deleted bool) {
		t.Helper()
		execFixture(
			`INSERT INTO chat_model_configs (id, model, display_name, enabled, is_default, deleted, deleted_at,
				context_limit, compression_threshold, ai_provider_id, organization_id, group_acl, created_at, updated_at)
			VALUES ($1, $2, $2, true, false, $3, (CASE WHEN $3 THEN $4::timestamptz ELSE NULL END),
				200000, 70, $5, $6, '{}'::jsonb, $4, $4)`,
			id, model, deleted, now, providerID, defaultOrgID,
		)
	}
	insertConfig(c1ID, "gpt-5.2-566", false)
	insertConfig(c3ID, "gpt-5.2-566-deleted", true)

	// site_configs seed: one value per required case.
	siteSeed := []struct{ key, value string }{
		// Identity re-key with effort suffix preserved.
		{"agents_chat_general_model_override", c1ID.String() + ":high"},
		// Empty global value: skipped, not seeded (the dogfood case).
		{"agents_chat_explore_model_override", ""},
		// Soft-deleted target: dropped, not re-keyed.
		{"agents_chat_title_generation_model_override", c3ID.String()},
		// Dangling target: skipped.
		{"agents_chat_compaction_model_override", uuid.New().String()},
		// Advisor blob with model fields plus runtime fields.
		{"agents_advisor_config", `{"enabled": true, "max_uses_per_run": 5, "max_output_tokens": 1024, "model_config_id": "` + c1ID.String() + `", "reasoning_effort": "low"}`},
		// Unrelated key survives untouched.
		{"agents_chat_system_prompt", "keep me"},
		// Hostile keys the re-key and delete must skip and the down must
		// not abort on.
		{"agents_chat_malformed_model_override:not-a-uuid", "junk"},
		{"agents_chat_general_model_override:", "junk2"},
		{"agents_advisor_model_override:not-a-uuid", "junk3"},
	}
	for _, row := range siteSeed {
		execFixture(`INSERT INTO site_configs (key, value) VALUES ($1, $2)`, row.key, row.value)
	}

	// user_configs seed: sentinels pass through, model-mode with effort
	// passes, deleted/dangling/malformed drop, unknown contexts untouched.
	userSeed := []struct {
		userID uuid.UUID
		key    string
		value  string
	}{
		{user1ID, "chat_personal_model_override:root", "model:" + c1ID.String() + ":max"},
		{user1ID, "chat_personal_model_override:general", "deployment_default"},
		{user1ID, "chat_personal_model_override:explore", "model:" + c3ID.String()},
		{user1ID, "chat_personal_model_override:evil", "chat_default"},
		{user1ID, "chat_personal_model_override:", "chat_default"},
		{user1ID, "theme_preference", "dark"},
		{user2ID, "chat_personal_model_override:root", "chat_default"},
		{user2ID, "chat_personal_model_override:general", "model:" + uuid.New().String()},
		{user2ID, "chat_personal_model_override:explore", "garbage-value"},
		{user2ID, "chat_personal_model_override:root:extra", "model:" + c1ID.String()},
	}
	for _, row := range userSeed {
		execFixture(`INSERT INTO user_configs (user_id, key, value) VALUES ($1, $2, $3)`,
			row.userID, row.key, row.value)
	}

	// Rewind the version row to 566's predecessor so the stepper re-applies
	// only 566's up. The predecessor is 564: 565 is allocated to a sibling
	// unit and absent here, and golang-migrate's readUp requires the
	// current version to exist in the source (the repo's Stepper swallows
	// the not-exist error as a silent no-op).
	_, err := sqlDB.ExecContext(ctx,
		"TRUNCATE schema_migrations; INSERT INTO schema_migrations (version, dirty) VALUES (564, false)")
	require.NoError(t, err)
	stepperUpToLatest(migrationVersion)

	getSiteValue := func(key string) (string, bool) {
		t.Helper()
		var value string
		err := sqlDB.QueryRowContext(ctx,
			"SELECT value FROM site_configs WHERE key = $1", key).Scan(&value)
		if err == sql.ErrNoRows {
			return "", false
		}
		require.NoError(t, err)
		return value, true
	}
	getUserValue := func(userID uuid.UUID, key string) (string, bool) {
		t.Helper()
		var value string
		err := sqlDB.QueryRowContext(ctx,
			"SELECT value FROM user_configs WHERE user_id = $1 AND key = $2", userID, key).Scan(&value)
		if err == sql.ErrNoRows {
			return "", false
		}
		require.NoError(t, err)
		return value, true
	}
	orgSuffix := ":" + defaultOrgID.String()

	// --- Up assertions: admin overrides ---
	value, ok := getSiteValue("agents_chat_general_model_override" + orgSuffix)
	require.True(t, ok, "default org general override seeded")
	require.Equal(t, c1ID.String()+":high", value, "identity re-key preserves the effort suffix")

	_, ok = getSiteValue("agents_chat_explore_model_override" + orgSuffix)
	require.False(t, ok, "empty global value is skipped, not seeded")

	_, ok = getSiteValue("agents_chat_title_generation_model_override" + orgSuffix)
	require.False(t, ok, "soft-deleted target is dropped, not re-keyed")

	_, ok = getSiteValue("agents_chat_compaction_model_override" + orgSuffix)
	require.False(t, ok, "dangling target is skipped")

	for _, base := range []string{
		"agents_chat_general_model_override",
		"agents_chat_explore_model_override",
		"agents_chat_title_generation_model_override",
		"agents_chat_compaction_model_override",
	} {
		_, ok = getSiteValue(base)
		require.False(t, ok, "old deployment-level key %s deleted", base)
	}

	// Non-default orgs receive no keys: the seed above has none, so every
	// uuid-suffixed key in the table must carry the default org's suffix.
	// Hostile keys without a uuid suffix are excluded from the count.
	var nonDefaultKeys int
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM site_configs
		WHERE key LIKE 'agents\_chat\_%\_model\_override:%'
		  AND key ~ '[0-9a-fA-F-]{36}$'
		  AND key NOT LIKE '%:' || $1::text`, defaultOrgID.String()).Scan(&nonDefaultKeys))
	require.Zero(t, nonDefaultKeys, "only the default org receives override keys")

	// --- Up assertions: advisor split ---
	value, ok = getSiteValue("agents_advisor_model_override" + orgSuffix)
	require.True(t, ok, "default org advisor override seeded")
	require.Equal(t, c1ID.String()+":low", value, "advisor model and effort move to the org key")

	value, ok = getSiteValue("agents_advisor_config")
	require.True(t, ok, "advisor config blob kept")
	require.JSONEq(t,
		`{"enabled": true, "max_uses_per_run": 5, "max_output_tokens": 1024}`,
		value,
		"advisor blob drops model fields and keeps runtime fields",
	)

	// Unrelated and hostile keys survive.
	value, ok = getSiteValue("agents_chat_system_prompt")
	require.True(t, ok)
	require.Equal(t, "keep me", value)
	_, ok = getSiteValue("agents_chat_malformed_model_override:not-a-uuid")
	require.True(t, ok, "hostile key untouched by the up")
	_, ok = getSiteValue("agents_chat_general_model_override:")
	require.True(t, ok, "empty-suffix hostile key untouched by the up")

	// --- Up assertions: personal overrides ---
	value, ok = getUserValue(user1ID, "chat_personal_model_override"+orgSuffix+":root")
	require.True(t, ok, "root model-mode value fanned to the default org")
	require.Equal(t, "model:"+c1ID.String()+":max", value, "effort suffix preserved")

	value, ok = getUserValue(user1ID, "chat_personal_model_override"+orgSuffix+":general")
	require.True(t, ok, "sentinel passes through")
	require.Equal(t, "deployment_default", value)

	_, ok = getUserValue(user1ID, "chat_personal_model_override"+orgSuffix+":explore")
	require.False(t, ok, "deleted-target personal value dropped")

	value, ok = getUserValue(user2ID, "chat_personal_model_override"+orgSuffix+":root")
	require.True(t, ok)
	require.Equal(t, "chat_default", value)

	_, ok = getUserValue(user2ID, "chat_personal_model_override"+orgSuffix+":general")
	require.False(t, ok, "dangling personal value dropped")

	_, ok = getUserValue(user2ID, "chat_personal_model_override"+orgSuffix+":explore")
	require.False(t, ok, "malformed personal value dropped")

	// Old keys in the three fanned contexts are gone; unknown-context and
	// unrelated rows survive.
	var legacyPersonal int
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_configs
		WHERE key IN ('chat_personal_model_override:root', 'chat_personal_model_override:general', 'chat_personal_model_override:explore')`).Scan(&legacyPersonal))
	require.Zero(t, legacyPersonal, "old personal override keys deleted")

	value, ok = getUserValue(user1ID, "chat_personal_model_override:evil")
	require.True(t, ok, "unknown-context key untouched by the up")
	require.Equal(t, "chat_default", value)
	value, ok = getUserValue(user1ID, "chat_personal_model_override:")
	require.True(t, ok, "empty-context hostile key untouched by the up")
	require.Equal(t, "chat_default", value)
	value, ok = getUserValue(user2ID, "chat_personal_model_override:root:extra")
	require.True(t, ok, "two-segment-context hostile key untouched by the up")
	require.Equal(t, "model:"+c1ID.String(), value)
	value, ok = getUserValue(user1ID, "theme_preference")
	require.True(t, ok)
	require.Equal(t, "dark", value)

	// --- Down: executed directly (the Stepper cannot commit after one
	// down step), asserting restoration and non-abort on hostile keys ---
	downSQL, err := fs.ReadFile(migrations.MigrationFS(), "000566_chat_model_override_org_scope.down.sql")
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, string(downSQL))
	require.NoError(t, err, "down must not abort on hostile keys")

	value, ok = getSiteValue("agents_chat_general_model_override")
	require.True(t, ok, "down restores the general base key")
	require.Equal(t, c1ID.String()+":high", value)

	// The empty/deleted/dangling pre-up values resolve to unset at read
	// time. The down does not distinguish them from never-set (the up
	// deleted their rows), so the base key is either absent or ''. Any
	// other restored value is a down bug: fail on it.
	for _, base := range []string{
		"agents_chat_explore_model_override",
		"agents_chat_title_generation_model_override",
		"agents_chat_compaction_model_override",
	} {
		value, ok = getSiteValue(base)
		require.True(t, !ok || value == "",
			"down restores %s as effectively unset (absent or empty); got ok=%v value=%q", base, ok, value)
	}

	value, ok = getSiteValue("agents_advisor_model_override")
	require.True(t, ok, "down restores the advisor override for downgrades")
	require.Equal(t, c1ID.String()+":low", value)

	value, ok = getSiteValue("agents_advisor_config")
	require.True(t, ok)
	require.JSONEq(t,
		`{"enabled": true, "max_uses_per_run": 5, "max_output_tokens": 1024}`,
		value,
		"down does not re-create model fields in the advisor blob",
	)

	var suffixedSite int
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM site_configs
		WHERE (key LIKE 'agents\_chat\_%\_model\_override:%' OR key LIKE 'agents\_advisor\_model\_override:%')
		  AND key ~ '[0-9a-fA-F-]{36}$'`).Scan(&suffixedSite))
	require.Zero(t, suffixedSite, "down removes all uuid-suffixed override keys")

	// Hostile keys survive the down too.
	_, ok = getSiteValue("agents_chat_malformed_model_override:not-a-uuid")
	require.True(t, ok, "hostile key survives the down")
	_, ok = getSiteValue("agents_advisor_model_override:not-a-uuid")
	require.True(t, ok, "hostile advisor key survives the down")

	// Personal fold-back.
	value, ok = getUserValue(user1ID, "chat_personal_model_override:root")
	require.True(t, ok, "down restores root personal override")
	require.Equal(t, "model:"+c1ID.String()+":max", value)
	value, ok = getUserValue(user1ID, "chat_personal_model_override:general")
	require.True(t, ok)
	require.Equal(t, "deployment_default", value)
	value, ok = getUserValue(user2ID, "chat_personal_model_override:root")
	require.True(t, ok)
	require.Equal(t, "chat_default", value)

	var suffixedPersonal int
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_configs
		WHERE key ~ '^chat_personal_model_override:[0-9a-fA-F-]{36}:(root|general|explore)$'`).Scan(&suffixedPersonal))
	require.Zero(t, suffixedPersonal, "down removes all org-suffixed personal keys")

	// Unknown-context rows are untouched in both directions.
	value, ok = getUserValue(user1ID, "chat_personal_model_override:evil")
	require.True(t, ok)
	require.Equal(t, "chat_default", value)

	// --- Re-up: rewind (to the existing predecessor, as above) and confirm
	// the up re-applies cleanly over the down's restored state ---
	_, err = sqlDB.ExecContext(ctx,
		"TRUNCATE schema_migrations; INSERT INTO schema_migrations (version, dirty) VALUES (564, false)")
	require.NoError(t, err)
	stepperUpToLatest(migrationVersion)

	value, ok = getSiteValue("agents_chat_general_model_override" + orgSuffix)
	require.True(t, ok, "re-up re-seeds the default org override")
	require.Equal(t, c1ID.String()+":high", value)
	value, ok = getUserValue(user1ID, "chat_personal_model_override"+orgSuffix+":root")
	require.True(t, ok, "re-up re-seeds the personal override")
	require.Equal(t, "model:"+c1ID.String()+":max", value)
}
