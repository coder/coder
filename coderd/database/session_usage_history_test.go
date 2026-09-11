package database_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/migrations"
)

func TestSessionUsageHistoryCapsAndNamespaces(t *testing.T) {
	t.Parallel()
	sqlDB := testSQLDB(t)
	require.NoError(t, migrations.Up(sqlDB))
	db := database.New(sqlDB)
	ctx := context.Background()
	start := dbtime.Now().Add(-2 * time.Hour).Truncate(30 * time.Minute)
	user, template1, template2, appTemplate := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	// The same user in two templates in one half hour, so the family minutes
	// have to be capped at 30 across templates. There is no sqlc query for the
	// child tables, so the history is seeded directly.
	for _, template := range []uuid.UUID{template1, template2} {
		_, err := sqlDB.ExecContext(ctx, `INSERT INTO template_usage_stats(start_time,end_time,user_id,template_id,usage_mins) VALUES($1,$2,$3,$4,20)`,
			start, start.Add(30*time.Minute), user, template)
		require.NoError(t, err)
		_, err = sqlDB.ExecContext(ctx, `INSERT INTO template_usage_stats_session_families(start_time,template_id,user_id,family,usage_mins) VALUES($1,$2,$3,'new_family',20),($1,$2,$3,'ssh',20),($1,$2,$3,'sftp',2)`,
			start, template, user)
		require.NoError(t, err)
	}
	// A bucket produced by app stats alone: it has no session usage, so it has
	// no child rows.
	_, err := sqlDB.ExecContext(ctx, `INSERT INTO template_usage_stats(start_time,end_time,user_id,template_id,usage_mins,app_usage_mins) VALUES($1,$2,$3,$4,5,'{"ssh":5}')`,
		start, start.Add(30*time.Minute), uuid.New(), appTemplate)
	require.NoError(t, err)
	usage, err := db.GetTemplateInsights(ctx, database.GetTemplateInsightsParams{StartTime: start, EndTime: start.Add(30 * time.Minute)})
	require.NoError(t, err)
	require.EqualValues(t, 2, usage.ActiveUsers)
	require.EqualValues(t, 35*60, usage.UsageTotalSeconds)
	require.ElementsMatch(t, []uuid.UUID{template1, template2, appTemplate}, usage.TemplateIDs)
	require.JSONEq(t, `{"new_family":1800,"ssh":1800,"sftp":240}`, string(usage.SessionFamilyUsageSeconds))
	var ids map[string][]uuid.UUID
	require.NoError(t, json.Unmarshal(usage.SessionFamilyTemplateIds, &ids))
	require.ElementsMatch(t, []uuid.UUID{template1, template2}, ids["new_family"])
	require.ElementsMatch(t, []uuid.UUID{template1, template2}, ids["ssh"])
	onlyApp, err := db.GetTemplateInsights(ctx, database.GetTemplateInsightsParams{StartTime: start, EndTime: start.Add(30 * time.Minute), TemplateIDs: []uuid.UUID{appTemplate}})
	require.NoError(t, err)
	require.EqualValues(t, 1, onlyApp.ActiveUsers)
	require.EqualValues(t, 300, onlyApp.UsageTotalSeconds)
	require.JSONEq(t, `{}`, string(onlyApp.SessionFamilyUsageSeconds))
	require.JSONEq(t, `{}`, string(onlyApp.SessionFamilyTemplateIds))
	// Filtering to one of the two templates leaves a single-template user, so
	// the capped path is not taken and the minutes are that template's alone.
	onlyOne, err := db.GetTemplateInsights(ctx, database.GetTemplateInsightsParams{StartTime: start, EndTime: start.Add(30 * time.Minute), TemplateIDs: []uuid.UUID{template1}})
	require.NoError(t, err)
	require.JSONEq(t, `{"new_family":1200,"ssh":1200,"sftp":120}`, string(onlyOne.SessionFamilyUsageSeconds))
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
	require.JSONEq(t, `{}`, string(row.SessionFamilyUsageSeconds))
	require.JSONEq(t, `{}`, string(row.SessionFamilyTemplateIds))
}
