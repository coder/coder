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
	"github.com/coder/coder/v2/codersdk"
)

// sessionUsageMins reads a session usage child table for one bucket. There is
// no sqlc query for the child tables, so the tests read them directly.
func sessionUsageMins(ctx context.Context, t *testing.T, sqlDB *sql.DB, table, nameColumn string, startTime time.Time, userID, templateID uuid.UUID) map[string]int64 {
	t.Helper()

	//nolint:gosec // Table and column names are constants in this test.
	rows, err := sqlDB.QueryContext(ctx, "SELECT "+nameColumn+", usage_mins FROM "+table+
		" WHERE start_time = $1 AND user_id = $2 AND template_id = $3", startTime, userID, templateID)
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

// sessionUsageDigest reads the digest the rollup stored for one bucket, and
// emptySessionUsageDigest is the digest of a bucket with no session usage.
func sessionUsageDigest(ctx context.Context, t *testing.T, sqlDB *sql.DB, startTime time.Time, userID, templateID uuid.UUID) sql.NullInt64 {
	t.Helper()

	var digest sql.NullInt64
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT session_usage_digest FROM template_usage_stats WHERE start_time = $1 AND user_id = $2 AND template_id = $3`,
		startTime, userID, templateID).Scan(&digest))
	return digest
}

func emptySessionUsageDigest(ctx context.Context, t *testing.T, sqlDB *sql.DB) sql.NullInt64 {
	t.Helper()

	var digest sql.NullInt64
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT hashtextextended('', 0)`).Scan(&digest))
	return digest
}

// withoutDigest clears the session usage digest, which is a hash of the child
// rows rather than a value worth comparing.
func withoutDigest(stats []database.TemplateUsageStat) []database.TemplateUsageStat {
	cleared := make([]database.TemplateUsageStat, 0, len(stats))
	for _, stat := range stats {
		stat.SessionUsageDigest = sql.NullInt64{}
		cleared = append(cleared, stat)
	}
	return cleared
}

func TestSessionUsageMapNull(t *testing.T) {
	t.Parallel()
	m := database.StringMapOfInt{"cursor": 1}
	require.NoError(t, m.Scan(nil))
	require.Nil(t, m)
	require.NoError(t, m.Scan([]byte(`{}`)))
	require.NotNil(t, m)
	require.Empty(t, m)
}

func TestSessionUsageRollupOverlapAndRegistry(t *testing.T) {
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

	var registry map[string]string
	require.NoError(t, json.Unmarshal(codersdk.SessionCountAppFamiliesJSON(), &registry))
	registry["new_app"] = "new_family"
	mapping, err := json.Marshal(registry)
	require.NoError(t, err)
	require.NoError(t, db.UpsertTemplateUsageStats(ctx, mapping))
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
	// Overlapping apps of one family share their minutes in the family table
	// but keep their own minutes in the app table.
	require.Equal(t, map[string]int64{"cursor": 2, "vscode": 2, "new_app": 1},
		sessionUsageMins(ctx, t, sqlDB, "template_usage_stats_session_apps", "app_name", start, user, template))
	require.Equal(t, map[string]int64{"vscode": 3, "new_family": 1},
		sessionUsageMins(ctx, t, sqlDB, "template_usage_stats_session_families", "family", start, user, template))
	digest := sessionUsageDigest(ctx, t, sqlDB, start, user, template)
	require.True(t, digest.Valid, "a rolled up bucket must carry a session usage digest")
	require.NotEqual(t, emptySessionUsageDigest(ctx, t, sqlDB), digest, "this bucket has session usage")

	// The existing watermark deliberately recomputes recent buckets.
	require.NoError(t, db.UpsertTemplateUsageStats(ctx, mapping))
	repeated, err := db.GetTemplateUsageStats(ctx, params)
	require.NoError(t, err)
	require.ElementsMatch(t, rows, repeated)
	require.Equal(t, map[string]int64{"vscode": 3, "new_family": 1},
		sessionUsageMins(ctx, t, sqlDB, "template_usage_stats_session_families", "family", start, user, template))
	// Recomputing identical session usage leaves the digest alone, which is
	// what keeps the rollup from rewriting the child rows of every bucket in
	// its window.
	require.Equal(t, digest, sessionUsageDigest(ctx, t, sqlDB, start, user, template))

	// Renaming a family in the registry moves the minutes: the recomputed
	// bucket drops the family row it no longer has and keeps its app rows,
	// even though the main row itself does not change.
	registry["new_app"] = "renamed_family"
	mapping, err = json.Marshal(registry)
	require.NoError(t, err)
	require.NoError(t, db.UpsertTemplateUsageStats(ctx, mapping))
	repeated, err = db.GetTemplateUsageStats(ctx, params)
	require.NoError(t, err)
	// Only the digest may differ on the main row: the rename changes what the
	// child rows hold and nothing else.
	require.ElementsMatch(t, withoutDigest(rows), withoutDigest(repeated))
	families := sessionUsageMins(ctx, t, sqlDB, "template_usage_stats_session_families", "family", start, user, template)
	require.EqualValues(t, 1, families["renamed_family"])
	require.NotContains(t, families, "new_family")
	apps := sessionUsageMins(ctx, t, sqlDB, "template_usage_stats_session_apps", "app_name", start, user, template)
	require.EqualValues(t, 1, apps["new_app"])
	// The rename shows up in no other column of the main row, so the digest is
	// what marks the bucket changed and lets the child writes reach it.
	require.NotEqual(t, digest, sessionUsageDigest(ctx, t, sqlDB, start, user, template))

	// A bucket that only has app stats records no session usage at all, so it
	// carries the empty digest rather than a null, and a deleted bucket takes
	// its session usage with it.
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
	require.NoError(t, db.UpsertTemplateUsageStats(ctx, mapping))
	require.Equal(t, emptySessionUsageDigest(ctx, t, sqlDB), sessionUsageDigest(ctx, t, sqlDB, start, owner.ID, appTemplate.ID))
	require.Empty(t, sessionUsageMins(ctx, t, sqlDB, "template_usage_stats_session_families", "family", start, owner.ID, appTemplate.ID))
	require.Empty(t, sessionUsageMins(ctx, t, sqlDB, "template_usage_stats_session_apps", "app_name", start, owner.ID, appTemplate.ID))

	require.Empty(t, sessionUsageMins(ctx, t, sqlDB, "template_usage_stats_session_families", "family", start, user, disconnected))
	_, err = sqlDB.ExecContext(ctx, `DELETE FROM template_usage_stats WHERE start_time = $1 AND user_id = $2 AND template_id = $3`, start, user, template)
	require.NoError(t, err)
	require.Empty(t, sessionUsageMins(ctx, t, sqlDB, "template_usage_stats_session_families", "family", start, user, template))
	require.Empty(t, sessionUsageMins(ctx, t, sqlDB, "template_usage_stats_session_apps", "app_name", start, user, template))

	// Live insights deduplicate the overlapping apps without half-hour caps.
	live, err := db.GetTemplateInsightsByTemplate(ctx, database.GetTemplateInsightsByTemplateParams{
		StartTime: start.Add(30 * time.Second), EndTime: start.Add(30 * time.Minute), AppFamilies: mapping,
	})
	require.NoError(t, err)
	require.Len(t, live, 0, "no connected report occurs in this partial request window")
}
