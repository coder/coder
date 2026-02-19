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
