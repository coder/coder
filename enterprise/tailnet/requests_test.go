package tailnet_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/enterprise/tailnet"
	"github.com/coder/coder/v2/tailnet/test"
	"github.com/coder/coder/v2/testutil"
)

func TestPGCoordinatorInvalidRequests(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	store, ps := dbtestutil.NewDB(t)
	coordinator, err := tailnet.NewPGCoord(ctx, slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), ps, store)
	require.NoError(t, err)
	t.Cleanup(func() { _ = coordinator.Close() })
	test.InvalidCoordinateRequestTest(ctx, t, coordinator)
}

func TestPGCoordinatorRequestPanic(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	store, ps := dbtestutil.NewDB(t)
	coordinator, err := tailnet.NewPGCoord(ctx, slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), ps, store)
	require.NoError(t, err)
	t.Cleanup(func() { _ = coordinator.Close() })
	test.CoordinateRequestPanicTest(ctx, t, coordinator)
}
