package schedule_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/testutil"
)

func TestCalculateAutoStop(t *testing.T) {
	t.Parallel()

	now := time.Now()

	chicago, err := time.LoadLocation("America/Chicago")
	require.NoError(t, err, "loading chicago time location")

	// pastDateNight is 9:45pm on a wednesday
	pastDateNight := time.Date(2024, 2, 14, 21, 45, 0, 0, chicago)

	// Wednesday the 8th of February 2023 at midnight. This date was
	// specifically chosen as it doesn't fall on a applicable week for both
	// fortnightly and triweekly autostop requirements.
	wednesdayMidnightUTC := time.Date(2023, 2, 8, 0, 0, 0, 0, time.UTC)

	sydneyQuietHours := "CRON_TZ=Australia/Sydney 0 0 * * *"
	sydneyLoc, err := time.LoadLocation("Australia/Sydney")
	require.NoError(t, err)
	// 10pm on Friday the 10th of February 2023 in Sydney.
	fridayEveningSydney := time.Date(2023, 2, 10, 22, 0, 0, 0, sydneyLoc)
	// 12am on Saturday the 11th of February2023 in Sydney.
	saturdayMidnightSydney := time.Date(2023, 2, 11, 0, 0, 0, 0, sydneyLoc)

	t.Log("now", now)
	t.Log("wednesdayMidnightUTC", wednesdayMidnightUTC)
	t.Log("fridayEveningSydney", fridayEveningSydney)
	t.Log("saturdayMidnightSydney", saturdayMidnightSydney)

	dstIn := time.Date(2023, 10, 1, 2, 0, 0, 0, sydneyLoc)   // 1 hour backward
	dstInQuietHours := "CRON_TZ=Australia/Sydney 30 2 * * *" // never
	// The expected behavior is that we will pick the next time that falls on
	// quiet hours after the DST transition. In this case, it will be the same
	// time the next day.
	dstInQuietHoursExpectedTime := time.Date(2023, 10, 2, 2, 30, 0, 0, sydneyLoc)
	beforeDstIn := time.Date(2023, 10, 1, 0, 0, 0, 0, sydneyLoc)
	saturdayMidnightAfterDstIn := time.Date(2023, 10, 7, 0, 0, 0, 0, sydneyLoc)

	// Wednesday after DST starts.
	duringDst := time.Date(2023, 10, 4, 0, 0, 0, 0, sydneyLoc)
	saturdayMidnightAfterDuringDst := saturdayMidnightAfterDstIn

	dstOut := time.Date(2024, 4, 7, 3, 0, 0, 0, sydneyLoc)                        // 1 hour forward
	dstOutQuietHours := "CRON_TZ=Australia/Sydney 30 3 * * *"                     // twice
	dstOutQuietHoursExpectedTime := time.Date(2024, 4, 7, 3, 30, 0, 0, sydneyLoc) // in reality, this is the first occurrence
	beforeDstOut := time.Date(2024, 4, 7, 0, 0, 0, 0, sydneyLoc)
	saturdayMidnightAfterDstOut := time.Date(2024, 4, 13, 0, 0, 0, 0, sydneyLoc)

	t.Log("dstIn", dstIn)
	t.Log("beforeDstIn", beforeDstIn)
	t.Log("saturdayMidnightAfterDstIn", saturdayMidnightAfterDstIn)
	t.Log("dstOut", dstOut)
	t.Log("beforeDstOut", beforeDstOut)
	t.Log("saturdayMidnightAfterDstOut", saturdayMidnightAfterDstOut)

	cases := []struct {
		name string
		now  time.Time

		wsAutostart       string
		templateAutoStart schedule.TemplateAutostartRequirement

		templateAllowAutostop       bool
		templateDefaultTTL          time.Duration
		templateAutostopRequirement schedule.TemplateAutostopRequirement
		userQuietHoursSchedule      string
		// workspaceTTL is usually copied from the template's TTL when the
		// workspace is made, so it takes precedence unless
		// templateAllowAutostop is false.
		workspaceTTL time.Duration

		// expectedDeadline is copied from expectedMaxDeadline if unset.
		expectedDeadline    time.Time
		expectedMaxDeadline time.Time
		errContains         string
	}{
		{
			name:                        "OK",
			now:                         now,
			templateAllowAutostop:       true,
			templateDefaultTTL:          0,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{},
			workspaceTTL:                0,
			expectedDeadline:            time.Time{},
			expectedMaxDeadline:         time.Time{},
		},
		{
			name:                        "Delete",
			now:                         now,
			templateAllowAutostop:       true,
			templateDefaultTTL:          0,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{},
			workspaceTTL:                0,
			expectedDeadline:            time.Time{},
			expectedMaxDeadline:         time.Time{},
		},
		{
			name:                        "WorkspaceTTL",
			now:                         now,
			templateAllowAutostop:       true,
			templateDefaultTTL:          0,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{},
			workspaceTTL:                time.Hour,
			expectedDeadline:            now.Add(time.Hour),
			expectedMaxDeadline:         time.Time{},
		},
		{
			name:                        "TemplateDefaultTTLIgnored",
			now:                         now,
			templateAllowAutostop:       true,
			templateDefaultTTL:          time.Hour,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{},
			workspaceTTL:                0,
			expectedDeadline:            time.Time{},
			expectedMaxDeadline:         time.Time{},
		},
		{
			name:                        "WorkspaceTTLOverridesTemplateDefaultTTL",
			now:                         now,
			templateAllowAutostop:       true,
			templateDefaultTTL:          2 * time.Hour,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{},
			workspaceTTL:                time.Hour,
			expectedDeadline:            now.Add(time.Hour),
			expectedMaxDeadline:         time.Time{},
		},
		{
			name:                        "TemplateBlockWorkspaceTTL",
			now:                         now,
			templateAllowAutostop:       false,
			templateDefaultTTL:          3 * time.Hour,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{},
			workspaceTTL:                4 * time.Hour,
			expectedDeadline:            now.Add(3 * time.Hour),
			expectedMaxDeadline:         time.Time{},
		},
		{
			name:                   "TemplateAutostopRequirement",
			now:                    wednesdayMidnightUTC,
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: sydneyQuietHours,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      0,          // weekly
			},
			workspaceTTL: 0,
			// expectedDeadline is copied from expectedMaxDeadline.
			expectedMaxDeadline: saturdayMidnightSydney.In(time.UTC),
		},
		{
			name:                   "TemplateAutostopRequirement1HourSkip",
			now:                    saturdayMidnightSydney.Add(-59 * time.Minute),
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: sydneyQuietHours,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      1,          // 1 also means weekly
			},
			workspaceTTL: 0,
			// expectedDeadline is copied from expectedMaxDeadline.
			expectedMaxDeadline: saturdayMidnightSydney.Add(7 * 24 * time.Hour).In(time.UTC),
		},
		{
			// The next autostop requirement should be skipped if the
			// workspace is started within 1 hour of it.
			name:                   "TemplateAutostopRequirementDaily",
			now:                    fridayEveningSydney,
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: sydneyQuietHours,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b01111111, // daily
				Weeks:      0,          // all weeks
			},
			workspaceTTL: 0,
			// expectedDeadline is copied from expectedMaxDeadline.
			expectedMaxDeadline: saturdayMidnightSydney.In(time.UTC),
		},
		{
			name:                   "TemplateAutostopRequirementFortnightly/Skip",
			now:                    wednesdayMidnightUTC,
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: sydneyQuietHours,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      2,          // every 2 weeks
			},
			workspaceTTL: 0,
			// expectedDeadline is copied from expectedMaxDeadline.
			expectedMaxDeadline: saturdayMidnightSydney.AddDate(0, 0, 7).In(time.UTC),
		},
		{
			name:                   "TemplateAutostopRequirementFortnightly/NoSkip",
			now:                    wednesdayMidnightUTC.AddDate(0, 0, 7),
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: sydneyQuietHours,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      2,          // every 2 weeks
			},
			workspaceTTL: 0,
			// expectedDeadline is copied from expectedMaxDeadline.
			expectedMaxDeadline: saturdayMidnightSydney.AddDate(0, 0, 7).In(time.UTC),
		},
		{
			name:                   "TemplateAutostopRequirementTriweekly/Skip",
			now:                    wednesdayMidnightUTC,
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: sydneyQuietHours,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      3,          // every 3 weeks
			},
			workspaceTTL: 0,
			// expectedDeadline is copied from expectedMaxDeadline.
			// The next triweekly autostop requirement happens next week
			// according to the epoch.
			expectedMaxDeadline: saturdayMidnightSydney.AddDate(0, 0, 7).In(time.UTC),
		},
		{
			name:                   "TemplateAutostopRequirementTriweekly/NoSkip",
			now:                    wednesdayMidnightUTC.AddDate(0, 0, 7),
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: sydneyQuietHours,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      3,          // every 3 weeks
			},
			workspaceTTL: 0,
			// expectedDeadline is copied from expectedMaxDeadline.
			expectedMaxDeadline: saturdayMidnightSydney.AddDate(0, 0, 7).In(time.UTC),
		},
		{
			name: "TemplateAutostopRequirementOverridesWorkspaceTTL",
			// now doesn't have to be UTC, but it helps us ensure that
			// timezones are compared correctly in this test.
			now:                    fridayEveningSydney.In(time.UTC),
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: sydneyQuietHours,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      0,          // weekly
			},
			workspaceTTL: 3 * time.Hour,
			// expectedDeadline is copied from expectedMaxDeadline.
			expectedMaxDeadline: saturdayMidnightSydney.In(time.UTC),
		},
		{
			name:                   "TemplateAutostopRequirementOverridesTemplateDefaultTTL",
			now:                    fridayEveningSydney.In(time.UTC),
			templateAllowAutostop:  true,
			templateDefaultTTL:     3 * time.Hour,
			userQuietHoursSchedule: sydneyQuietHours,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      0,          // weekly
			},
			workspaceTTL: 0,
			// expectedDeadline is copied from expectedMaxDeadline.
			expectedMaxDeadline: saturdayMidnightSydney.In(time.UTC),
		},
		{
			name: "TimeBeforeEpoch",
			// The epoch is 2023-01-02 in each timezone. We set the time to
			// 1 second before 11pm the previous day, as this is the latest time
			// we allow due to our 2h leeway logic.
			now:                    time.Date(2023, 1, 1, 21, 59, 59, 0, sydneyLoc),
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: sydneyQuietHours,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      2,          // every fortnight
			},
			workspaceTTL: 0,
			errContains:  "coder server system clock is incorrect",
		},
		{
			name:                   "DaylightSavings/OK",
			now:                    duringDst,
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: sydneyQuietHours,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      1,          // weekly
			},
			workspaceTTL: 0,
			// expectedDeadline is copied from expectedMaxDeadline.
			expectedMaxDeadline: saturdayMidnightAfterDuringDst,
		},
		{
			name:                   "DaylightSavings/SwitchMidWeek/In",
			now:                    beforeDstIn,
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: sydneyQuietHours,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      1,          // weekly
			},
			workspaceTTL: 0,
			// expectedDeadline is copied from expectedMaxDeadline.
			expectedMaxDeadline: saturdayMidnightAfterDstIn,
		},
		{
			name:                   "DaylightSavings/SwitchMidWeek/Out",
			now:                    beforeDstOut,
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: sydneyQuietHours,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      1,          // weekly
			},
			workspaceTTL: 0,
			// expectedDeadline is copied from expectedMaxDeadline.
			expectedMaxDeadline: saturdayMidnightAfterDstOut,
		},
		{
			name:                   "DaylightSavings/QuietHoursFallsOnDstSwitch/In",
			now:                    beforeDstIn.Add(-24 * time.Hour),
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: dstInQuietHours,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b01000000, // Sunday
				Weeks:      1,          // weekly
			},
			workspaceTTL: 0,
			// expectedDeadline is copied from expectedMaxDeadline.
			expectedMaxDeadline: dstInQuietHoursExpectedTime,
		},
		{
			name:                   "DaylightSavings/QuietHoursFallsOnDstSwitch/Out",
			now:                    beforeDstOut.Add(-24 * time.Hour),
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: dstOutQuietHours,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b01000000, // Sunday
				Weeks:      1,          // weekly
			},
			workspaceTTL: 0,
			// expectedDeadline is copied from expectedMaxDeadline.
			expectedMaxDeadline: dstOutQuietHoursExpectedTime,
		},
		{
			// A user expects this workspace to be online from 9am -> 9pm.
			// So if a deadline is going to land in the middle of this range,
			// we should bump it to the end.
			// This is already done on `ActivityBumpWorkspace`, but that requires
			// activity on the workspace.
			name: "AutostopCrossAutostartBorder",
			// Starting at 9:45pm, with the autostart at 9am.
			now:                   pastDateNight,
			templateAllowAutostop: false,
			templateDefaultTTL:    time.Hour * 12,
			workspaceTTL:          time.Hour * 12,
			// At 9am every morning
			wsAutostart: "CRON_TZ=America/Chicago 0 9 * * *",

			// No quiet hours
			templateAutoStart: schedule.TemplateAutostartRequirement{
				// Just allow all days of the week
				DaysOfWeek: 0b01111111,
			},
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{},
			userQuietHoursSchedule:      "",

			expectedDeadline:    time.Date(pastDateNight.Year(), pastDateNight.Month(), pastDateNight.Day()+1, 21, 0, 0, 0, chicago),
			expectedMaxDeadline: time.Time{},
			errContains:         "",
		},
		{
			// Same as AutostopCrossAutostartBorder, but just misses the autostart.
			name: "AutostopCrossMissAutostartBorder",
			// Starting at 8:45pm, with the autostart at 9am.
			now:                   time.Date(pastDateNight.Year(), pastDateNight.Month(), pastDateNight.Day(), 20, 30, 0, 0, chicago),
			templateAllowAutostop: false,
			templateDefaultTTL:    time.Hour * 12,
			workspaceTTL:          time.Hour * 12,
			// At 9am every morning
			wsAutostart: "CRON_TZ=America/Chicago 0 9 * * *",

			// No quiet hours
			templateAutoStart: schedule.TemplateAutostartRequirement{
				// Just allow all days of the week
				DaysOfWeek: 0b01111111,
			},
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{},
			userQuietHoursSchedule:      "",

			expectedDeadline:    time.Date(pastDateNight.Year(), pastDateNight.Month(), pastDateNight.Day()+1, 8, 30, 0, 0, chicago),
			expectedMaxDeadline: time.Time{},
			errContains:         "",
		},
		{
			// Same as AutostopCrossAutostartBorderMaxEarlyDeadline with max deadline to limit it.
			// The autostop deadline is before the autostart threshold.
			name: "AutostopCrossAutostartBorderMaxEarlyDeadline",
			// Starting at 9:45pm, with the autostart at 9am.
			now:                   pastDateNight,
			templateAllowAutostop: false,
			templateDefaultTTL:    time.Hour * 12,
			workspaceTTL:          time.Hour * 12,
			// At 9am every morning
			wsAutostart: "CRON_TZ=America/Chicago 0 9 * * *",

			// No quiet hours
			templateAutoStart: schedule.TemplateAutostartRequirement{
				// Just allow all days of the week
				DaysOfWeek: 0b01111111,
			},
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				// Autostop every day
				DaysOfWeek: 0b01111111,
				Weeks:      0,
			},
			// 6am quiet hours
			userQuietHoursSchedule: "CRON_TZ=America/Chicago 0 6 * * *",

			expectedDeadline:    time.Date(pastDateNight.Year(), pastDateNight.Month(), pastDateNight.Day()+1, 6, 0, 0, 0, chicago),
			expectedMaxDeadline: time.Date(pastDateNight.Year(), pastDateNight.Month(), pastDateNight.Day()+1, 6, 0, 0, 0, chicago),
			errContains:         "",
		},
		{
			// Same as AutostopCrossAutostartBorder with max deadline to limit it.
			// The autostop deadline is after autostart threshold.
			// So the deadline is > 12 hours, but stops at the max deadline.
			name: "AutostopCrossAutostartBorderMaxDeadline",
			// Starting at 9:45pm, with the autostart at 9am.
			now:                   pastDateNight,
			templateAllowAutostop: false,
			templateDefaultTTL:    time.Hour * 12,
			workspaceTTL:          time.Hour * 12,
			// At 9am every morning
			wsAutostart: "CRON_TZ=America/Chicago 0 9 * * *",

			// No quiet hours
			templateAutoStart: schedule.TemplateAutostartRequirement{
				// Just allow all days of the week
				DaysOfWeek: 0b01111111,
			},
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				// Autostop every day
				DaysOfWeek: 0b01111111,
				Weeks:      0,
			},
			// 11am quiet hours, yea this is werid case.
			userQuietHoursSchedule: "CRON_TZ=America/Chicago 0 11 * * *",

			expectedDeadline:    time.Date(pastDateNight.Year(), pastDateNight.Month(), pastDateNight.Day()+1, 11, 0, 0, 0, chicago),
			expectedMaxDeadline: time.Date(pastDateNight.Year(), pastDateNight.Month(), pastDateNight.Day()+1, 11, 0, 0, 0, chicago),
			errContains:         "",
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			db, _ := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitLong)

			templateScheduleStore := schedule.MockTemplateScheduleStore{
				GetFn: func(_ context.Context, _ database.Store, _ uuid.UUID) (schedule.TemplateScheduleOptions, error) {
					return schedule.TemplateScheduleOptions{
						UserAutostartEnabled: false,
						UserAutostopEnabled:  c.templateAllowAutostop,
						DefaultTTL:           c.templateDefaultTTL,
						AutostopRequirement:  c.templateAutostopRequirement,
						AutostartRequirement: c.templateAutoStart,
					}, nil
				},
			}

			userQuietHoursScheduleStore := schedule.MockUserQuietHoursScheduleStore{
				GetFn: func(_ context.Context, _ database.Store, _ uuid.UUID) (schedule.UserQuietHoursScheduleOptions, error) {
					if c.userQuietHoursSchedule == "" {
						return schedule.UserQuietHoursScheduleOptions{
							Schedule: nil,
						}, nil
					}

					sched, err := cron.Daily(c.userQuietHoursSchedule)
					if !assert.NoError(t, err) {
						return schedule.UserQuietHoursScheduleOptions{}, err
					}

					return schedule.UserQuietHoursScheduleOptions{
						Schedule: sched,
						UserSet:  false,
					}, nil
				},
			}

			org := dbgen.Organization(t, db, database.Organization{})
			user := dbgen.User(t, db, database.User{
				QuietHoursSchedule: c.userQuietHoursSchedule,
			})
			template := dbgen.Template(t, db, database.Template{
				Name:           "template",
				Provisioner:    database.ProvisionerTypeEcho,
				OrganizationID: org.ID,
				CreatedBy:      user.ID,
			})
			err := db.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
				ID:                            template.ID,
				UpdatedAt:                     dbtime.Now(),
				AllowUserAutostart:            c.templateAllowAutostop,
				AutostopRequirementDaysOfWeek: int16(c.templateAutostopRequirement.DaysOfWeek),
				AutostopRequirementWeeks:      c.templateAutostopRequirement.Weeks,
			})
			require.NoError(t, err)
			template, err = db.GetTemplateByID(ctx, template.ID)
			require.NoError(t, err)
			workspaceTTL := sql.NullInt64{}
			if c.workspaceTTL != 0 {
				workspaceTTL = sql.NullInt64{
					Int64: int64(c.workspaceTTL),
					Valid: true,
				}
			}

			autostart := sql.NullString{}
			if c.wsAutostart != "" {
				autostart = sql.NullString{
					String: c.wsAutostart,
					Valid:  true,
				}
			}
			workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
				TemplateID:        template.ID,
				OrganizationID:    org.ID,
				OwnerID:           user.ID,
				Ttl:               workspaceTTL,
				AutostartSchedule: autostart,
			})

			autostop, err := schedule.CalculateAutostop(ctx, schedule.CalculateAutostopParams{
				Database:                    db,
				TemplateScheduleStore:       templateScheduleStore,
				UserQuietHoursScheduleStore: userQuietHoursScheduleStore,
				Now:                         c.now,
				Workspace:                   workspace,
				WorkspaceAutostart:          c.wsAutostart,
			})
			if c.errContains != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, c.errContains)
				return
			}
			require.NoError(t, err)

			// If the max deadline is set, the deadline should also be set.
			// Default to the max deadline if the deadline is not set.
			if c.expectedDeadline.IsZero() {
				c.expectedDeadline = c.expectedMaxDeadline
			}

			if c.expectedDeadline.IsZero() {
				require.True(t, autostop.Deadline.IsZero())
			} else {
				require.WithinDuration(t, c.expectedDeadline, autostop.Deadline, 15*time.Second, "deadline does not match expected")
			}
			if c.expectedMaxDeadline.IsZero() {
				require.True(t, autostop.MaxDeadline.IsZero())
			} else {
				require.WithinDuration(t, c.expectedMaxDeadline, autostop.MaxDeadline, 15*time.Second, "max deadline does not match expected")
				require.GreaterOrEqual(t, autostop.MaxDeadline.Unix(), autostop.Deadline.Unix(), "max deadline is smaller than deadline")
			}
		})
	}
}

