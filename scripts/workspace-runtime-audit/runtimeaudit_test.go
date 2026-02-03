// Package runtimeaudit_test implements a test runner for the workspace runtime audit SQL script.
// It is intentionally not named with `_test.go` suffix to avoid running `go test` automatically.
//
// This script is not recommend to be used without guidance from the Coder team.
package runtimeaudit_test

import (
	"database/sql"
	_ "embed"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
)

//go:embed workspace-runtime-audit.sql
var runtimeAuditScript string

type auditRow struct {
	WorkspaceID        uuid.UUID
	WorkspaceCreatedAt time.Time
	UsageHours         int
}

func TestRuntimeAudit(t *testing.T) {
	t.Parallel()

	// Cannot run `/copy` meta command over Exec, so comment it out.
	runtimeAuditScript = strings.ReplaceAll(runtimeAuditScript, "\\copy", "-- \\copy")

	// Use the SELECT instead
	runtimeAuditScript = strings.ReplaceAll(runtimeAuditScript, "-- SELECT * FROM _workspace_usage_results", "SELECT * FROM _workspace_usage_results")

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

	// Setup database with some workspace runtimes
	s := initSetup(t, db)
	startPeriod := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	endPeriod := time.Date(2025, 12, 31, 23, 58, 59, 0, time.UTC)

	// Shorthands (keeps vectors readable).
	decUTC := func(d, h, m int) time.Time { return time.Date(2025, 12, d, h, m, 0, 0, time.UTC) }
	novUTC := func(d, h, m int) time.Time { return time.Date(2025, 11, d, h, m, 0, 0, time.UTC) }
	janUTC := func(d, h, m int) time.Time { return time.Date(2026, 1, d, h, m, 0, 0, time.UTC) }

	roundUpHours := func(end, start time.Time) int {
		return int(math.Ceil(end.Sub(start).Hours()))
	}

	type vec struct {
		name   string
		builds []workspaceBuildArgs
		expect func(createdAt time.Time, inputs []workspaceBuildArgs) int
	}

	vectors := []vec{
		// -------------------------
		// Happy path / core logic
		// -------------------------

		{
			name: "long_run_inside_window_start_to_stop_counts_and_rounds",
			// Basic succeeded start -> stop within window.
			builds: []workspaceBuildArgs{
				{at: decUTC(3, 0, 15), transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusSucceeded},
				{at: decUTC(20, 12, 0), transition: database.WorkspaceTransitionStop, jobStatus: database.ProvisionerJobStatusSucceeded},
			},
			expect: func(_ time.Time, in []workspaceBuildArgs) int {
				return roundUpHours(in[1].at, in[0].at)
			},
		},
		{
			name: "failed_start_does_not_count_usage",
			// Only succeeded starts count; failed start means no accumulation.
			builds: []workspaceBuildArgs{
				{at: decUTC(5, 10, 0), transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusFailed},
				{at: decUTC(5, 11, 0), transition: database.WorkspaceTransitionStop, jobStatus: database.ProvisionerJobStatusSucceeded},
			},
			expect: func(_ time.Time, _ []workspaceBuildArgs) int { return 0 },
		},
		{
			name: "multiple_starts_while_running_ignores_later_start_uses_first_start",
			// Second succeeded start is ignored while already "on".
			builds: []workspaceBuildArgs{
				{at: decUTC(6, 10, 0), transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusSucceeded},
				{at: decUTC(6, 10, 5), transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusSucceeded},
				{at: decUTC(6, 10, 30), transition: database.WorkspaceTransitionStop, jobStatus: database.ProvisionerJobStatusSucceeded},
			},
			expect: func(_ time.Time, in []workspaceBuildArgs) int {
				return roundUpHours(in[2].at, in[0].at)
			},
		},
		{
			name: "delete_transition_treated_as_stop",
			// Non-(start+succeeded) transitions behave like stop when running.
			builds: []workspaceBuildArgs{
				{at: decUTC(7, 9, 0), transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusSucceeded},
				{at: decUTC(7, 9, 20), transition: database.WorkspaceTransitionDelete, jobStatus: database.ProvisionerJobStatusSucceeded},
			},
			expect: func(_ time.Time, in []workspaceBuildArgs) int {
				return roundUpHours(in[1].at, in[0].at)
			},
		},

		// -------------------------
		// Window clipping
		// -------------------------

		{
			name: "started_before_window_clips_start_to_window_start",
			// Start before period; only count from startPeriod.
			builds: []workspaceBuildArgs{
				{at: novUTC(27, 23, 0), transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusSucceeded},
				{at: decUTC(1, 1, 0), transition: database.WorkspaceTransitionStop, jobStatus: database.ProvisionerJobStatusSucceeded},
			},
			expect: func(_ time.Time, in []workspaceBuildArgs) int {
				return roundUpHours(in[1].at, startPeriod)
			},
		},
		{
			name: "stopped_after_window_clips_stop_to_window_end",
			// Stop after period; only count until endPeriod.
			builds: []workspaceBuildArgs{
				{at: decUTC(31, 23, 30), transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusSucceeded},
				{at: janUTC(1, 1, 0), transition: database.WorkspaceTransitionStop, jobStatus: database.ProvisionerJobStatusSucceeded},
			},
			expect: func(_ time.Time, in []workspaceBuildArgs) int {
				return roundUpHours(endPeriod, in[0].at)
			},
		},
		{
			name: "window_clips_both_start_and_stop",
			// Stop after period; only count until endPeriod.
			builds: []workspaceBuildArgs{
				{at: novUTC(27, 23, 30), transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusSucceeded},
				{at: janUTC(1, 8, 0), transition: database.WorkspaceTransitionStop, jobStatus: database.ProvisionerJobStatusSucceeded},
			},
			expect: func(_ time.Time, _ []workspaceBuildArgs) int {
				return roundUpHours(endPeriod, startPeriod)
			},
		},
		{
			name: "stop_exactly_at_window_start_counts_zero_due_to_strict_gt",
			// Script uses `turned_off > start_time`, so equality yields 0.
			builds: []workspaceBuildArgs{
				{at: novUTC(30, 23, 0), transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusSucceeded},
				{at: startPeriod, transition: database.WorkspaceTransitionStop, jobStatus: database.ProvisionerJobStatusSucceeded},
			},
			expect: func(_ time.Time, _ []workspaceBuildArgs) int { return 0 },
		},

		// -------------------------
		// Still running at end
		// -------------------------

		{
			name: "started_in_window_no_stop_accumulates_until_window_end",
			// Tail case: still on -> add (endPeriod - turned_on).
			builds: []workspaceBuildArgs{
				{at: decUTC(10, 8, 0), transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusSucceeded},
			},
			expect: func(_ time.Time, _ []workspaceBuildArgs) int {
				return roundUpHours(endPeriod, decUTC(10, 8, 0))
			},
		},
		{
			name: "started_before_window_no_stop_accumulates_full_window",
			// Tail case + clipping: start before period -> treat as startPeriod.
			builds: []workspaceBuildArgs{
				{at: novUTC(1, 0, 0), transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusSucceeded},
			},
			expect: func(_ time.Time, _ []workspaceBuildArgs) int {
				return roundUpHours(endPeriod, startPeriod)
			},
		},

		// -------------------------
		// Multi-segment behavior (single ceil at end)
		// -------------------------

		{
			name: "two_segments_sum_then_single_round_up",
			// 20m + 20m => 40m total => ceil(total)=1 hour.
			builds: []workspaceBuildArgs{
				{at: decUTC(12, 10, 0), transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusSucceeded},
				{at: decUTC(12, 10, 20), transition: database.WorkspaceTransitionStop, jobStatus: database.ProvisionerJobStatusSucceeded},
				{at: decUTC(12, 11, 0), transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusSucceeded},
				{at: decUTC(12, 11, 20), transition: database.WorkspaceTransitionStop, jobStatus: database.ProvisionerJobStatusSucceeded},
			},
			expect: func(_ time.Time, _ []workspaceBuildArgs) int {
				return 1
			},
		},
		{
			name: "two_segments_sum",
			// 20m + 20m => 40m total => ceil(total)=1 hour.
			builds: []workspaceBuildArgs{
				{at: decUTC(12, 10, 0), transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusSucceeded},
				{at: decUTC(12, 15, 20), transition: database.WorkspaceTransitionStop, jobStatus: database.ProvisionerJobStatusSucceeded},

				{at: decUTC(13, 11, 0), transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusSucceeded},
				{at: decUTC(12, 13, 14), transition: database.WorkspaceTransitionStop, jobStatus: database.ProvisionerJobStatusSucceeded},

				{at: decUTC(14, 7, 0), transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusSucceeded},
				{at: decUTC(14, 13, 14), transition: database.WorkspaceTransitionStop, jobStatus: database.ProvisionerJobStatusSucceeded},

				{at: decUTC(15, 4, 0), transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusSucceeded},
				{at: decUTC(15, 8, 14), transition: database.WorkspaceTransitionStop, jobStatus: database.ProvisionerJobStatusSucceeded},

				// Add a failed start/stop to ensure it's ignored.
				{at: decUTC(16, 0, 0), transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusFailed},
				{at: decUTC(17, 0, 0), transition: database.WorkspaceTransitionStop, jobStatus: database.ProvisionerJobStatusSucceeded},
			},
			expect: func(_ time.Time, in []workspaceBuildArgs) int {
				return roundUpHours(in[1].at, in[0].at) +
					roundUpHours(in[3].at, in[2].at) +
					roundUpHours(in[5].at, in[4].at) +
					roundUpHours(in[7].at, in[6].at)
			},
		},

		// -------------------------
		// Outside-the-window activity
		// -------------------------

		{
			name: "activity_entirely_before_window_counts_zero",
			// Stop before startPeriod => no overlap.
			builds: []workspaceBuildArgs{
				{at: novUTC(10, 10, 0), transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusSucceeded},
				{at: novUTC(10, 10, 10), transition: database.WorkspaceTransitionStop, jobStatus: database.ProvisionerJobStatusSucceeded},
			},
			expect: func(_ time.Time, _ []workspaceBuildArgs) int { return 0 },
		},
		{
			name: "activity_entirely_after_window_counts_zero",
			// Start after endPeriod => no overlap.
			builds: []workspaceBuildArgs{
				{at: janUTC(2, 10, 0), transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusSucceeded},
				{at: janUTC(2, 10, 10), transition: database.WorkspaceTransitionStop, jobStatus: database.ProvisionerJobStatusSucceeded},
			},
			expect: func(_ time.Time, _ []workspaceBuildArgs) int { return 0 },
		},

		// -------------------------
		// Canceled / failed builds
		// -------------------------

		{
			name: "canceled_start_does_not_count_usage",
			// Only start+succeeded counts; canceled start is ignored.
			builds: []workspaceBuildArgs{
				{at: decUTC(8, 9, 0), canceled: true, transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusCanceled},
				{at: decUTC(8, 10, 0), canceled: false, transition: database.WorkspaceTransitionStop, jobStatus: database.ProvisionerJobStatusSucceeded},
			},
			expect: func(_ time.Time, _ []workspaceBuildArgs) int { return 0 },
		},
		{
			name: "failed_start_does_not_count_even_if_later_stop_occurs",
			// Start failed => never turns on => later stop does nothing.
			builds: []workspaceBuildArgs{
				{at: decUTC(9, 9, 0), canceled: false, transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusFailed},
				{at: decUTC(9, 12, 0), canceled: false, transition: database.WorkspaceTransitionStop, jobStatus: database.ProvisionerJobStatusSucceeded},
			},
			expect: func(_ time.Time, _ []workspaceBuildArgs) int { return 0 },
		},
		{
			name: "canceled_stop_still_stops_timer_and_counts_time",
			// Any non-(start+succeeded) is treated as stop while running, regardless of status/canceled.
			builds: []workspaceBuildArgs{
				{at: decUTC(10, 9, 0), canceled: false, transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusSucceeded},
				{at: decUTC(10, 9, 40), canceled: true, transition: database.WorkspaceTransitionStop, jobStatus: database.ProvisionerJobStatusCanceled},
			},
			expect: func(_ time.Time, in []workspaceBuildArgs) int {
				return roundUpHours(in[1].at, in[0].at)
			},
		},
		{
			name: "failed_stop_still_stops_timer_and_counts_time",
			// Same as above: stop is stop even if job failed (ELSE path).
			builds: []workspaceBuildArgs{
				{at: decUTC(11, 10, 0), canceled: false, transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusSucceeded},
				{at: decUTC(11, 10, 10), canceled: false, transition: database.WorkspaceTransitionStop, jobStatus: database.ProvisionerJobStatusFailed},
			},
			expect: func(_ time.Time, in []workspaceBuildArgs) int {
				return roundUpHours(in[1].at, in[0].at)
			},
		},
		{
			name: "failed_transition_stops_timer_and_counts_time",
			// A failed *non-stop* transition (e.g. delete) still stops if currently on.
			builds: []workspaceBuildArgs{
				{at: decUTC(12, 8, 0), canceled: false, transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusSucceeded},
				{at: decUTC(12, 8, 5), canceled: false, transition: database.WorkspaceTransitionDelete, jobStatus: database.ProvisionerJobStatusFailed},
			},
			expect: func(_ time.Time, in []workspaceBuildArgs) int {
				return roundUpHours(in[1].at, in[0].at)
			},
		},
		{
			name: "start_succeeded_then_start_failed_does_not_reset_start_time",
			// When already on, a subsequent non-(start+succeeded) build triggers stop logic.
			// This verifies you *do not* treat start+failed as a "start"; it will stop the running timer.
			builds: []workspaceBuildArgs{
				{at: decUTC(13, 9, 0), canceled: false, transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusSucceeded},
				// This goes to ELSE branch (because job_status != succeeded) and will stop the timer.
				{at: decUTC(13, 9, 30), canceled: false, transition: database.WorkspaceTransitionStart, jobStatus: database.ProvisionerJobStatusFailed},
				// Subsequent stop should not add more time because timer was reset.
				{at: decUTC(13, 10, 0), canceled: false, transition: database.WorkspaceTransitionStop, jobStatus: database.ProvisionerJobStatusSucceeded},
			},
			expect: func(_ time.Time, in []workspaceBuildArgs) int {
				// Only counts from first start to failed-start event.
				return roundUpHours(in[1].at, in[0].at)
			},
		},
	}

	// Create all workspaces
	workspaces := make([]database.WorkspaceTable, len(vectors))

	for i, v := range vectors {
		wrk := s.createWorkspace(t, db, v.builds)
		workspaces[i] = wrk
	}

	row, err := sqlDB.Query(runtimeAuditScript)
	require.NoError(t, err)

	found := make(map[uuid.UUID]auditRow)
	for row.Next() {
		var r auditRow
		err = row.Scan(&r.WorkspaceID, &r.WorkspaceCreatedAt, &r.UsageHours)
		require.NoError(t, err)
		found[r.WorkspaceID] = r
	}

	for i, v := range vectors {
		t.Run(v.name, func(t *testing.T) {
			t.Parallel()
			v.expect(workspaces[i].CreatedAt, v.builds)
		})
	}
}

