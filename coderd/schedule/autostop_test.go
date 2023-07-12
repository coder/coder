package schedule_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/database/dbtestutil"
	"github.com/coder/coder/coderd/schedule"
	"github.com/coder/coder/testutil"
)

func TestCalculateAutoStop(t *testing.T) {
	t.Parallel()

	now := time.Now()

	// Wednesday the 8th of February 2023 at midnight. This date was
	// specifically chosen as it doesn't fall on a applicable week for both
	// fortnightly and triweekly restart requirements.
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

	cases := []struct {
		name                  string
		now                   time.Time
		templateAllowAutostop bool
		templateDefaultTTL    time.Duration
		// TODO(@dean): remove max_ttl tests
		useMaxTTL                  bool
		templateMaxTTL             time.Duration
		templateRestartRequirement schedule.TemplateRestartRequirement
		userQuietHoursSchedule     string
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
			name:                       "OK",
			now:                        now,
			templateAllowAutostop:      true,
			templateDefaultTTL:         0,
			templateRestartRequirement: schedule.TemplateRestartRequirement{},
			workspaceTTL:               0,
			expectedDeadline:           time.Time{},
			expectedMaxDeadline:        time.Time{},
		},
		{
			name:                       "Delete",
			now:                        now,
			templateAllowAutostop:      true,
			templateDefaultTTL:         0,
			templateRestartRequirement: schedule.TemplateRestartRequirement{},
			workspaceTTL:               0,
			expectedDeadline:           time.Time{},
			expectedMaxDeadline:        time.Time{},
		},
		{
			name:                       "WorkspaceTTL",
			now:                        now,
			templateAllowAutostop:      true,
			templateDefaultTTL:         0,
			templateRestartRequirement: schedule.TemplateRestartRequirement{},
			workspaceTTL:               time.Hour,
			expectedDeadline:           now.Add(time.Hour),
			expectedMaxDeadline:        time.Time{},
		},
		{
			name:                       "TemplateDefaultTTLIgnored",
			now:                        now,
			templateAllowAutostop:      true,
			templateDefaultTTL:         time.Hour,
			templateRestartRequirement: schedule.TemplateRestartRequirement{},
			workspaceTTL:               0,
			expectedDeadline:           time.Time{},
			expectedMaxDeadline:        time.Time{},
		},
		{
			name:                       "WorkspaceTTLOverridesTemplateDefaultTTL",
			now:                        now,
			templateAllowAutostop:      true,
			templateDefaultTTL:         2 * time.Hour,
			templateRestartRequirement: schedule.TemplateRestartRequirement{},
			workspaceTTL:               time.Hour,
			expectedDeadline:           now.Add(time.Hour),
			expectedMaxDeadline:        time.Time{},
		},
		{
			name:                       "TemplateBlockWorkspaceTTL",
			now:                        now,
			templateAllowAutostop:      false,
			templateDefaultTTL:         3 * time.Hour,
			templateRestartRequirement: schedule.TemplateRestartRequirement{},
			workspaceTTL:               4 * time.Hour,
			expectedDeadline:           now.Add(3 * time.Hour),
			expectedMaxDeadline:        time.Time{},
		},
		{
			name:                   "TemplateRestartRequirement",
			now:                    wednesdayMidnightUTC,
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: sydneyQuietHours,
			templateRestartRequirement: schedule.TemplateRestartRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      0,          // weekly
			},
			workspaceTTL: 0,
			// expectedDeadline is copied from expectedMaxDeadline.
			expectedMaxDeadline: saturdayMidnightSydney.In(time.UTC),
		},
		{
			name:                   "TemplateRestartRequirement1HourSkip",
			now:                    saturdayMidnightSydney.Add(-59 * time.Minute),
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: sydneyQuietHours,
			templateRestartRequirement: schedule.TemplateRestartRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      1,          // 1 also means weekly
			},
			workspaceTTL: 0,
			// expectedDeadline is copied from expectedMaxDeadline.
			expectedMaxDeadline: saturdayMidnightSydney.Add(7 * 24 * time.Hour).In(time.UTC),
		},
		{
			// The next restart requirement should be skipped if the
			// workspace is started within 1 hour of it.
			name:                   "TemplateRestartRequirementDaily",
			now:                    fridayEveningSydney,
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: sydneyQuietHours,
			templateRestartRequirement: schedule.TemplateRestartRequirement{
				DaysOfWeek: 0b01111111, // daily
				Weeks:      0,          // all weeks
			},
			workspaceTTL: 0,
			// expectedDeadline is copied from expectedMaxDeadline.
			expectedMaxDeadline: saturdayMidnightSydney.In(time.UTC),
		},
		{
			name:                   "TemplateRestartRequirementFortnightly/Skip",
			now:                    wednesdayMidnightUTC,
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: sydneyQuietHours,
			templateRestartRequirement: schedule.TemplateRestartRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      2,          // every 2 weeks
			},
			workspaceTTL: 0,
			// expectedDeadline is copied from expectedMaxDeadline.
			expectedMaxDeadline: saturdayMidnightSydney.AddDate(0, 0, 7).In(time.UTC),
		},
		{
			name:                   "TemplateRestartRequirementFortnightly/NoSkip",
			now:                    wednesdayMidnightUTC.AddDate(0, 0, 7),
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: sydneyQuietHours,
			templateRestartRequirement: schedule.TemplateRestartRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      2,          // every 2 weeks
			},
			workspaceTTL: 0,
			// expectedDeadline is copied from expectedMaxDeadline.
			expectedMaxDeadline: saturdayMidnightSydney.AddDate(0, 0, 7).In(time.UTC),
		},
		{
			name:                   "TemplateRestartRequirementTriweekly/Skip",
			now:                    wednesdayMidnightUTC,
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: sydneyQuietHours,
			templateRestartRequirement: schedule.TemplateRestartRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      3,          // every 3 weeks
			},
			workspaceTTL: 0,
			// expectedDeadline is copied from expectedMaxDeadline.
			// The next triweekly restart requirement happens next week
			// according to the epoch.
			expectedMaxDeadline: saturdayMidnightSydney.AddDate(0, 0, 7).In(time.UTC),
		},
		{
			name:                   "TemplateRestartRequirementTriweekly/NoSkip",
			now:                    wednesdayMidnightUTC.AddDate(0, 0, 7),
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: sydneyQuietHours,
			templateRestartRequirement: schedule.TemplateRestartRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      3,          // every 3 weeks
			},
			workspaceTTL: 0,
			// expectedDeadline is copied from expectedMaxDeadline.
			expectedMaxDeadline: saturdayMidnightSydney.AddDate(0, 0, 7).In(time.UTC),
		},
		{
			name: "TemplateRestartRequirementOverridesWorkspaceTTL",
			// now doesn't have to be UTC, but it helps us ensure that
			// timezones are compared correctly in this test.
			now:                    fridayEveningSydney.In(time.UTC),
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: sydneyQuietHours,
			templateRestartRequirement: schedule.TemplateRestartRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      0,          // weekly
			},
			workspaceTTL: 3 * time.Hour,
			// expectedDeadline is copied from expectedMaxDeadline.
			expectedMaxDeadline: saturdayMidnightSydney.In(time.UTC),
		},
		{
			name:                   "TemplateRestartRequirementOverridesTemplateDefaultTTL",
			now:                    fridayEveningSydney.In(time.UTC),
			templateAllowAutostop:  true,
			templateDefaultTTL:     3 * time.Hour,
			userQuietHoursSchedule: sydneyQuietHours,
			templateRestartRequirement: schedule.TemplateRestartRequirement{
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
			// we allow due to our 1h leeway logic.
			now:                    time.Date(2023, 1, 1, 22, 59, 59, 0, sydneyLoc),
			templateAllowAutostop:  true,
			templateDefaultTTL:     0,
			userQuietHoursSchedule: sydneyQuietHours,
			templateRestartRequirement: schedule.TemplateRestartRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      2,          // every fortnight
			},
			workspaceTTL: 0,
			errContains:  "coder server system clock is incorrect",
		},
		{
			name:                   "RestartRequirementIgnoresMaxTTL",
			now:                    fridayEveningSydney.In(time.UTC),
			templateAllowAutostop:  false,
			templateDefaultTTL:     0,
			useMaxTTL:              false,
			templateMaxTTL:         time.Hour, // should be ignored
			userQuietHoursSchedule: sydneyQuietHours,
			templateRestartRequirement: schedule.TemplateRestartRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      0,          // weekly
			},
			workspaceTTL: 0,
			// expectedDeadline is copied from expectedMaxDeadline.
			expectedMaxDeadline: saturdayMidnightSydney.In(time.UTC),
		},
		{
			name:                   "MaxTTLIgnoresRestartRequirement",
			now:                    fridayEveningSydney.In(time.UTC),
			templateAllowAutostop:  false,
			templateDefaultTTL:     0,
			useMaxTTL:              true,
			templateMaxTTL:         time.Hour, // should NOT be ignored
			userQuietHoursSchedule: sydneyQuietHours,
			templateRestartRequirement: schedule.TemplateRestartRequirement{
				DaysOfWeek: 0b00100000, // Saturday
				Weeks:      0,          // weekly
			},
			workspaceTTL: 0,
			// expectedDeadline is copied from expectedMaxDeadline.
			expectedMaxDeadline: fridayEveningSydney.Add(time.Hour).In(time.UTC),
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
						UserAutostartEnabled:  false,
						UserAutostopEnabled:   c.templateAllowAutostop,
						DefaultTTL:            c.templateDefaultTTL,
						MaxTTL:                c.templateMaxTTL,
						UseRestartRequirement: !c.useMaxTTL,
						RestartRequirement:    c.templateRestartRequirement,
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

					sched, err := schedule.Daily(c.userQuietHoursSchedule)
					if !assert.NoError(t, err) {
						return schedule.UserQuietHoursScheduleOptions{}, err
					}

					return schedule.UserQuietHoursScheduleOptions{
						Schedule: sched,
						UserSet:  false,
						Duration: 4 * time.Hour,
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
			template, err := db.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
				ID:                           template.ID,
				UpdatedAt:                    database.Now(),
				AllowUserAutostart:           c.templateAllowAutostop,
				RestartRequirementDaysOfWeek: int16(c.templateRestartRequirement.DaysOfWeek),
				RestartRequirementWeeks:      c.templateRestartRequirement.Weeks,
			})
			require.NoError(t, err)
			workspaceTTL := sql.NullInt64{}
			if c.workspaceTTL != 0 {
				workspaceTTL = sql.NullInt64{
					Int64: int64(c.workspaceTTL),
					Valid: true,
				}
			}
			workspace := dbgen.Workspace(t, db, database.Workspace{
				TemplateID:     template.ID,
				OrganizationID: org.ID,
				OwnerID:        user.ID,
				Ttl:            workspaceTTL,
			})

			autostop, err := schedule.CalculateAutostop(ctx, schedule.CalculateAutostopParams{
				Database:                    db,
				TemplateScheduleStore:       templateScheduleStore,
				UserQuietHoursScheduleStore: userQuietHoursScheduleStore,
				Now:                         c.now,
				Workspace:                   workspace,
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