func TestFindWeek(t *testing.T) {
	t.Parallel()

	timezones := []string{
		"UTC",
		"America/Los_Angeles",
		"America/New_York",
		"Europe/Dublin",
		"Europe/London",
		"Europe/Paris",
		"Asia/Kolkata", // India (UTC+5:30)
		"Asia/Tokyo",
		"Australia/Sydney",
		"Australia/Brisbane",
	}

	for _, tz := range timezones {
		tz := tz
		t.Run("Loc/"+tz, func(t *testing.T) {
			t.Parallel()

			loc, err := time.LoadLocation(tz)
			require.NoError(t, err)

			now := time.Now().In(loc)
			currentWeek, err := schedule.WeeksSinceEpoch(now)
			require.NoError(t, err)

			diffMonday := now.Weekday() - time.Monday
			if now.Weekday() == time.Sunday {
				// Sunday is 0, but Monday is the first day of the week in the
				// code.
				diffMonday = 6
			}
			currentWeekMondayExpected := now.AddDate(0, 0, -int(diffMonday))
			require.Equal(t, time.Monday, currentWeekMondayExpected.Weekday())
			y, m, d := currentWeekMondayExpected.Date()
			// Change to midnight.
			currentWeekMondayExpected = time.Date(y, m, d, 0, 0, 0, 0, loc)

			currentWeekMonday, err := schedule.GetMondayOfWeek(now.Location(), currentWeek)
			require.NoError(t, err)
			require.Equal(t, time.Monday, currentWeekMonday.Weekday())
			require.Equal(t, currentWeekMondayExpected, currentWeekMonday)

			t.Log("now", now)
			t.Log("currentWeek", currentWeek)
			t.Log("currentMonday", currentWeekMonday)

			// Loop through every single Monday and Sunday for the next 100
			// years and make sure the week calculations are correct.
			for i := int64(1); i < 52*100; i++ {
				msg := fmt.Sprintf("week %d", i)

				monday := currentWeekMonday.AddDate(0, 0, int(i*7))
				y, m, d := monday.Date()
				monday = time.Date(y, m, d, 0, 0, 0, 0, loc)
				require.Equal(t, monday.Weekday(), time.Monday, msg)
				t.Log(msg, "monday", monday)

				week, err := schedule.WeeksSinceEpoch(monday)
				require.NoError(t, err, msg)
				require.Equal(t, currentWeek+i, week, msg)

				gotMonday, err := schedule.GetMondayOfWeek(monday.Location(), week)
				require.NoError(t, err, msg)
				require.Equal(t, monday, gotMonday, msg)

				// Check that we get the same week number for late Sunday.
				sunday := time.Date(y, m, d+6, 23, 59, 59, 0, loc)
				require.Equal(t, sunday.Weekday(), time.Sunday, msg)
				t.Log(msg, "sunday", sunday)

				week, err = schedule.WeeksSinceEpoch(sunday)
				require.NoError(t, err, msg)
				require.Equal(t, currentWeek+i, week, msg)
			}
		})
	}
}