type setup struct {
	org database.Organization
	usr database.User
}

func initSetup(t *testing.T, db database.Store) *setup {
	usr := dbgen.User(t, db, database.User{})
	org := dbfake.Organization(t, db).
		Members(usr).
		Do()
	return &setup{
		org: org.Org,
		usr: usr,
	}
}

type workspaceBuildArgs struct {
	at         time.Time
	canceled   bool
	transition database.WorkspaceTransition
	jobStatus  database.ProvisionerJobStatus
}

func (s *setup) createWorkspace(t *testing.T, db database.Store, builds []workspaceBuildArgs) database.WorkspaceTable {
	// Insert the first build
	tv := dbfake.TemplateVersion(t, db).
		Seed(database.TemplateVersion{
			OrganizationID: s.org.ID,
			CreatedBy:      s.usr.ID,
		}).
		Do()

	wrk := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        s.usr.ID,
		OrganizationID: s.org.ID,
		TemplateID:     tv.Template.ID,
		CreatedAt:      builds[0].at,
	})

	for i, b := range builds {
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			CreatedAt: b.at,
			UpdatedAt: b.at,
			StartedAt: sql.NullTime{
				Time:  b.at,
				Valid: true,
			},
			CanceledAt: sql.NullTime{
				Time:  b.at,
				Valid: b.canceled,
			},
			CompletedAt: sql.NullTime{
				Time:  b.at,
				Valid: true,
			},
			Error:          sql.NullString{},
			OrganizationID: s.org.ID,
			InitiatorID:    s.usr.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			JobStatus:      b.jobStatus,
		})

		dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			CreatedAt:         b.at,
			UpdatedAt:         b.at,
			WorkspaceID:       wrk.ID,
			TemplateVersionID: tv.TemplateVersion.ID,
			///nolint:gosec // this will not overflow
			BuildNumber: int32(i) + 1,
			Transition:  b.transition,
			InitiatorID: s.usr.ID,
			JobID:       job.ID,
		})
	}

	return wrk
}
