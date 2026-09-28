package database_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/migrations"
)

// sessionUsageMins reads one bucket's session usage. GetTemplateInsights caps
// and totals a window across users, so it cannot show a single bucket.
func sessionUsageMins(ctx context.Context, t *testing.T, sqlDB *sql.DB, startTime time.Time, userID, templateID uuid.UUID) map[string]int64 {
	t.Helper()

	rows, err := sqlDB.QueryContext(ctx, `SELECT app_name, usage_mins FROM template_usage_stats_session_apps
		WHERE start_time = $1 AND user_id = $2 AND template_id = $3`, startTime, userID, templateID)
	require.NoError(t, err)
	defer rows.Close()

	got := map[string]int64{}
	for rows.Next() {
		var name string
		var usageMins int64
		require.NoError(t, rows.Scan(&name, &usageMins))
		got[name] = usageMins
	}
	require.NoError(t, rows.Err())
	return got
}

// sessionUsageRowVersion tells a rewritten child row from an untouched one.
func sessionUsageRowVersion(ctx context.Context, t *testing.T, sqlDB *sql.DB, startTime time.Time, userID, templateID uuid.UUID, appName string) string {
	t.Helper()

	var version string
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT xmin::text FROM template_usage_stats_session_apps
			WHERE start_time = $1 AND user_id = $2 AND template_id = $3 AND app_name = $4`,
		startTime, userID, templateID, appName).Scan(&version))
	return version
}

func TestSessionUsageRollup(t *testing.T) {
	t.Parallel()
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ctx := context.Background()
	start := dbtime.Now().Add(-3 * time.Hour).Truncate(30 * time.Minute)
	user, template := uuid.New(), uuid.New()
	insert := func(offset time.Duration, userID, templateID uuid.UUID, connected int64, counts map[string]int64) {
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt: start.Add(offset), UserID: userID, TemplateID: templateID, AgentID: uuid.New(),
			ConnectionCount: connected, SessionCounts: dbgen.SessionCounts(t, counts),
		})
	}
	insert(0, user, template, 1, map[string]int64{"cursor": 2, "vscode": 1, "new_app": 1})
	insert(30*time.Second, user, template, 0, map[string]int64{"cursor": 1, "new_app": 1})
	insert(time.Minute, user, template, 0, map[string]int64{"cursor": 1})
	insert(29*time.Minute, user, template, 0, map[string]int64{"vscode": 1})
	insert(30*time.Minute, user, template, 1, map[string]int64{"cursor": 1})
	insert(0, uuid.New(), template, 1, map[string]int64{"vscode": 1})
	insert(0, user, uuid.New(), 1, map[string]int64{"cursor": 1})
	disconnected := uuid.New()
	insert(0, user, disconnected, 0, map[string]int64{"cursor": 1})
	empty := uuid.New()
	insert(0, user, empty, 1, map[string]int64{})

	require.NoError(t, db.UpsertTemplateUsageStats(ctx))
	params := database.GetTemplateUsageStatsParams{StartTime: start, EndTime: start.Add(time.Hour)}
	rows, err := db.GetTemplateUsageStats(ctx, params)
	require.NoError(t, err)
	require.Len(t, rows, 4)
	found := false
	for _, row := range rows {
		require.NotEqual(t, disconnected, row.TemplateID)
		require.NotEqual(t, empty, row.TemplateID)
		if row.UserID == user && row.TemplateID == template && row.StartTime.Equal(start) {
			found = true
			require.EqualValues(t, 3, row.UsageMins)
		}
	}
	require.True(t, found, "the overlapping app bucket must be present")
	// Each app keeps its own minutes; callers add up a family's apps.
	require.Equal(t, map[string]int64{"cursor": 2, "vscode": 2, "new_app": 1},
		sessionUsageMins(ctx, t, sqlDB, start, user, template))

	// The watermark recomputes recent buckets, so unchanged minutes must not
	// rewrite the row.
	version := sessionUsageRowVersion(ctx, t, sqlDB, start, user, template, "cursor")
	require.NoError(t, db.UpsertTemplateUsageStats(ctx))
	repeated, err := db.GetTemplateUsageStats(ctx, params)
	require.NoError(t, err)
	require.ElementsMatch(t, rows, repeated)
	require.Equal(t, map[string]int64{"cursor": 2, "vscode": 2, "new_app": 1},
		sessionUsageMins(ctx, t, sqlDB, start, user, template))
	require.Equal(t, version, sessionUsageRowVersion(ctx, t, sqlDB, start, user, template, "cursor"))

	// A new app in an already counted minute leaves the main row alone.
	insert(0, user, template, 1, map[string]int64{"another_app": 1})
	require.NoError(t, db.UpsertTemplateUsageStats(ctx))
	repeated, err = db.GetTemplateUsageStats(ctx, params)
	require.NoError(t, err)
	require.ElementsMatch(t, rows, repeated)
	require.Equal(t, map[string]int64{"cursor": 2, "vscode": 2, "new_app": 1, "another_app": 1},
		sessionUsageMins(ctx, t, sqlDB, start, user, template))
	require.Equal(t, version, sessionUsageRowVersion(ctx, t, sqlDB, start, user, template, "cursor"))

	// An app-stats-only bucket records no session usage, and a deleted bucket
	// takes its session usage with it.
	org := dbgen.Organization(t, db, database.Organization{})
	owner := dbgen.User(t, db, database.User{Name: "app-stats-only"})
	appTemplate := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: owner.ID})
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{OrganizationID: org.ID, TemplateID: appTemplate.ID, OwnerID: owner.ID})
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{OrganizationID: org.ID})
	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: job.ID})
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: resource.ID})
	dbgen.WorkspaceAppStat(t, db, database.WorkspaceAppStat{
		UserID:           owner.ID,
		WorkspaceID:      workspace.ID,
		AgentID:          agent.ID,
		AccessMethod:     "path",
		SlugOrPort:       "code-server",
		SessionStartedAt: start.Add(time.Minute),
		SessionEndedAt:   start.Add(3 * time.Minute),
	})
	require.NoError(t, db.UpsertTemplateUsageStats(ctx))
	require.Empty(t, sessionUsageMins(ctx, t, sqlDB, start, owner.ID, appTemplate.ID))

	require.Empty(t, sessionUsageMins(ctx, t, sqlDB, start, user, disconnected))
	_, err = sqlDB.ExecContext(ctx, `DELETE FROM template_usage_stats WHERE start_time = $1 AND user_id = $2 AND template_id = $3`, start, user, template)
	require.NoError(t, err)
	require.Empty(t, sessionUsageMins(ctx, t, sqlDB, start, user, template))

	// Live insights report the raw app names without half-hour caps.
	live, err := db.GetTemplateInsightsByTemplate(ctx, database.GetTemplateInsightsByTemplateParams{
		StartTime: start.Add(30 * time.Second), EndTime: start.Add(30 * time.Minute),
	})
	require.NoError(t, err)
	require.Len(t, live, 0, "no connected report occurs in this partial request window")
}

// TestSessionUsageHistoryCapsAndNamespaces reads history the rollup cannot
// produce: family-named rows, which only migration 000596 writes, and an app
// name the registry does not know. Neither arrives through agent stats, so the
// buckets are seeded directly.
func TestSessionUsageHistoryCapsAndNamespaces(t *testing.T) {
	t.Parallel()
	sqlDB := testSQLDB(t)
	require.NoError(t, migrations.Up(sqlDB))
	db := database.New(sqlDB)
	ctx := context.Background()
	start := dbtime.Now().Add(-2 * time.Hour).Truncate(30 * time.Minute)
	user, template1, template2, appTemplate := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	// One user in two templates in one half hour, so the minutes cap at 30.
	for _, template := range []uuid.UUID{template1, template2} {
		_, err := sqlDB.ExecContext(ctx, `INSERT INTO template_usage_stats(start_time,end_time,user_id,template_id,usage_mins) VALUES($1,$2,$3,$4,20)`,
			start, start.Add(30*time.Minute), user, template)
		require.NoError(t, err)
		_, err = sqlDB.ExecContext(ctx, `INSERT INTO template_usage_stats_session_apps(start_time,template_id,user_id,app_name,usage_mins) VALUES($1,$2,$3,'some_new_ide',20),($1,$2,$3,'ssh',20),($1,$2,$3,'sftp',2)`,
			start, template, user)
		require.NoError(t, err)
	}
	// App stats alone: no session usage, so no child rows.
	_, err := sqlDB.ExecContext(ctx, `INSERT INTO template_usage_stats(start_time,end_time,user_id,template_id,usage_mins,app_usage_mins) VALUES($1,$2,$3,$4,5,'{"ssh":5}')`,
		start, start.Add(30*time.Minute), uuid.New(), appTemplate)
	require.NoError(t, err)
	usage, err := db.GetTemplateInsights(ctx, database.GetTemplateInsightsParams{StartTime: start, EndTime: start.Add(30 * time.Minute)})
	require.NoError(t, err)
	require.EqualValues(t, 2, usage.ActiveUsers)
	require.EqualValues(t, 35*60, usage.UsageTotalSeconds)
	require.ElementsMatch(t, []uuid.UUID{template1, template2, appTemplate}, usage.TemplateIDs)
	require.JSONEq(t, `{"some_new_ide":1800,"ssh":1800,"sftp":240}`, string(usage.SessionAppUsageSeconds))
	var ids map[string][]uuid.UUID
	require.NoError(t, json.Unmarshal(usage.SessionAppTemplateIds, &ids))
	require.ElementsMatch(t, []uuid.UUID{template1, template2}, ids["some_new_ide"])
	require.ElementsMatch(t, []uuid.UUID{template1, template2}, ids["ssh"])
	onlyApp, err := db.GetTemplateInsights(ctx, database.GetTemplateInsightsParams{StartTime: start, EndTime: start.Add(30 * time.Minute), TemplateIDs: []uuid.UUID{appTemplate}})
	require.NoError(t, err)
	require.EqualValues(t, 1, onlyApp.ActiveUsers)
	require.EqualValues(t, 300, onlyApp.UsageTotalSeconds)
	require.JSONEq(t, `{}`, string(onlyApp.SessionAppUsageSeconds))
	require.JSONEq(t, `{}`, string(onlyApp.SessionAppTemplateIds))
	// One template leaves a single-template user, so the cap does not apply.
	onlyOne, err := db.GetTemplateInsights(ctx, database.GetTemplateInsightsParams{StartTime: start, EndTime: start.Add(30 * time.Minute), TemplateIDs: []uuid.UUID{template1}})
	require.NoError(t, err)
	require.JSONEq(t, `{"some_new_ide":1200,"ssh":1200,"sftp":120}`, string(onlyOne.SessionAppUsageSeconds))
}

func TestSessionUsageEmptyHistory(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	start := dbtime.Now().Truncate(30 * time.Minute)
	row, err := db.GetTemplateInsights(context.Background(), database.GetTemplateInsightsParams{StartTime: start, EndTime: start.Add(time.Hour)})
	require.NoError(t, err)
	require.Empty(t, row.TemplateIDs)
	require.Zero(t, row.ActiveUsers)
	require.Zero(t, row.UsageTotalSeconds)
	require.JSONEq(t, `{}`, string(row.SessionAppUsageSeconds))
	require.JSONEq(t, `{}`, string(row.SessionAppTemplateIds))
}
