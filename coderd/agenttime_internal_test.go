package coderd

import (
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
)

func TestAgentTimeQuery(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 15, 13, 0, 0, 0, time.UTC)
	org := uuid.New()
	t.Run("DefaultsAndScope", func(t *testing.T) {
		t.Parallel()
		q, errors := parseAgentTimeQuery(url.Values{}, org.String(), now)
		require.Empty(t, errors)
		require.True(t, q.start.IsZero())
		require.Equal(t, "2026-03-16", q.end.Format(time.DateOnly))
		require.Equal(t, org, q.organizationID)
		require.Equal(t, int32(25), q.limit)
	})
	for _, query := range []string{
		"start_date=2026-02-30", "start_date=2026-03-10T00:00:00Z", "start_date=0000-01-01",
		"start_date=2026-03-15&end_date=2026-03-15", "start_date=2026-03-16&end_date=2026-03-15",
		"end_date=2026-03-17", "interval=hour", "group_by=group", "sort_by=runtime", "sort_order=bad",
		"limit=0", "limit=101", "limit=bad", "offset=-1", "offset=2147483648", "offset=bad",
		"organization_id=invalid", "user_id=invalid", "organization_id=" + uuid.Nil.String(), "user_id=" + uuid.Nil.String(),
		"organization_id=" + uuid.NewString(), "unrecognized=true", "interval=day&interval=week",
	} {
		t.Run(query, func(t *testing.T) {
			t.Parallel()
			values, err := url.ParseQuery(query)
			require.NoError(t, err)
			_, errors := parseAgentTimeQuery(values, org.String(), now)
			require.NotEmpty(t, errors)
		})
	}
	for _, path := range []string{"invalid", uuid.Nil.String()} {
		t.Run("InvalidScope/"+path, func(t *testing.T) {
			t.Parallel()
			_, errors := parseAgentTimeQuery(url.Values{}, path, now)
			require.NotEmpty(t, errors)
		})
	}
}

func TestAgentTimeCalendarBuckets(t *testing.T) {
	t.Parallel()
	parse := func(value string) time.Time {
		date, err := time.Parse(time.DateOnly, value)
		require.NoError(t, err)
		return date
	}
	for _, tt := range []struct {
		name, start, end string
		interval         codersdk.AgentTimeInterval
		starts, ends     []string
		partial          []bool
	}{
		{"LeapDays", "2024-02-28", "2024-03-02", codersdk.AgentTimeIntervalDay, []string{"2024-02-28", "2024-02-29", "2024-03-01"}, []string{"2024-02-29", "2024-03-01", "2024-03-02"}, []bool{false, false, false}},
		{"MondayWeeks", "2024-12-29", "2025-01-14", codersdk.AgentTimeIntervalWeek, []string{"2024-12-29", "2024-12-30", "2025-01-06", "2025-01-13"}, []string{"2024-12-30", "2025-01-06", "2025-01-13", "2025-01-14"}, []bool{true, false, false, true}},
		{"CalendarMonths", "2024-01-31", "2024-04-01", codersdk.AgentTimeIntervalMonth, []string{"2024-01-31", "2024-02-01", "2024-03-01"}, []string{"2024-02-01", "2024-03-01", "2024-04-01"}, []bool{true, false, false}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			buckets, err := agentTimeBuckets(parse(tt.start), parse(tt.end), tt.interval, parse("2026-01-01"))
			require.NoError(t, err)
			require.Len(t, buckets, len(tt.starts))
			for i, bucket := range buckets {
				require.Equal(t, tt.starts[i], bucket.StartDate)
				require.Equal(t, tt.ends[i], bucket.EndDate)
				require.Equal(t, tt.partial[i], bucket.Partial)
				require.Nil(t, bucket.AgentTimeMS, "coverage must be established before zero filling")
				require.False(t, bucket.Complete)
			}
		})
	}
	t.Run("CurrentDay", func(t *testing.T) {
		t.Parallel()
		buckets, err := agentTimeBuckets(parse("2026-01-01"), parse("2026-01-02"), codersdk.AgentTimeIntervalDay, parse("2026-01-01").Add(12*time.Hour))
		require.NoError(t, err)
		require.True(t, buckets[0].Partial)
	})
	t.Run("PointBudgetNotRetentionLimit", func(t *testing.T) {
		t.Parallel()
		start, end := parse("2000-01-01"), parse("2026-01-01")
		_, err := agentTimeBuckets(start, end, codersdk.AgentTimeIntervalDay, end)
		require.ErrorContains(t, err, "1000 chart points")
		buckets, err := agentTimeBuckets(start, end, codersdk.AgentTimeIntervalMonth, end)
		require.NoError(t, err)
		require.Len(t, buckets, 26*12)
		buckets, err = agentTimeBuckets(start, start.AddDate(0, 0, 1000), codersdk.AgentTimeIntervalDay, end)
		require.NoError(t, err)
		require.Len(t, buckets, 1000)
	})
}

