package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestPremiumFunnelEvent(t *testing.T) {
	t.Parallel()

	t.Run("CTAClick", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		fTelemetry := newFakeTelemetryReporter(ctx, t, 10)
		client := coderdtest.New(t, &coderdtest.Options{
			TelemetryReporter: fTelemetry,
		})
		_ = coderdtest.CreateFirstUser(t, client)

		eventID := uuid.New()
		err := client.ReportPremiumFunnelEvent(ctx, codersdk.PremiumFunnelEventRequest{
			ID:      eventID,
			Source:  codersdk.PremiumFunnelSourceAuditLog,
			Variant: codersdk.PremiumFunnelVariantAIGovernance,
		})
		require.NoError(t, err)

		event := receivePremiumFunnelEvent(ctx, t, fTelemetry)
		require.Equal(t, eventID, event.ID)
		require.Equal(t, telemetry.PremiumFunnelEventCTAClick, event.EventType)
		require.Equal(t, string(codersdk.PremiumFunnelSourceAuditLog), event.Source)
		require.Equal(t, string(codersdk.PremiumFunnelVariantAIGovernance), event.Variant)
		// Only trial signups carry an attribution.
		require.Equal(t, uuid.Nil, event.AttributionID)
		require.NotEqual(t, uuid.Nil, event.UserID)
		require.False(t, event.CreatedAt.IsZero())
	})

	t.Run("UnknownSource", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		err := client.ReportPremiumFunnelEvent(ctx, codersdk.PremiumFunnelEventRequest{
			ID:      uuid.New(),
			Source:  "/organizations/acme-corp/idp-sync",
			Variant: codersdk.PremiumFunnelVariantPremium,
		})
		require.Error(t, err)

		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "source", sdkErr.Validations[0].Field)
	})

	t.Run("MemberCannotReport", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		client := coderdtest.New(t, nil)
		admin := coderdtest.CreateFirstUser(t, client)
		memberClient, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)

		err := memberClient.ReportPremiumFunnelEvent(ctx, codersdk.PremiumFunnelEventRequest{
			ID:      uuid.New(),
			Source:  codersdk.PremiumFunnelSourceAppearance,
			Variant: codersdk.PremiumFunnelVariantPremium,
		})
		require.Error(t, err)

		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}

// receivePremiumFunnelEvent drains snapshots until one carries a funnel event.
// Creating the first user reports its own snapshots on the same channel.
func receivePremiumFunnelEvent(ctx context.Context, t *testing.T, reporter *fakeTelemetryReporter) telemetry.PremiumFunnelEvent {
	t.Helper()

	for {
		snapshot := testutil.TryReceive(ctx, t, reporter.snapshots)
		if len(snapshot.PremiumFunnelEvents) > 0 {
			require.Len(t, snapshot.PremiumFunnelEvents, 1)
			return snapshot.PremiumFunnelEvents[0]
		}
	}
}
