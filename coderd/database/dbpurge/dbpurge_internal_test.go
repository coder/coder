package dbpurge

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestDBPurgeAuthorization(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	rawDB, _ := dbtestutil.NewDB(t)

	authz := rbac.NewAuthorizer(prometheus.NewRegistry())
	db := dbauthz.New(rawDB, authz, testutil.Logger(t), coderdtest.AccessControlStorePointer())

	ctx = dbauthz.AsDBPurge(ctx)

	clk := quartz.NewMock(t)
	now := time.Date(2025, 1, 15, 7, 30, 0, 0, time.UTC)
	clk.Set(now)

	vals := &codersdk.DeploymentValues{ /* same vals as before */ }

	inst := &instance{
		logger: testutil.Logger(t),
		vals:   vals,
		clk:    clk,
		// metrics can be nil in this test
	}

	err := inst.purgeTick(ctx, db, now)
	require.NoError(t, err)
}
