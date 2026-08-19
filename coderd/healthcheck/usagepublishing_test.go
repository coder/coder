package healthcheck_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/entitlements"
	"github.com/coder/coder/v2/coderd/healthcheck"
	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/codersdk"
)

func TestUsagePublishing(t *testing.T) {
	t.Parallel()

	set := func(status codersdk.UsagePublishingStatus) *entitlements.Set {
		set := entitlements.New()
		set.Modify(func(entitlements *codersdk.Entitlements) {
			entitlements.UsagePublishing = status
		})
		return set
	}

	t.Run("NilEntitlements", func(t *testing.T) {
		t.Parallel()

		var report healthcheck.UsagePublishingReport
		report.Run(context.Background(), &healthcheck.UsagePublishingReportOptions{})

		require.True(t, report.Healthy)
		require.Equal(t, health.SeverityOK, report.Severity)
		require.Empty(t, report.Warnings)
		require.False(t, report.PublishingEnabled)
		require.Nil(t, report.LastPublishedAt)
		require.Nil(t, report.FailingSince)
	})

	t.Run("Disabled", func(t *testing.T) {
		t.Parallel()

		var report healthcheck.UsagePublishingReport
		report.Run(context.Background(), &healthcheck.UsagePublishingReportOptions{
			Entitlements: set(codersdk.UsagePublishingStatus{
				PublishingEnabled: false,
			}),
		})

		require.True(t, report.Healthy)
		require.Equal(t, health.SeverityOK, report.Severity)
		require.Empty(t, report.Warnings)
		require.False(t, report.PublishingEnabled)
		require.Nil(t, report.LastPublishedAt)
		require.Nil(t, report.FailingSince)
	})

	t.Run("StatusUnavailable", func(t *testing.T) {
		t.Parallel()

		// A failed status query must not present the empty status as
		// healthy; that would erase an active failure warning.
		var report healthcheck.UsagePublishingReport
		report.Run(context.Background(), &healthcheck.UsagePublishingReportOptions{
			Entitlements: set(codersdk.UsagePublishingStatus{
				PublishingEnabled: true,
				StatusUnavailable: true,
			}),
		})

		require.Equal(t, health.SeverityWarning, report.Severity)
		require.Len(t, report.Warnings, 1)
		require.Equal(t, health.CodeUnknown, report.Warnings[0].Code)
		require.True(t, report.StatusUnavailable)
	})

	t.Run("Healthy", func(t *testing.T) {
		t.Parallel()

		lastPublishedAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		var report healthcheck.UsagePublishingReport
		report.Run(context.Background(), &healthcheck.UsagePublishingReportOptions{
			Entitlements: set(codersdk.UsagePublishingStatus{
				PublishingEnabled: true,
				LastPublishedAt:   &lastPublishedAt,
			}),
		})

		require.True(t, report.Healthy)
		require.Equal(t, health.SeverityOK, report.Severity)
		require.Empty(t, report.Warnings)
		require.True(t, report.PublishingEnabled)
		require.Equal(t, &lastPublishedAt, report.LastPublishedAt)
		require.Nil(t, report.FailingSince)
	})

	t.Run("Failing", func(t *testing.T) {
		t.Parallel()

		failingSince := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		var report healthcheck.UsagePublishingReport
		report.Run(context.Background(), &healthcheck.UsagePublishingReportOptions{
			Entitlements: set(codersdk.UsagePublishingStatus{
				PublishingEnabled: true,
				FailingSince:      &failingSince,
			}),
		})

		require.True(t, report.Healthy)
		require.Equal(t, health.SeverityWarning, report.Severity)
		require.Len(t, report.Warnings, 1)
		require.Equal(t, health.CodeUsagePublishingFailing, report.Warnings[0].Code)
		require.True(t, report.PublishingEnabled)
		require.Equal(t, &failingSince, report.FailingSince)
	})
}
