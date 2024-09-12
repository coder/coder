package reports

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/quartz"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/testutil"
)

func TestReportFailedWorkspaceBuilds(t *testing.T) {
	t.Parallel()

	t.Run("InitialState_NoBuilds_NoReport", func(t *testing.T) {
		t.Parallel()

		// Setup
		logger, db, notifEnq, clk := setup(t)
		// nolint:gocritic // reportFailedWorkspaceBuilds is called by system.
		ctx := dbauthz.AsSystemRestricted(context.Background())

		// When
		err := reportFailedWorkspaceBuilds(ctx, logger, db, notifEnq, clk)

		// Then
		require.NoError(t, err)
	})

	t.Run("FailedBuilds_TemplateAdminOptIn_FirstRun_Report_SecondRunTooEarly_NoReport_ThirdRun_Report", func(t *testing.T) {
		t.Parallel()
		// TODO
	})

	t.Run("NoFailedBuilds_TemplateAdminIn_NoReport", func(t *testing.T) {
		t.Parallel()
		// TODO
	})

	t.Run("FailedBuilds_TemplateAdminOptOut_NoReport", func(t *testing.T) {
		t.Parallel()
		// TODO
	})

	t.Run("StaleFailedBuilds_TemplateAdminOptIn_NoReport_Cleanup", func(t *testing.T) {
		t.Parallel()
		// TODO
	})
}

func setup(t *testing.T) (slog.Logger, database.Store, notifications.Enqueuer, quartz.Clock) {
	t.Helper()

	logger := slogtest.Make(t, &slogtest.Options{})
	rdb, _ := dbtestutil.NewDB(t)
	db := dbauthz.New(rdb, rbac.NewAuthorizer(prometheus.NewRegistry()), logger, coderdtest.AccessControlStorePointer())
	notifyEnq := &testutil.FakeNotificationsEnqueuer{}
	clk := quartz.NewMock(t)
	return logger, db, notifyEnq, clk
}
