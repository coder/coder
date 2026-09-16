package database_test

import (
	"context"
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
	separate, combined := uuid.New(), uuid.New()
	template := uuid.New()
	stat := func(user uuid.UUID, counts map[string]int64) {
		t.Helper()
		dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			CreatedAt: start, UserID: user, TemplateID: template,
			AgentID: uuid.New(), ConnectionCount: 1,
			SessionCounts: dbgen.SessionCounts(t, counts),
		})
	}

	// Agents report app names, so a name can carry the delimiters the digest
	// uses. Two apps must not encode the same as one app named after them, or
	// a bucket keeps stale child rows.
	stat(separate, map[string]int64{"a": 1, "b": 1})
	stat(combined, map[string]int64{"a:1|1:b": 1})
	require.NoError(t, db.UpsertTemplateUsageStats(ctx))

	require.Equal(t, map[string]int64{"a": 1, "b": 1}, sessionUsageMins(ctx, t, sqlDB, start, separate, template))
	require.Equal(t, map[string]int64{"a:1|1:b": 1}, sessionUsageMins(ctx, t, sqlDB, start, combined, template))
	require.NotEqual(t,
		sessionUsageDigest(ctx, t, sqlDB, start, separate, template),
		sessionUsageDigest(ctx, t, sqlDB, start, combined, template))

	// Repeating the same usage must not rewrite any child row.
	rowVersion := func() string {
		t.Helper()
		var version string
		require.NoError(t, sqlDB.QueryRowContext(ctx,
			`SELECT xmin::text FROM template_usage_stats_session_apps WHERE start_time = $1 AND user_id = $2 AND template_id = $3 AND app_name = $4`,
			start, combined, template, "a:1|1:b").Scan(&version))
		return version
	}
	before, digest := rowVersion(), sessionUsageDigest(ctx, t, sqlDB, start, combined, template)
	require.NoError(t, db.UpsertTemplateUsageStats(ctx))
	require.Equal(t, before, rowVersion())
	require.Equal(t, digest, sessionUsageDigest(ctx, t, sqlDB, start, combined, template))
}
