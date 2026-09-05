package coderd_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
)

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
