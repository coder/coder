package enidpsync_test

import (
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/entitlements"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/enterprise/coderd/enidpsync"
	"github.com/coder/coder/v2/testutil"
)

func TestEnterpriseParseGroupClaims(t *testing.T) {
	t.Parallel()

	t.Run("NoEntitlements", func(t *testing.T) {
		t.Parallel()

		s := enidpsync.NewSync(slogtest.Make(t, &slogtest.Options{}),
			runtimeconfig.NewNoopManager(),
			entitlements.New(),
			idpsync.DeploymentSyncSettings{})

		ctx := testutil.Context(t, testutil.WaitMedium)

		params, err := s.ParseGroupClaims(ctx, jwt.MapClaims{})
		require.Nil(t, err)

		require.False(t, params.SyncEnabled)
	})
}
