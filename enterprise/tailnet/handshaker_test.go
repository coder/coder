package tailnet_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/enterprise/tailnet"
	agpltest "github.com/coder/coder/v2/tailnet/test"
	"github.com/coder/coder/v2/testutil"
)

func TestPGCoordinator_ReadyForHandshake_OK(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	coord1, err := tailnet.NewPGCoord(ctx, logger.Named("coord1"), ps, store)
	require.NoError(t, err)
	defer coord1.Close()

	agpltest.ReadyForHandshakeTest(ctx, t, coord1)
}

func TestPGCoordinator_ReadyForHandshake_NoPermission(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	coord1, err := tailnet.NewPGCoord(ctx, logger.Named("coord1"), ps, store)
	require.NoError(t, err)
	defer coord1.Close()

	agpltest.ReadyForHandshakeNoPermissionTest(ctx, t, coord1)
}
