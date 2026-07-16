package autobuild

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/schedule"
)

func Test_getNextTransition_TaskAutoPause(t *testing.T) {
	t.Parallel()

	// Set up a workspace that is eligible for autostop (past deadline).
	now := time.Now()
	pastDeadline := now.Add(-time.Hour)

	okUser := database.User{Status: database.UserStatusActive}
	okBuild := database.WorkspaceBuild{
		Transition: database.WorkspaceTransitionStart,
		Deadline:   pastDeadline,
	}
	okJob := database.ProvisionerJob{
		JobStatus: database.ProvisionerJobStatusSucceeded,
	}
	okTemplateSchedule := schedule.TemplateScheduleOptions{}

	// Failed build setup for failedstop tests.
	failedBuild := database.WorkspaceBuild{
		Transition: database.WorkspaceTransitionStart,
	}
	failedJob := database.ProvisionerJob{
		JobStatus:   database.ProvisionerJobStatusFailed,
		CompletedAt: sql.NullTime{Time: now.Add(-time.Hour), Valid: true},
	}
	failedTemplateSchedule := schedule.TemplateScheduleOptions{
		FailureTTL: time.Minute, // TTL already elapsed since job completed an hour ago.
	}

	testCases := []struct {
		Name             string
		Workspace        database.Workspace
		Build            database.WorkspaceBuild
		Job              database.ProvisionerJob
		TemplateSchedule schedule.TemplateScheduleOptions
		ExpectedReason   database.BuildReason
	}{
		{
			Name: "RegularWorkspace_Autostop",
			Workspace: database.Workspace{
				DormantAt: sql.NullTime{Valid: false},
			},
			Build:            okBuild,
			Job:              okJob,
			TemplateSchedule: okTemplateSchedule,
			ExpectedReason:   database.BuildReasonAutostop,
		},
		{
			Name: "TaskWorkspace_Autostop_UsesTaskAutoPause",
			Workspace: database.Workspace{
				DormantAt: sql.NullTime{Valid: false},
				TaskID:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
			},
			Build:            okBuild,
			Job:              okJob,
			TemplateSchedule: okTemplateSchedule,
			ExpectedReason:   database.BuildReasonTaskAutoPause,
		},
		{
			Name: "RegularWorkspace_FailedStop",
			Workspace: database.Workspace{
				DormantAt: sql.NullTime{Valid: false},
			},
			Build:            failedBuild,
			Job:              failedJob,
			TemplateSchedule: failedTemplateSchedule,
			ExpectedReason:   database.BuildReasonAutostop,
		},
		{
			Name: "TaskWorkspace_FailedStop_UsesTaskAutoPause",
			Workspace: database.Workspace{
				DormantAt: sql.NullTime{Valid: false},
				TaskID:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
			},
			Build:            failedBuild,
			Job:              failedJob,
			TemplateSchedule: failedTemplateSchedule,
			ExpectedReason:   database.BuildReasonTaskAutoPause,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			transition, reason, err := getNextTransition(
				okUser,
				tc.Workspace,
				tc.Build,
				tc.Job,
				tc.TemplateSchedule,
				now,
			)
			require.NoError(t, err)
			require.Equal(t, database.WorkspaceTransitionStop, transition)
			require.Equal(t, tc.ExpectedReason, reason)
		})
	}
}

func Test_getNextTransition_NoAction(t *testing.T) {
	t.Parallel()

	now := time.Now()

	// A stopped workspace with no autostart schedule, no dormancy, and no
	// deletion configured has no transition due. The default case must report
	// "nothing to do" via an empty transition AND empty reason, with a nil
	// error (not a sentinel error).
	user := database.User{Status: database.UserStatusActive}
	ws := database.Workspace{
		DormantAt: sql.NullTime{Valid: false},
	}
	build := database.WorkspaceBuild{
		Transition: database.WorkspaceTransitionStop,
	}
	job := database.ProvisionerJob{
		JobStatus: database.ProvisionerJobStatusSucceeded,
	}
	templateSchedule := schedule.TemplateScheduleOptions{}

	transition, reason, err := getNextTransition(user, ws, build, job, templateSchedule, now)
	require.NoError(t, err)
	require.Equal(t, database.WorkspaceTransition(""), transition)
	require.Equal(t, database.BuildReason(""), reason)
}

