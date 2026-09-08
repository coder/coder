package coderd_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

func TestAgentTimeReport(t *testing.T) {
	t.Parallel()
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	// Collection does not depend on enterprise licensing or usage statistics.
	client := coderdtest.New(t, &coderdtest.Options{
		Database: db, Pubsub: ps,
		DeploymentValues: coderdtest.DeploymentValues(t, func(dv *codersdk.DeploymentValues) {
			dv.StatsCollection.UsageStats.Enable = false
		}),
	})
	owner := coderdtest.CreateFirstUser(t, client)
	memberClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	orgAdminClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))
	otherOrg := dbgen.Organization(t, db, database.Organization{})
	deletedOrg, deletedUser := uuid.New(), uuid.New()
	for _, row := range []struct {
		org, user uuid.UUID
		day       string
		ms        int64
	}{
		{owner.OrganizationID, owner.UserID, "2025-01-05", 9},
		{owner.OrganizationID, owner.UserID, "2025-01-06", 100},
		{owner.OrganizationID, deletedUser, "2025-01-07", 50},
		{otherOrg.ID, owner.UserID, "2025-01-07", 20},
		{deletedOrg, deletedUser, "2025-01-08", 30},
		{owner.OrganizationID, owner.UserID, "2025-02-01", 999},
	} {
		_, err := sqlDB.ExecContext(t.Context(), "INSERT INTO agent_time_daily (organization_id,user_id,day,agent_time_ms) VALUES ($1,$2,$3,$4)", row.org, row.user, row.day, row.ms)
		require.NoError(t, err)
	}
	_, err := sqlDB.ExecContext(t.Context(), "INSERT INTO agent_time_organization_daily (organization_id,day,agent_time_ms) SELECT organization_id,day,SUM(agent_time_ms) FROM agent_time_daily GROUP BY 1,2 ON CONFLICT (organization_id,day) DO UPDATE SET agent_time_ms=EXCLUDED.agent_time_ms")
	require.NoError(t, err)
	req := codersdk.AgentTimeRequest{StartDate: "2025-01-01", EndDate: "2025-02-01", Interval: codersdk.AgentTimeIntervalWeek, Limit: 1}
	report, err := client.AgentTime(t.Context(), req)
	require.NoError(t, err)
	require.Equal(t, "209", report.TotalAgentTimeMS)
	require.EqualValues(t, 3, report.Count)
	require.Len(t, report.Rows, 1)
	require.Equal(t, owner.OrganizationID.String(), report.Rows[0].ID)
	require.Equal(t, "159", report.Rows[0].AgentTimeMS)
	require.Len(t, report.Buckets, 5)
	require.Equal(t, "9", *report.Buckets[0].AgentTimeMS)
	require.Equal(t, "200", *report.Buckets[1].AgentTimeMS)
	require.Nil(t, report.Buckets[2].AgentTimeMS, "unknown historical days must not be shown as zero")
	require.False(t, report.Buckets[1].Complete)
	require.Contains(t, report.HistoricalNotice, "cannot be recovered")
	require.Equal(t, "2025-01-05", *report.Status.EarliestDate)

	req.Offset = 10
	page, err := client.AgentTime(t.Context(), req)
	require.NoError(t, err)
	require.Empty(t, page.Rows)
	require.Equal(t, report.TotalAgentTimeMS, page.TotalAgentTimeMS)
	require.Equal(t, report.Count, page.Count)
	require.Equal(t, report.Buckets, page.Buckets)

	req.Offset = 0
	req.Limit = 100
	req.GroupBy = "user"
	scoped, err := client.OrganizationAgentTime(t.Context(), owner.OrganizationID.String(), req)
	require.NoError(t, err)
	require.Equal(t, "159", scoped.TotalAgentTimeMS)
	require.EqualValues(t, 2, scoped.Count)
	require.Equal(t, owner.UserID.String(), scoped.Rows[0].ID)
	require.Equal(t, "109", scoped.Rows[0].AgentTimeMS)
	require.Equal(t, "Deleted user", scoped.Rows[1].Name)
	require.True(t, scoped.Rows[1].Deleted)
	req.UserID = deletedUser.String()
	filtered, err := client.OrganizationAgentTime(t.Context(), owner.OrganizationID.String(), req)
	require.NoError(t, err)
	require.Equal(t, "50", filtered.TotalAgentTimeMS)
	require.Len(t, filtered.Rows, 1)
	req.UserID = ""
	req.GroupBy = "organization"
	deleted, err := client.OrganizationAgentTime(t.Context(), deletedOrg.String(), req)
	require.NoError(t, err)
	require.Equal(t, "30", deleted.TotalAgentTimeMS)
	require.Equal(t, "Deleted organization", deleted.Rows[0].Name)
	require.True(t, deleted.Rows[0].Deleted)

	req.OrganizationID = otherOrg.ID.String()
	_, err = client.OrganizationAgentTime(t.Context(), owner.OrganizationID.String(), req)
	require.Error(t, err)
	var apiError *codersdk.Error
	require.ErrorAs(t, err, &apiError)
	require.Equal(t, http.StatusBadRequest, apiError.StatusCode())
	req.OrganizationID = ""
	for _, unauthorized := range []*codersdk.Client{memberClient, orgAdminClient} {
		_, err = unauthorized.AgentTime(t.Context(), req)
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusForbidden, apiError.StatusCode())
		_, err = unauthorized.OrganizationAgentTime(t.Context(), owner.OrganizationID.String(), req)
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusForbidden, apiError.StatusCode())
	}

	req.SortBy = "agent_time"
	req.SortOrder = "asc"
	sorted, err := client.AgentTime(t.Context(), req)
	require.NoError(t, err)
	require.Equal(t, []string{"20", "30", "159"}, []string{sorted.Rows[0].AgentTimeMS, sorted.Rows[1].AgentTimeMS, sorted.Rows[2].AgentTimeMS})
	req.StartDate = ""
	req.EndDate = "2025-01-09"
	all, err := client.AgentTime(t.Context(), req)
	require.NoError(t, err)
	require.Equal(t, "2025-01-05", all.StartDate)
	require.Equal(t, "209", all.TotalAgentTimeMS)
	req.StartDate = "2025-03-01"
	req.EndDate = "2025-03-02"
	empty, err := client.AgentTime(t.Context(), req)
	require.NoError(t, err)
	require.Equal(t, "0", empty.TotalAgentTimeMS)
	require.Zero(t, empty.Count)
	require.Empty(t, empty.Rows)
}

