package coderd_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestUserQuietHours(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		defaultQuietHoursSchedule := "CRON_TZ=America/Chicago 0 1 * * *"
		defaultScheduleParsed, err := cron.Daily(defaultQuietHoursSchedule)
		require.NoError(t, err)
		nextTime := defaultScheduleParsed.Next(time.Now().In(defaultScheduleParsed.Location()))
		if time.Until(nextTime) < time.Hour {
			// Use a different default schedule instead, because we want to avoid
			// the schedule "ticking over" during this test run.
			defaultQuietHoursSchedule = "CRON_TZ=America/Chicago 0 13 * * *"
			defaultScheduleParsed, err = cron.Daily(defaultQuietHoursSchedule)
			require.NoError(t, err)
		}

		dv := coderdtest.DeploymentValues(t)
		dv.UserQuietHoursSchedule.DefaultSchedule.Set(defaultQuietHoursSchedule)
		dv.Experiments.Set(string(codersdk.ExperimentTemplateAutostopRequirement))

		adminClient, adminUser := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAdvancedTemplateScheduling:  1,
					codersdk.FeatureTemplateAutostopRequirement: 1,
				},
			},
		})

		// Do it with another user to make sure that we're not hitting RBAC
		// errors.
		client, user := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)

		// Get quiet hours for a user that doesn't have them set.
		ctx := testutil.Context(t, testutil.WaitLong)
		sched1, err := client.UserQuietHoursSchedule(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Equal(t, defaultScheduleParsed.String(), sched1.RawSchedule)
		require.False(t, sched1.UserSet)
		require.Equal(t, defaultScheduleParsed.TimeParsed().Format("15:40"), sched1.Time)
		require.Equal(t, defaultScheduleParsed.Location().String(), sched1.Timezone)
		require.WithinDuration(t, defaultScheduleParsed.Next(time.Now()), sched1.Next, 15*time.Second)

		// Set their quiet hours.
		customQuietHoursSchedule := "CRON_TZ=Australia/Sydney 0 0 * * *"
		customScheduleParsed, err := cron.Daily(customQuietHoursSchedule)
		require.NoError(t, err)
		nextTime = customScheduleParsed.Next(time.Now().In(customScheduleParsed.Location()))
		if time.Until(nextTime) < time.Hour {
			// Use a different default schedule instead, because we want to avoid
			// the schedule "ticking over" during this test run.
			customQuietHoursSchedule = "CRON_TZ=Australia/Sydney 0 12 * * *"
			customScheduleParsed, err = cron.Daily(customQuietHoursSchedule)
			require.NoError(t, err)
		}

		sched2, err := client.UpdateUserQuietHoursSchedule(ctx, user.ID.String(), codersdk.UpdateUserQuietHoursScheduleRequest{
			Schedule: customQuietHoursSchedule,
		})
		require.NoError(t, err)
		require.Equal(t, customScheduleParsed.String(), sched2.RawSchedule)
		require.True(t, sched2.UserSet)
		require.Equal(t, customScheduleParsed.TimeParsed().Format("15:40"), sched2.Time)
		require.Equal(t, customScheduleParsed.Location().String(), sched2.Timezone)
		require.WithinDuration(t, customScheduleParsed.Next(time.Now()), sched2.Next, 15*time.Second)

		// Get quiet hours for a user that has them set.
		sched3, err := client.UserQuietHoursSchedule(ctx, user.ID.String())
		require.NoError(t, err)
		require.Equal(t, customScheduleParsed.String(), sched3.RawSchedule)
		require.True(t, sched3.UserSet)
		require.Equal(t, customScheduleParsed.TimeParsed().Format("15:40"), sched3.Time)
		require.Equal(t, customScheduleParsed.Location().String(), sched3.Timezone)
		require.WithinDuration(t, customScheduleParsed.Next(time.Now()), sched3.Next, 15*time.Second)

		// Try setting a garbage schedule.
		_, err = client.UpdateUserQuietHoursSchedule(ctx, user.ID.String(), codersdk.UpdateUserQuietHoursScheduleRequest{
			Schedule: "garbage",
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "parse daily schedule")

		// Try setting a non-daily schedule.
		_, err = client.UpdateUserQuietHoursSchedule(ctx, user.ID.String(), codersdk.UpdateUserQuietHoursScheduleRequest{
			Schedule: "CRON_TZ=America/Chicago 0 0 * * 1",
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "parse daily schedule")

		// Try setting a schedule with a timezone that doesn't exist.
		_, err = client.UpdateUserQuietHoursSchedule(ctx, user.ID.String(), codersdk.UpdateUserQuietHoursScheduleRequest{
			Schedule: "CRON_TZ=Deans/House 0 0 * * *",
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "parse daily schedule")

		// Try setting a schedule with more than one time.
		_, err = client.UpdateUserQuietHoursSchedule(ctx, user.ID.String(), codersdk.UpdateUserQuietHoursScheduleRequest{
			Schedule: "CRON_TZ=America/Chicago 0 0,12 * * *",
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "more than one time")
		_, err = client.UpdateUserQuietHoursSchedule(ctx, user.ID.String(), codersdk.UpdateUserQuietHoursScheduleRequest{
			Schedule: "CRON_TZ=America/Chicago 0-30 0 * * *",
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "more than one time")

		// We don't allow unsetting the custom schedule so we don't need to worry
		// about it in this test.
	})

	t.Run("NotEntitled", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.UserQuietHoursSchedule.DefaultSchedule.Set("CRON_TZ=America/Chicago 0 0 * * *")
		dv.Experiments.Set(string(codersdk.ExperimentTemplateAutostopRequirement))

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAdvancedTemplateScheduling: 1,
					// Not entitled.
					// codersdk.FeatureTemplateAutostopRequirement: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := client.UserQuietHoursSchedule(ctx, user.UserID.String())
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})

	t.Run("NotEnabled", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.UserQuietHoursSchedule.DefaultSchedule.Set("")
		dv.Experiments.Set(string(codersdk.ExperimentTemplateAutostopRequirement))

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			NoDefaultQuietHoursSchedule: true,
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAdvancedTemplateScheduling:  1,
					codersdk.FeatureTemplateAutostopRequirement: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := client.UserQuietHoursSchedule(ctx, user.UserID.String())
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})

	t.Run("NoFeatureFlag", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.UserQuietHoursSchedule.DefaultSchedule.Set("CRON_TZ=America/Chicago 0 0 * * *")
		dv.UserQuietHoursSchedule.DefaultSchedule.Set("")

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAdvancedTemplateScheduling:  1,
					codersdk.FeatureTemplateAutostopRequirement: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := client.UserQuietHoursSchedule(ctx, user.UserID.String())
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}