func TestAgentTimeCoverage(t *testing.T) {
	t.Parallel()
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	org, user := uuid.New(), uuid.New()
	_, err := sqlDB.ExecContext(t.Context(), `
 WITH capture AS (UPDATE agent_time_capture SET capture_started_at='2025-01-01 12:00:00+00' RETURNING id)
 INSERT INTO agent_time_backfill_status (organization_id,completed_at,processed_messages)
 VALUES ($1,'2025-01-02 00:00:00+00',42);
 `, org)
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(t.Context(), "INSERT INTO agent_time_daily (organization_id,user_id,day,agent_time_ms) VALUES ($1,$2,'2024-12-31',7)", org, user)
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(t.Context(), "INSERT INTO agent_time_organization_daily (organization_id,day,agent_time_ms) SELECT organization_id,day,SUM(agent_time_ms) FROM agent_time_daily GROUP BY 1,2 ON CONFLICT (organization_id,day) DO UPDATE SET agent_time_ms=EXCLUDED.agent_time_ms")
	require.NoError(t, err)
	q, validations := parseAgentTimeQuery(url.Values{"start_date": {"2024-12-31"}, "end_date": {"2025-01-04"}}, org.String(), time.Date(2025, 1, 3, 12, 0, 0, 0, time.UTC))
	require.Empty(t, validations)
	report, err := readAgentTimeReport(t.Context(), db, q, time.Date(2025, 1, 3, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.True(t, report.Status.BackfillComplete)
	require.Equal(t, "42", report.Status.ProcessedMessages)
	require.Equal(t, "7", *report.Buckets[0].AgentTimeMS)
	require.False(t, report.Buckets[0].Complete)
	require.Nil(t, report.Buckets[1].AgentTimeMS, "capture began mid-day")
	require.Equal(t, "0", *report.Buckets[2].AgentTimeMS)
	require.True(t, report.Buckets[2].Complete)
	require.False(t, report.Buckets[2].Partial)
	require.True(t, report.Buckets[3].Partial)
	_, err = sqlDB.ExecContext(t.Context(), "UPDATE agent_time_backfill_status SET completed_at=NULL,last_error='backfill failed' WHERE organization_id=$1", org)
	require.NoError(t, err)
	incomplete, err := readAgentTimeReport(t.Context(), db, q, time.Date(2025, 1, 3, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.False(t, incomplete.Status.BackfillComplete)
	require.Equal(t, "backfill failed", incomplete.Status.BackfillError)
	require.Nil(t, incomplete.Buckets[2].AgentTimeMS)
}

func TestAgentTimeReportPrecision(t *testing.T) {
	t.Parallel()
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	org := uuid.New()
	_, err := sqlDB.ExecContext(t.Context(), `
 INSERT INTO agent_time_daily (organization_id,user_id,day,agent_time_ms)
 VALUES ($1,$2,'2025-01-01',9223372036854775807),($1,$3,'2025-01-01',9223372036854775807)
 `, org, uuid.New(), uuid.New())
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(t.Context(), "INSERT INTO agent_time_organization_daily (organization_id,day,agent_time_ms) SELECT organization_id,day,SUM(agent_time_ms) FROM agent_time_daily GROUP BY 1,2 ON CONFLICT (organization_id,day) DO UPDATE SET agent_time_ms=EXCLUDED.agent_time_ms")
	require.NoError(t, err)
	now := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	for _, group := range []string{"organization", "user"} {
		q, validations := parseAgentTimeQuery(url.Values{"start_date": {"2025-01-01"}, "end_date": {"2025-01-02"}, "group_by": {group}}, org.String(), now)
		require.Empty(t, validations)
		report, err := readAgentTimeReport(t.Context(), db, q, now)
		require.NoError(t, err)
		require.Equal(t, "18446744073709551614", report.TotalAgentTimeMS)
		require.Equal(t, "18446744073709551614", *report.Buckets[0].AgentTimeMS)
	}
}
