package database_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/codersdk"
)

// templateInsightsParams passes the registry's app names and the response
// limit, as the template insights handler does.
func templateInsightsParams(startTime, endTime time.Time, templateIDs ...uuid.UUID) database.GetTemplateInsightsParams {
	return database.GetTemplateInsightsParams{
		StartTime:           startTime,
		EndTime:             endTime,
		TemplateIDs:         templateIDs,
		RegisteredAppNames:  slices.Sorted(maps.Keys(codersdk.SessionCountAppFamilies())),
		MaxUnregisteredApps: codersdk.TemplateInsightsMaxUnregisteredApps,
	}
}

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
	insert(0, user, template, 1, map[string]int64{"cursor": 2, "vscode": 1, "new_app": 1, "overflow": 1})
	insert(30*time.Second, user, template, 0, map[string]int64{"cursor": 1, "new_app": 1, "overflow": 1})
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
	// Each app keeps its own minutes, including overlapping app usage.
	require.Equal(t, map[string]int64{"cursor": 2, "vscode": 2, "new_app": 1, "overflow": 1},
		sessionUsageMins(ctx, t, sqlDB, start, user, template))

	// The watermark recomputes recent buckets, so unchanged minutes must not
	// rewrite the row.
	version := sessionUsageRowVersion(ctx, t, sqlDB, start, user, template, "cursor")
	require.NoError(t, db.UpsertTemplateUsageStats(ctx))
	repeated, err := db.GetTemplateUsageStats(ctx, params)
	require.NoError(t, err)
	require.ElementsMatch(t, rows, repeated)
	require.Equal(t, map[string]int64{"cursor": 2, "vscode": 2, "new_app": 1, "overflow": 1},
		sessionUsageMins(ctx, t, sqlDB, start, user, template))
	require.Equal(t, version, sessionUsageRowVersion(ctx, t, sqlDB, start, user, template, "cursor"))

	// Template insights keep each app's own minutes, so apps open at the same
	// time add up past the active time, while no single app exceeds it.
	insights, err := db.GetTemplateInsights(ctx, templateInsightsParams(start, start.Add(time.Hour), template))
	require.NoError(t, err)
	require.EqualValues(t, 5*60, insights.UsageTotalSeconds)
	require.JSONEq(t, `{"cursor":180,"vscode":180,"new_app":60,"overflow":60}`, string(insights.SessionAppUsageSeconds))
	var appSeconds map[string]int64
	require.NoError(t, json.Unmarshal(insights.SessionAppUsageSeconds, &appSeconds))
	for name, seconds := range appSeconds {
		require.LessOrEqual(t, seconds, insights.UsageTotalSeconds, name)
	}

	// A new app in an already counted minute leaves the main row alone.
	insert(0, user, template, 1, map[string]int64{"another_app": 1})
	require.NoError(t, db.UpsertTemplateUsageStats(ctx))
	repeated, err = db.GetTemplateUsageStats(ctx, params)
	require.NoError(t, err)
	require.ElementsMatch(t, rows, repeated)
	require.Equal(t, map[string]int64{"cursor": 2, "vscode": 2, "new_app": 1, "overflow": 1, "another_app": 1},
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

	// Live insights count distinct minutes per family, with no 30-minute cap.
	live, err := db.GetTemplateInsightsByTemplate(ctx, templateInsightsByTemplateParams(t, start.Add(30*time.Second), start.Add(30*time.Minute)))
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
		_, err = sqlDB.ExecContext(ctx, `INSERT INTO template_usage_stats_session_apps(start_time,template_id,user_id,app_name,usage_mins) VALUES($1,$2,$3,'some_new_ide',20),($1,$2,$3,'ssh',20),($1,$2,$3,'sftp',2),($1,$2,$3,'overflow',20)`,
			start, template, user)
		require.NoError(t, err)
	}
	// App stats alone: no session usage, so no child rows.
	_, err := sqlDB.ExecContext(ctx, `INSERT INTO template_usage_stats(start_time,end_time,user_id,template_id,usage_mins,app_usage_mins) VALUES($1,$2,$3,$4,5,'{"ssh":5}')`,
		start, start.Add(30*time.Minute), uuid.New(), appTemplate)
	require.NoError(t, err)
	usage, err := db.GetTemplateInsights(ctx, templateInsightsParams(start, start.Add(30*time.Minute)))
	require.NoError(t, err)
	require.EqualValues(t, 2, usage.ActiveUsers)
	require.EqualValues(t, 35*60, usage.UsageTotalSeconds)
	require.ElementsMatch(t, []uuid.UUID{template1, template2, appTemplate}, usage.TemplateIDs)
	require.JSONEq(t, `{"some_new_ide":1800,"ssh":1800,"sftp":240,"overflow":1800}`, string(usage.SessionAppUsageSeconds))
	var appSeconds map[string]int64
	require.NoError(t, json.Unmarshal(usage.SessionAppUsageSeconds, &appSeconds))
	for name, seconds := range appSeconds {
		require.LessOrEqual(t, seconds, usage.UsageTotalSeconds, name)
	}
	var ids map[string][]uuid.UUID
	require.NoError(t, json.Unmarshal(usage.SessionAppTemplateIds, &ids))
	require.ElementsMatch(t, []uuid.UUID{template1, template2}, ids["some_new_ide"])
	require.ElementsMatch(t, []uuid.UUID{template1, template2}, ids["ssh"])
	onlyApp, err := db.GetTemplateInsights(ctx, templateInsightsParams(start, start.Add(30*time.Minute), appTemplate))
	require.NoError(t, err)
	require.EqualValues(t, 1, onlyApp.ActiveUsers)
	require.EqualValues(t, 300, onlyApp.UsageTotalSeconds)
	require.JSONEq(t, `{}`, string(onlyApp.SessionAppUsageSeconds))
	require.JSONEq(t, `{}`, string(onlyApp.SessionAppTemplateIds))
	// One template leaves a single-template user, so the cap does not apply.
	onlyOne, err := db.GetTemplateInsights(ctx, templateInsightsParams(start, start.Add(30*time.Minute), template1))
	require.NoError(t, err)
	require.JSONEq(t, `{"some_new_ide":1200,"ssh":1200,"sftp":120,"overflow":1200}`, string(onlyOne.SessionAppUsageSeconds))
}

