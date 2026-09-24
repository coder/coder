package usage_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
)

func TestEntitlementsUsagePublishing(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name            string
		failureAge      time.Duration
		recordPublished bool
		wantWarning     bool
	}{
		{name: "UsagePublishingEnabled"},
		{name: "UsagePublishingRecentFailure", failureAge: license.UsagePublishingFailureThreshold - time.Minute},
		{name: "UsagePublishingSustainedFailure", failureAge: license.UsagePublishingFailureThreshold + time.Minute, wantWarning: true},
		{name: "UsagePublishingLastPublished", recordPublished: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			adminClient, _, api, _ := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
				DontAddLicense: true,
			})
			now := dbtime.Now().Truncate(time.Second)
			coderdenttest.AddLicense(t, adminClient, coderdenttest.LicenseOptions{
				PublishUsageData: true,
			})
			if tc.failureAge > 0 {
				api.UsagePublishHealth.RecordCycleFailureForTest(now.Add(-tc.failureAge))
			}
			if tc.recordPublished {
				api.UsagePublishHealth.RecordCyclePublishedForTest(now)
			}
			require.NoError(t, api.AGPL.RefreshEntitlements(context.Background()))

			res, err := adminClient.Entitlements(context.Background())
			require.NoError(t, err)
			require.True(t, res.UsagePublishing.PublishingEnabled)
			if tc.recordPublished {
				require.NotNil(t, res.UsagePublishing.LastPublishedAt)
				require.WithinDuration(t, now, *res.UsagePublishing.LastPublishedAt, time.Second)
			} else {
				require.Nil(t, res.UsagePublishing.LastPublishedAt)
			}
			if tc.wantWarning {
				require.NotNil(t, res.UsagePublishing.FailingSince)
				require.WithinDuration(t, now.Add(-tc.failureAge), *res.UsagePublishing.FailingSince, time.Second)
				require.Contains(t, res.Warnings, codersdk.LicenseUsagePublishingFailingWarningText)
			} else {
				require.Nil(t, res.UsagePublishing.FailingSince)
				require.NotContains(t, res.Warnings, codersdk.LicenseUsagePublishingFailingWarningText)
			}
		})
	}

	t.Run("UsagePublishingDisabledResetsHealth", func(t *testing.T) {
		t.Parallel()
		adminClient, _, api, _ := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			DontAddLicense: true,
		})
		api.UsagePublishHealth.RecordCyclePublishedForTest(dbtime.Now())
		api.UsagePublishHealth.RecordCycleFailureForTest(dbtime.Now().Add(-license.UsagePublishingFailureThreshold))

		coderdenttest.AddLicense(t, adminClient, coderdenttest.LicenseOptions{})
		res, err := adminClient.Entitlements(context.Background())
		require.NoError(t, err)
		require.False(t, res.UsagePublishing.PublishingEnabled)
		require.Nil(t, res.UsagePublishing.LastPublishedAt)
		require.Nil(t, res.UsagePublishing.FailingSince)
		snapshot := api.UsagePublishHealth.Snapshot()
		require.True(t, snapshot.LastPublishedAt.IsZero())
		require.True(t, snapshot.FailureStartedAt.IsZero())

		coderdenttest.AddLicense(t, adminClient, coderdenttest.LicenseOptions{PublishUsageData: true})
		res, err = adminClient.Entitlements(context.Background())
		require.NoError(t, err)
		require.True(t, res.UsagePublishing.PublishingEnabled)
		require.Nil(t, res.UsagePublishing.LastPublishedAt)
		require.Nil(t, res.UsagePublishing.FailingSince)
	})
}