func TestShouldRemindAutostop(t *testing.T) {
	t.Parallel()

	currentTick := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	const ttl = time.Hour

	// inWindow places the deadline 30m out, inside the 1h lead window.
	inWindow := func() database.WorkspaceBuild {
		return database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionStart,
			Deadline:   currentTick.Add(30 * time.Minute),
		}
	}

	// idle places last_used_at well outside the 15-minute active threshold so the
	// active-user guard never trips. It is the default for cases that leave
	// LastUsedAt unset; cases that exercise the active-user guard set LastUsedAt
	// explicitly.
	idle := currentTick.Add(-2 * ttl)

	testCases := []struct {
		Name             string
		Build            database.WorkspaceBuild
		LastUsedAt       time.Time
		TemplateSchedule schedule.TemplateScheduleOptions
		Expected         bool
	}{
		{
			Name:             "InWindow",
			Build:            inWindow(),
			TemplateSchedule: schedule.TemplateScheduleOptions{TimeTilAutostopNotify: ttl},
			Expected:         true,
		},
		{
			Name:             "TemplateDisabled",
			Build:            inWindow(),
			TemplateSchedule: schedule.TemplateScheduleOptions{TimeTilAutostopNotify: 0},
			Expected:         false,
		},
		{
			Name: "TransitionStop",
			Build: func() database.WorkspaceBuild {
				b := inWindow()
				b.Transition = database.WorkspaceTransitionStop
				return b
			}(),
			TemplateSchedule: schedule.TemplateScheduleOptions{TimeTilAutostopNotify: ttl},
			Expected:         false,
		},
		{
			Name: "ZeroDeadline",
			Build: func() database.WorkspaceBuild {
				b := inWindow()
				b.Deadline = time.Time{}
				return b
			}(),
			TemplateSchedule: schedule.TemplateScheduleOptions{TimeTilAutostopNotify: ttl},
			Expected:         false,
		},
		{
			Name: "DeadlineInPast",
			Build: func() database.WorkspaceBuild {
				b := inWindow()
				b.Deadline = currentTick.Add(-time.Minute)
				return b
			}(),
			TemplateSchedule: schedule.TemplateScheduleOptions{TimeTilAutostopNotify: ttl},
			Expected:         false,
		},
		{
			Name: "BeforeWindow",
			Build: func() database.WorkspaceBuild {
				b := inWindow()
				// Deadline two hours out, ttl is only one hour.
				b.Deadline = currentTick.Add(2 * time.Hour)
				return b
			}(),
			TemplateSchedule: schedule.TemplateScheduleOptions{TimeTilAutostopNotify: ttl},
			Expected:         false,
		},
		{
			Name: "AlreadyNotified",
			Build: func() database.WorkspaceBuild {
				b := inWindow()
				b.NotifiedAutostopDeadline = b.Deadline
				return b
			}(),
			TemplateSchedule: schedule.TemplateScheduleOptions{TimeTilAutostopNotify: ttl},
			Expected:         false,
		},
		{
			// Deadline == currentTick: the stop is already due, so
			// !build.Deadline.After(currentTick) rejects it (not a reminder).
			Name: "ExactDeadline",
			Build: func() database.WorkspaceBuild {
				b := inWindow()
				b.Deadline = currentTick
				return b
			}(),
			TemplateSchedule: schedule.TemplateScheduleOptions{TimeTilAutostopNotify: ttl},
			Expected:         false,
		},
		{
			// Deadline exactly at the window opening edge (now + ttl) is
			// eligible: the lead-window check uses After, so the edge passes.
			Name: "WindowEdge",
			Build: func() database.WorkspaceBuild {
				b := inWindow()
				b.Deadline = currentTick.Add(ttl)
				return b
			}(),
			TemplateSchedule: schedule.TemplateScheduleOptions{TimeTilAutostopNotify: ttl},
			Expected:         true,
		},
		{
			// ActiveUser: the workspace was used within the 15-minute active
			// threshold and activity bumps are enabled with no max_deadline
			// ceiling, so the deadline can keep getting bumped out of the window
			// and the reminder is suppressed.
			Name:             "ActiveUser",
			Build:            inWindow(),
			LastUsedAt:       currentTick.Add(-1 * time.Minute),
			TemplateSchedule: schedule.TemplateScheduleOptions{TimeTilAutostopNotify: ttl, ActivityBump: time.Hour},
			Expected:         false,
		},
		{
			// ActiveButMaxDeadlineWithinWindow: the user is active and bumps are
			// enabled, but the hard max_deadline ceiling sits inside the lead
			// window, so a bump cannot push the stop out. The workspace will stop
			// regardless of activity, so we still remind.
			Name: "ActiveButMaxDeadlineWithinWindow",
			Build: func() database.WorkspaceBuild {
				b := inWindow()
				b.MaxDeadline = currentTick.Add(ttl / 2)
				return b
			}(),
			LastUsedAt:       currentTick.Add(-1 * time.Minute),
			TemplateSchedule: schedule.TemplateScheduleOptions{TimeTilAutostopNotify: ttl, ActivityBump: time.Hour},
			Expected:         true,
		},
		{
			// ActiveButBumpDisabled: the user is active, but activity bumps are
			// disabled (activity_bump == 0), so the deadline cannot move. The
			// workspace will stop, so we still remind.
			Name:             "ActiveButBumpDisabled",
			Build:            inWindow(),
			LastUsedAt:       currentTick.Add(-1 * time.Minute),
			TemplateSchedule: schedule.TemplateScheduleOptions{TimeTilAutostopNotify: ttl, ActivityBump: 0},
			Expected:         true,
		},
		{
			// IdleUser: the workspace was last used 20 minutes ago, outside the
			// 15-minute active threshold, so it is not active and the reminder
			// fires. Activity bumps are enabled here, so the true result is
			// genuinely due to idleness and not to disabled bumping.
			Name:             "IdleUser",
			Build:            inWindow(),
			LastUsedAt:       currentTick.Add(-20 * time.Minute),
			TemplateSchedule: schedule.TemplateScheduleOptions{TimeTilAutostopNotify: ttl, ActivityBump: time.Hour},
			Expected:         true,
		},
		{
			// Exactly at the threshold: the Go guard (< threshold) treats
			// the user as not active and reminds; the SQL complement
			// (>= threshold) keeps the row. Both agree; pins the boundary
			// against a "<"/">=" off-by-one regression.
			Name:             "ActiveThresholdBoundary",
			Build:            inWindow(),
			LastUsedAt:       currentTick.Add(-autostopReminderActiveThreshold),
			TemplateSchedule: schedule.TemplateScheduleOptions{TimeTilAutostopNotify: ttl, ActivityBump: time.Hour},
			Expected:         true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			// Cases that do not exercise the active-user guard leave LastUsedAt
			// unset; default those to an idle time outside the lead window.
			lastUsedAt := tc.LastUsedAt
			if lastUsedAt.IsZero() {
				lastUsedAt = idle
			}

			require.Equal(t, tc.Expected, shouldRemindAutostop(tc.Build, lastUsedAt, tc.TemplateSchedule, currentTick))
		})
	}
}