func TestSessionUsageEmptyHistory(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	start := dbtime.Now().Truncate(30 * time.Minute)
	row, err := db.GetTemplateInsights(context.Background(), templateInsightsParams(start, start.Add(time.Hour)))
	require.NoError(t, err)
	require.Empty(t, row.TemplateIDs)
	require.Zero(t, row.ActiveUsers)
	require.Zero(t, row.UsageTotalSeconds)
	require.JSONEq(t, `{}`, string(row.SessionAppUsageSeconds))
	require.JSONEq(t, `{}`, string(row.SessionAppTemplateIds))
}

// TestSessionUsageResponseCap lists every registered app, the unknown row,
// and the busiest unregistered apps, folding the rest into overflow without
// touching what is stored.
func TestSessionUsageResponseCap(t *testing.T) {
	t.Parallel()
	sqlDB := testSQLDB(t)
	require.NoError(t, migrations.Up(sqlDB))
	db := database.New(sqlDB)
	ctx := context.Background()
	start := dbtime.Now().Add(-2 * time.Hour).Truncate(30 * time.Minute)
	userA, userB := uuid.New(), uuid.New()
	template1, template2, template3 := uuid.New(), uuid.New(), uuid.New()

	bucket := func(startTime time.Time, user, template uuid.UUID, usageMins int) {
		t.Helper()
		_, err := sqlDB.ExecContext(ctx, `INSERT INTO template_usage_stats(start_time,end_time,user_id,template_id,usage_mins) VALUES($1,$2,$3,$4,$5)`,
			startTime, startTime.Add(30*time.Minute), user, template, usageMins)
		require.NoError(t, err)
	}
	app := func(startTime time.Time, user, template uuid.UUID, appName string, usageMins int) {
		t.Helper()
		_, err := sqlDB.ExecContext(ctx, `INSERT INTO template_usage_stats_session_apps(start_time,template_id,user_id,app_name,usage_mins) VALUES($1,$2,$3,$4,$5)`,
			startTime, template, user, appName, usageMins)
		require.NoError(t, err)
	}

	// User A uses two templates in one half hour but is active for only 10
	// minutes of it, so the apps' minutes add up past the active time.
	bucket(start, userA, template1, 6)
	bucket(start, userA, template2, 4)
	// User B is active for 3 minutes in a third template.
	later := start.Add(30 * time.Minute)
	bucket(later, userB, template3, 3)

	limit := int(codersdk.TemplateInsightsMaxUnregisteredApps)
	// limit-1 unregistered apps at 6 minutes, then two at 4 minutes, so the
	// last listed slot goes to the name that sorts first.
	for i := range limit - 1 {
		app(start, userA, template1, fmt.Sprintf("app_%02d", i), 6)
	}
	app(start, userA, template1, "tie_a", 4)
	app(start, userA, template1, "tie_b", 4)
	// The least used unregistered apps, across templates and users.
	app(start, userA, template1, "fold_x", 2)
	app(later, userB, template3, "fold_x", 1)
	app(start, userA, template2, "fold_y", 2)
	// Registered apps stay listed however little they were used, and the
	// accounting rows never take an unregistered slot.
	app(start, userA, template1, "vscode", 1)
	app(start, userA, template2, "cursor", 1)
	app(start, userA, template1, "unknown", 3)
	app(start, userA, template1, "overflow", 4)

	usage, err := db.GetTemplateInsights(ctx, templateInsightsParams(start, start.Add(time.Hour)))
	require.NoError(t, err)
	require.EqualValues(t, 13*60, usage.UsageTotalSeconds)
	require.EqualValues(t, 3, usage.SessionFoldedAppCount)

	var appSeconds map[string]int64
	require.NoError(t, json.Unmarshal(usage.SessionAppUsageSeconds, &appSeconds))
	want := map[string]int64{"tie_a": 240, "vscode": 60, "cursor": 60, "unknown": 180}
	for i := range limit - 1 {
		want[fmt.Sprintf("app_%02d", i)] = 360
	}
	// The folded apps and the stored overflow sum to 12 minutes in user A's
	// half hour, bounded by its 10 active minutes, plus user B's 1 minute.
	// The sum alone would report 13 minutes.
	want["overflow"] = 11 * 60
	require.Equal(t, want, appSeconds)
	for name, seconds := range appSeconds {
		require.LessOrEqual(t, seconds, usage.UsageTotalSeconds, name)
	}

	var ids map[string][]uuid.UUID
	require.NoError(t, json.Unmarshal(usage.SessionAppTemplateIds, &ids))
	require.Len(t, ids, len(want))
	require.ElementsMatch(t, []uuid.UUID{template1, template2, template3}, ids["overflow"])
	require.ElementsMatch(t, []uuid.UUID{template2}, ids["cursor"])

	// The ranking is deterministic.
	again, err := db.GetTemplateInsights(ctx, templateInsightsParams(start, start.Add(time.Hour)))
	require.NoError(t, err)
	require.JSONEq(t, string(usage.SessionAppUsageSeconds), string(again.SessionAppUsageSeconds))

	// The response folds; storage keeps every app.
	var stored int
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM template_usage_stats_session_apps WHERE app_name IN ('tie_b', 'fold_x', 'fold_y')`).Scan(&stored))
	require.Equal(t, 4, stored)

	// Without apps to fold, overflow reports its stored minutes exactly.
	params := templateInsightsParams(start, start.Add(time.Hour))
	params.MaxUnregisteredApps = 1000
	unfolded, err := db.GetTemplateInsights(ctx, params)
	require.NoError(t, err)
	require.Zero(t, unfolded.SessionFoldedAppCount)
	require.NoError(t, json.Unmarshal(unfolded.SessionAppUsageSeconds, &appSeconds))
	require.EqualValues(t, 240, appSeconds["overflow"])
	require.EqualValues(t, 240, appSeconds["tie_b"])
	require.EqualValues(t, 180, appSeconds["fold_x"])
	require.NoError(t, json.Unmarshal(unfolded.SessionAppTemplateIds, &ids))
	require.ElementsMatch(t, []uuid.UUID{template1}, ids["overflow"])
}
