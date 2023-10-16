package autobuild

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/schedule"
)

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
			okWeekdayBit = 1 << uint(i)
		}
	}

	testCases := []struct {
		Name             string
		Workspace        database.Workspace
		Build            database.WorkspaceBuild
		Job              database.ProvisionerJob
		TemplateSchedule schedule.TemplateScheduleOptions
		Tick             time.Time

		ExpectedResponse bool
	}{
		{
			Name:             "Ok",
			Workspace:        okWorkspace,
			Build:            okBuild,
			Job:              okJob,
			TemplateSchedule: okTemplateSchedule,
			Tick:             okTick,
			ExpectedResponse: true,
		},
		{
			Name:      "AutostartOnlyDayEnabled",
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
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			autostart := isEligibleForAutostart(c.Workspace, c.Build, c.Job, c.TemplateSchedule, c.Tick)
			require.Equal(t, c.ExpectedResponse, autostart, "autostart not expected")
		})
	}
}