func Test_isEligibleForAutostart(t *testing.T) {
	t.Parallel()

	// okXXX should be set to values that make 'isEligibleForAutostart' return true.

	// Intentionally chosen to be a non UTC time that changes the day of the week
	// when converted to UTC.
	localLocation, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatal(err)
	}

	// 5s after the autostart in UTC.
	okTick := time.Date(2021, 1, 1, 20, 0, 5, 0, localLocation).UTC()
	okUser := database.User{Status: database.UserStatusActive}
	okWorkspace := database.Workspace{
		DormantAt: sql.NullTime{Valid: false},
		AutostartSchedule: sql.NullString{
			Valid: true,
			// Every day at 8pm America/Chicago, which is 2am UTC the next day.
			String: "CRON_TZ=America/Chicago 0 20 * * *",
		},
	}
	okBuild := database.WorkspaceBuild{
		Transition: database.WorkspaceTransitionStop,
		// Put 24hr before the tick so it's eligible for autostart.
		CreatedAt: okTick.Add(time.Hour * -24),
	}
	okJob := database.ProvisionerJob{
		JobStatus: database.ProvisionerJobStatusSucceeded,
	}
	okTemplateSchedule := schedule.TemplateScheduleOptions{
		UserAutostartEnabled: true,
		AutostartRequirement: schedule.TemplateAutostartRequirement{
			DaysOfWeek: 0b01111111,
		},
	}
	var okWeekdayBit uint8
	for i, weekday := range schedule.DaysOfWeek {
		// Find the local weekday
		if okTick.In(localLocation).Weekday() == weekday {
			// #nosec G115 - Safe conversion as i is the index of a 7-day week and will be in the range 0-6
			okWeekdayBit = 1 << uint(i)
		}
	}

	testCases := []struct {
		Name             string
		User             database.User
		Workspace        database.Workspace
		Build            database.WorkspaceBuild
		Job              database.ProvisionerJob
		TemplateSchedule schedule.TemplateScheduleOptions
		Tick             time.Time

		ExpectedResponse bool
	}{
		{
			Name:             "Ok",
			User:             okUser,
			Workspace:        okWorkspace,
			Build:            okBuild,
			Job:              okJob,
			TemplateSchedule: okTemplateSchedule,
			Tick:             okTick,
			ExpectedResponse: true,
		},
		{
			Name:             "SuspendedUser",
			User:             database.User{Status: database.UserStatusSuspended},
			Workspace:        okWorkspace,
			Build:            okBuild,
			Job:              okJob,
			TemplateSchedule: okTemplateSchedule,
			Tick:             okTick,
			ExpectedResponse: false,
		},
		{
			Name:      "AutostartOnlyDayEnabled",
			User:      okUser,
			Workspace: okWorkspace,
			Build:     okBuild,
			Job:       okJob,
			TemplateSchedule: schedule.TemplateScheduleOptions{
				UserAutostartEnabled: true,
				AutostartRequirement: schedule.TemplateAutostartRequirement{
					// Specific day of week is allowed
					DaysOfWeek: okWeekdayBit,
				},
			},
			Tick:             okTick,
			ExpectedResponse: true,
		},
		{
			Name:      "AutostartOnlyDayDisabled",
			User:      okUser,
			Workspace: okWorkspace,
			Build:     okBuild,
			Job:       okJob,
			TemplateSchedule: schedule.TemplateScheduleOptions{
				UserAutostartEnabled: true,
				AutostartRequirement: schedule.TemplateAutostartRequirement{
					// Specific day of week is disallowed
					DaysOfWeek: 0b01111111 & (^okWeekdayBit),
				},
			},
			Tick:             okTick,
			ExpectedResponse: false,
		},
		{
			Name:      "AutostartAllDaysDisabled",
			User:      okUser,
			Workspace: okWorkspace,
			Build:     okBuild,
			Job:       okJob,
			TemplateSchedule: schedule.TemplateScheduleOptions{
				UserAutostartEnabled: true,
				AutostartRequirement: schedule.TemplateAutostartRequirement{
					// All days disabled
					DaysOfWeek: 0,
				},
			},
			Tick:             okTick,
			ExpectedResponse: false,
		},
		{
			Name:      "BuildTransitionNotStop",
			User:      okUser,
			Workspace: okWorkspace,
			Build: func(b database.WorkspaceBuild) database.WorkspaceBuild {
				cpy := b
				cpy.Transition = database.WorkspaceTransitionStart
				return cpy
			}(okBuild),
			Job:              okJob,
			TemplateSchedule: okTemplateSchedule,
			Tick:             okTick,
			ExpectedResponse: false,
		},
	}

	for _, c := range testCases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			autostart := isEligibleForAutostart(c.User, c.Workspace, c.Build, c.Job, c.TemplateSchedule, c.Tick)
			require.Equal(t, c.ExpectedResponse, autostart, "autostart not expected")
		})
	}
}
