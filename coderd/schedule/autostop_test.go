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

	// In Sydney, clocks jump from 02:00 to 03:00 on October 1, 2023,
	// skipping that day's 02:30 quiet-hours start.
	dstIn := time.Date(2023, 10, 1, 2, 0, 0, 0, sydneyLoc)
	dstInQuietHours := "CRON_TZ=Australia/Sydney 30 2 * * *"
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
		name             string
		buildCompletedAt time.Time
		notBefore        time.Time

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
			buildCompletedAt:            now,
			templateAllowAutostop:       true,
			templateDefaultTTL:          0,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{},
			workspaceTTL:                0,
			expectedDeadline:            time.Time{},
			expectedMaxDeadline:         time.Time{},
		},
		{
			name:                        "Delete",
			buildCompletedAt:            now,
			templateAllowAutostop:       true,
			templateDefaultTTL:          0,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{},
			workspaceTTL:                0,
			expectedDeadline:            time.Time{},
			expectedMaxDeadline:         time.Time{},
		},
		{
			name:                        "WorkspaceTTL",
			buildCompletedAt:            now,
			templateAllowAutostop:       true,
			templateDefaultTTL:          0,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{},
			workspaceTTL:                time.Hour,
			expectedDeadline:            now.Add(time.Hour),
			expectedMaxDeadline:         time.Time{},
		},
		{
			name:                        "TemplateDefaultTTLIgnored",
			buildCompletedAt:            now,
			templateAllowAutostop:       true,
			templateDefaultTTL:          time.Hour,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{},
			workspaceTTL:                0,
			expectedDeadline:            time.Time{},
			expectedMaxDeadline:         time.Time{},
		},
		{
			name:                        "WorkspaceTTLOverridesTemplateDefaultTTL",
			buildCompletedAt:            now,
			templateAllowAutostop:       true,
			templateDefaultTTL:          2 * time.Hour,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{},
			workspaceTTL:                time.Hour,
			expectedDeadline:            now.Add(time.Hour),
			expectedMaxDeadline:         time.Time{},
		},
		{
			name:                        "TemplateBlockWorkspaceTTL",
			buildCompletedAt:            now,
			templateAllowAutostop:       false,
			templateDefaultTTL:          3 * time.Hour,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{},
			workspaceTTL:                4 * time.Hour,
			expectedDeadline:            now.Add(3 * time.Hour),
			expectedMaxDeadline:         time.Time{},
		},
		{
			name:                   "TemplateAutostopRequirement",
			buildCompletedAt:       wednesdayMidnightUTC,
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
			// The build completes before Friday's quiet hours. The
			// Saturday-only rule requires Saturday's 23:50 start.
			name:                   "TemplateAutostopRequirement/SkipsPreviousDayQuietHours",
			buildCompletedAt:       time.Date(2023, 1, 20, 12, 0, 0, 0, time.UTC),
			userQuietHoursSchedule: "CRON_TZ=UTC 50 23 * * *",
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      1,          // weekly
			},
			expectedMaxDeadline: time.Date(2023, 1, 21, 23, 50, 0, 0, time.UTC),
		},
		{
			name:                   "TemplateAutostopRequirement1HourSkip",
			buildCompletedAt:       saturdayMidnightSydney.Add(-59 * time.Minute),
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
			buildCompletedAt:       fridayEveningSydney,
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
			buildCompletedAt:       wednesdayMidnightUTC,
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
			buildCompletedAt:       wednesdayMidnightUTC.AddDate(0, 0, 7),
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
			// January 21 is in an eligible week, after its Friday stop.
			// January 27 falls in an off week.
			name:                   "TemplateAutostopRequirementFortnightly/AfterLastRequiredDay",
			buildCompletedAt:       time.Date(2023, 1, 21, 12, 0, 0, 0, time.UTC),
			userQuietHoursSchedule: "CRON_TZ=UTC 0 0 * * *",
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b00010000, // Friday
				Weeks:      2,          // every 2 weeks
			},
			expectedMaxDeadline: time.Date(2023, 2, 3, 0, 0, 0, 0, time.UTC),
		},
		{
			name:                   "TemplateAutostopRequirementTriweekly/Skip",
			buildCompletedAt:       wednesdayMidnightUTC,
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
			buildCompletedAt:       wednesdayMidnightUTC.AddDate(0, 0, 7),
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
			buildCompletedAt:       fridayEveningSydney.In(time.UTC),
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
			buildCompletedAt:       fridayEveningSydney.In(time.UTC),
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
			buildCompletedAt:       time.Date(2023, 1, 1, 21, 59, 59, 0, sydneyLoc),
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
			buildCompletedAt:       duringDst,
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
			buildCompletedAt:       beforeDstIn,
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
			buildCompletedAt:       beforeDstOut,
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
			buildCompletedAt:       beforeDstIn.Add(-24 * time.Hour),
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
			// The save cutoff moves the search from the previous Sunday to
			// the spring-forward Sunday. The clock jump skips 02:30 that day,
			// so the deadline is Monday at 02:30.
			name:                   "DaylightSavings/TemplateSaveOnSpringForward",
			buildCompletedAt:       beforeDstIn.AddDate(0, 0, -7),
			notBefore:              beforeDstIn,
			templateAllowAutostop:  true,
			userQuietHoursSchedule: dstInQuietHours,
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b01000000, // Sunday
				Weeks:      1,          // weekly
			},
			expectedMaxDeadline: dstInQuietHoursExpectedTime,
		},
		{
			// Saving at 04:00 in Chatham (UTC+13:45) is after the clock
			// jump skipped that Sunday's 02:45 quiet-hours start.
			name:                   "DaylightSavings/ChathamSaveAfterSpringForwardUsesNextWeek",
			buildCompletedAt:       time.Date(2025, time.September, 27, 0, 0, 0, 0, time.UTC),
			notBefore:              time.Date(2025, time.September, 27, 14, 15, 0, 0, time.UTC),
			userQuietHoursSchedule: "CRON_TZ=Pacific/Chatham 45 2 * * *",
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b01000000, // Sunday
				Weeks:      1,          // weekly
			},
			// October 5 at 02:45 in Chatham (UTC+13:45).
			expectedMaxDeadline: time.Date(2025, time.October, 4, 13, 0, 0, 0, time.UTC),
		},
		{
			name:                   "DaylightSavings/QuietHoursFallsOnDstSwitch/Out",
			buildCompletedAt:       beforeDstOut.Add(-24 * time.Hour),
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
			// In Seattle, 01:30 occurs twice on this Sunday. The cutoff at
			// 01:45 PDT falls between the two quiet-hours starts.
			name:                   "DaylightSavings/UsesSecondOccurrenceAfterCutoff",
			buildCompletedAt:       time.Date(2025, time.November, 1, 19, 0, 0, 0, time.UTC),
			notBefore:              time.Date(2025, time.November, 2, 8, 45, 0, 0, time.UTC),
			userQuietHoursSchedule: "CRON_TZ=America/Los_Angeles 30 1 * * *",
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b01000000, // Sunday
				Weeks:      1,          // weekly
			},
			// Sunday 01:30 PST.
			expectedMaxDeadline: time.Date(2025, time.November, 2, 9, 30, 0, 0, time.UTC),
		},
		{
			// The build completes Sunday at 04:00 in Chatham (UTC+12:45),
			// after the real 03:45 quiet-hours start on the rollback day.
			name:                   "DaylightSavings/ChathamBuildAfterQuietHoursUsesNextWeek",
			buildCompletedAt:       time.Date(2025, time.April, 5, 15, 15, 0, 0, time.UTC),
			userQuietHoursSchedule: "CRON_TZ=Pacific/Chatham 45 3 * * *",
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b01000000, // Sunday
				Weeks:      1,          // weekly
			},
			// April 13 at 03:45 in Chatham.
			expectedMaxDeadline: time.Date(2025, time.April, 12, 15, 0, 0, 0, time.UTC),
		},
		{
			// Saving at the second 03:15 in Chatham still leaves today's
			// 03:45 quiet-hours start available.
			name:                   "DaylightSavings/RecoversQuietHoursOnSameDay",
			buildCompletedAt:       time.Date(2025, time.April, 5, 0, 0, 0, 0, time.UTC),
			notBefore:              time.Date(2025, time.April, 5, 14, 30, 0, 0, time.UTC),
			userQuietHoursSchedule: "CRON_TZ=Pacific/Chatham 45 3 * * *",
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b01000000, // Sunday
				Weeks:      1,          // weekly
			},
			// April 6 at 03:45 in Chatham (UTC+12:45).
			expectedMaxDeadline: time.Date(2025, time.April, 5, 15, 0, 0, 0, time.UTC),
		},
		{
			// Saving at the first 03:15 in Chatham (UTC+13:45) precedes
			// the rollback. Cron can then report an early 02:45 start.
			name:                   "DaylightSavings/ChathamSaveBeforeRollbackUsesSameDay",
			buildCompletedAt:       time.Date(2025, time.April, 5, 0, 0, 0, 0, time.UTC),
			notBefore:              time.Date(2025, time.April, 5, 13, 30, 0, 0, time.UTC),
			userQuietHoursSchedule: "CRON_TZ=Pacific/Chatham 45 3 * * *",
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b01000000, // Sunday
				Weeks:      1,          // weekly
			},
			// April 6 at 03:45 in Chatham (UTC+12:45).
			expectedMaxDeadline: time.Date(2025, time.April, 5, 15, 0, 0, 0, time.UTC),
		},
		{
			// Lord Howe rolls back from 02:00 to 01:30. Saving at the
			// second 01:45 still leaves that Sunday's 02:00 available.
			name:                   "DaylightSavings/LordHoweUsesQuietHoursOnRollbackDay",
			buildCompletedAt:       time.Date(2025, time.April, 5, 0, 0, 0, 0, time.UTC),
			notBefore:              time.Date(2025, time.April, 5, 15, 15, 0, 0, time.UTC),
			userQuietHoursSchedule: "CRON_TZ=Australia/Lord_Howe 0 2 * * *",
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b01000000, // Sunday
				Weeks:      1,          // weekly
			},
			// April 6 at 02:00 in Lord Howe (UTC+10:30).
			expectedMaxDeadline: time.Date(2025, time.April, 5, 15, 30, 0, 0, time.UTC),
		},
		{
			// Goose Bay rolled back from Sunday 00:01 to Saturday 22:01,
			// so Saturday 23:50 occurs twice.
			name:                   "DaylightSavings/SkipsBothPreviousDayOccurrences",
			buildCompletedAt:       time.Date(1988, time.October, 30, 0, 0, 0, 0, time.UTC),
			userQuietHoursSchedule: "CRON_TZ=America/Goose_Bay 50 23 * * *",
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b01000000, // Sunday
				Weeks:      1,          // weekly
			},
			// October 30 at 23:50 in Goose Bay.
			expectedMaxDeadline: time.Date(1988, time.October, 31, 3, 50, 0, 0, time.UTC),
		},
		{
			// The cutoff is the first Sunday 00:00:30 in Goose Bay
			// (UTC-02:00). The rollback makes Saturday 23:50 recur later.
			name:                   "DaylightSavings/UsesRepeatedPreviousDayAfterCutoff",
			buildCompletedAt:       time.Date(1988, time.October, 30, 0, 0, 0, 0, time.UTC),
			notBefore:              time.Date(1988, time.October, 30, 2, 0, 30, 0, time.UTC),
			userQuietHoursSchedule: "CRON_TZ=America/Goose_Bay 50 23 * * *",
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      1,          // weekly
			},
			// October 29 at 23:50 in Goose Bay (UTC-04:00).
			expectedMaxDeadline: time.Date(1988, time.October, 30, 3, 50, 0, 0, time.UTC),
		},
		{
			// Manila skipped Tuesday, December 31, 1844. The first eligible
			// Tuesday after the December 24 build is January 7, 14 days later.
			name: "SkippedCalendarDay",
			// December 24 at 08:00 in Manila, when the local offset was UTC-15:56:08.
			buildCompletedAt:       time.Date(1844, time.December, 24, 23, 56, 8, 0, time.UTC),
			userQuietHoursSchedule: "CRON_TZ=Asia/Manila 0 0 * * *",
			templateAutostopRequirement: schedule.TemplateAutostopRequirement{
				DaysOfWeek: 0b00000010, // Tuesday
				Weeks:      1,
			},
			// January 7 at 00:00 in Manila, after the offset advanced to UTC+08:03:52.
			expectedMaxDeadline: time.Date(1845, time.January, 6, 15, 56, 8, 0, time.UTC),
		},
		{
			// A user expects this workspace to be online from 9am -> 9pm.
			// So if a deadline is going to land in the middle of this range,
			// we should bump it to the end.
			// This is already done on `ActivityBumpWorkspace`, but that requires
			// activity on the workspace.
			name: "AutostopCrossAutostartBorder",
			// Starting at 9:45pm, with the autostart at 9am.
			buildCompletedAt:      pastDateNight,
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
			buildCompletedAt:      time.Date(pastDateNight.Year(), pastDateNight.Month(), pastDateNight.Day(), 20, 30, 0, 0, chicago),
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
			buildCompletedAt:      pastDateNight,
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
			buildCompletedAt:      pastDateNight,
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
				Database:                     db,
				TemplateScheduleStore:        templateScheduleStore,
				UserQuietHoursScheduleStore:  userQuietHoursScheduleStore,
				WorkspaceBuildCompletedAt:    c.buildCompletedAt,
				Workspace:                    workspace,
				WorkspaceAutostart:           c.wsAutostart,
				AutostopRequirementNotBefore: c.notBefore,
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
		t.Run("Loc/"+tz, func(t *testing.T) {
			t.Parallel()

			loc, err := time.LoadLocation(tz)
			require.NoError(t, err)

			epoch := time.Date(2023, time.January, 2, 0, 0, 0, 0, loc)
			for expectedWeek := range int64(52 * 100) {
				msg := fmt.Sprintf("week %d", expectedWeek)

				monday := epoch.AddDate(0, 0, int(expectedWeek*7))
				require.Equal(t, time.Monday, monday.Weekday(), msg)
				t.Log(msg, "monday", monday)

				week, err := schedule.WeeksSinceEpoch(monday)
				require.NoError(t, err, msg)
				require.Equal(t, expectedWeek, week, msg)

				// Check that we get the same week number for late Sunday.
				y, m, d := monday.Date()
				sunday := time.Date(y, m, d+6, 23, 59, 59, 0, loc)
				require.Equal(t, time.Sunday, sunday.Weekday(), msg)
				t.Log(msg, "sunday", sunday)

				week, err = schedule.WeeksSinceEpoch(sunday)
				require.NoError(t, err, msg)
				require.Equal(t, expectedWeek, week, msg)
			}
		})
	}
}
