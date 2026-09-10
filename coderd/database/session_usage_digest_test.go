package database_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

func TestSessionUsageDigestDistinguishesDelimitedNames(t *testing.T) {
	t.Parallel()

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ctx := context.Background()
	start := dbtime.Now().Add(-time.Hour).Truncate(30 * time.Minute)
	user, template := uuid.New(), uuid.New()
	dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
		CreatedAt: start, UserID: user, TemplateID: template,
		AgentID: uuid.New(), ConnectionCount: 1,
		SessionCounts: dbgen.SessionCounts(t, map[string]int64{"app_a": 1, "app_b": 1}),
	})

	rollup := func(registry map[string]string) {
		t.Helper()
		mapping, err := json.Marshal(registry)
		require.NoError(t, err)
		require.NoError(t, db.UpsertTemplateUsageStats(ctx, mapping))
	}
	families := func() map[string]int64 {
		return sessionUsageMins(ctx, t, sqlDB, "template_usage_stats_session_families", "family", start, user, template)
	}

	rollup(map[string]string{"app_a": "a", "app_b": "b"})
	require.Equal(t, map[string]int64{"a": 1, "b": 1}, families())
	before := sessionUsageDigest(ctx, t, sqlDB, start, user, template)

	// A registry change can merge two families without changing app usage or
	// the main bucket totals. Delimiters in a family name must not make its
	// digest identical to the two separate family rows it replaces.
	registry := map[string]string{"app_a": "a:1|1:b", "app_b": "a:1|1:b"}
	rollup(registry)
	require.Equal(t, map[string]int64{"a:1|1:b": 1}, families())
	after := sessionUsageDigest(ctx, t, sqlDB, start, user, template)
	require.NotEqual(t, before, after)
	require.Equal(t, map[string]int64{"app_a": 1, "app_b": 1},
		sessionUsageMins(ctx, t, sqlDB, "template_usage_stats_session_apps", "app_name", start, user, template))

	// Repeating the same attribution must not rewrite any child row.
	var rowVersion string
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT xmin::text FROM template_usage_stats_session_families WHERE start_time = $1 AND user_id = $2 AND template_id = $3`,
		start, user, template).Scan(&rowVersion))
	rollup(registry)
	var repeatedVersion string
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT xmin::text FROM template_usage_stats_session_families WHERE start_time = $1 AND user_id = $2 AND template_id = $3`,
		start, user, template).Scan(&repeatedVersion))
	require.Equal(t, rowVersion, repeatedVersion)
	require.Equal(t, after, sessionUsageDigest(ctx, t, sqlDB, start, user, template))
}