// BenchmarkAgentTimeReports uses 3.65 million daily rows: 100 organizations,
// 10,000 users, and one recorded day per user every ten days over ten years.
// Message volume does not affect report cost: no report query reads messages.
// The interactive budget is 500ms per database report (summary, chart, page).
// PostgreSQL 13.21 EXPLAIN ANALYZE on temporary fixtures, median of three runs:
// canonical month/year/ten-years: 19.6/399.7/3563.6ms; organization summaries:
// 0.63/4.93/45.0ms. These measurements exclude network and HTTP overhead.
func BenchmarkAgentTimeReports(b *testing.B) {
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(b)
	_, err := sqlDB.ExecContext(b.Context(), `
 INSERT INTO agent_time_daily (organization_id,user_id,day,agent_time_ms)
 SELECT md5((u % 100)::text)::uuid, md5(u::text)::uuid,
   DATE '2016-01-01' + d * 10, 3600000
 FROM generate_series(1,10000) u CROSS JOIN generate_series(0,364) d;
 INSERT INTO agent_time_organization_daily (organization_id,day,agent_time_ms) SELECT organization_id,day,SUM(agent_time_ms) FROM agent_time_daily GROUP BY 1,2 ON CONFLICT (organization_id,day) DO UPDATE SET agent_time_ms=EXCLUDED.agent_time_ms;
 ANALYZE agent_time_daily;
 ANALYZE agent_time_organization_daily;`)
	require.NoError(b, err)
	end := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, summaries := range []bool{false, true} {
		for _, years := range []int{0, 1, 10} {
			name, start, interval := "month", end.AddDate(0, -1, 0), "day"
			if years > 0 {
				name = fmt.Sprintf("%d-years", years)
				start = end.AddDate(-years, 0, 0)
				interval = "month"
			}
			b.Run(fmt.Sprintf("summary=%t/%s", summaries, name), func(b *testing.B) {
				for b.Loop() {
					_, err := db.GetAgentTimeSummary(b.Context(), database.GetAgentTimeSummaryParams{StartDate: start, EndDate: end, GroupBy: "organization", UseOrganizationSummary: summaries})
					require.NoError(b, err)
					_, err = db.GetAgentTimeBuckets(b.Context(), database.GetAgentTimeBucketsParams{StartDate: start, EndDate: end, Interval: interval, UseOrganizationSummary: summaries})
					require.NoError(b, err)
					_, err = db.GetAgentTimeBreakdown(b.Context(), database.GetAgentTimeBreakdownParams{StartDate: start, EndDate: end, GroupBy: "organization", PageLimit: 25, SortBy: "agent_time", SortOrder: "desc", UseOrganizationSummary: summaries})
					require.NoError(b, err)
				}
			})
		}
	}
}
