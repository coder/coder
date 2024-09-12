package reports

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestReportFailedWorkspaceBuilds(t *testing.T) {
	t.Parallel()

	t.Run("FailedBuilds_TemplateAdminOptIn_FirstRun_Report_SecondRunTooEarly_NoReport_ThirdRun_Report", func(t *testing.T) {
		t.Parallel()

		// Prepare dependencies
		logger := slogtest.Make(t, &slogtest.Options{})
		rdb, _ := dbtestutil.NewDB(t)
		db := dbauthz.New(rdb, rbac.NewAuthorizer(prometheus.NewRegistry()), logger, coderdtest.AccessControlStorePointer())
		notifyEnq := &testutil.FakeNotificationsEnqueuer{}
		clk := quartz.NewMock(t)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancel()

		// Given

		// When
		err := reportFailedWorkspaceBuilds(ctx, logger, db, notifyEnq, clk)
		require.NoError(t, err)

		// Then

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
