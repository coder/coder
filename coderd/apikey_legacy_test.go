package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestLegacyTokenCompatibility(t *testing.T) {
	t.Parallel()

	t.Run("exposes legacy scope defaults", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		first := coderdtest.CreateFirstUser(t, client)
		legacyKey, _ := coderdtest.LegacyToken(t, db, first.UserID)

		ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitShort)
		defer cancel()

		keys, err := client.Tokens(ctx, codersdk.Me, codersdk.TokensFilter{})
		require.NoErrorf(t, err, "tokens request failed: %v", err)
		require.Len(t, keys, 1)

		key := keys[0]
		require.Equal(t, legacyKey.TokenName, key.TokenName)
		require.Equal(t, codersdk.APIKeyScopeAll, key.Scope)
		require.Contains(t, key.Scopes, codersdk.APIKeyScopeCoderAll)
		require.Len(t, key.AllowList, 1)
		require.Equal(t, "*:*", key.AllowList[0].String())
	})

	t.Run("update via legacy scope field", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		first := coderdtest.CreateFirstUser(t, client)
		legacyKey, _ := coderdtest.LegacyToken(t, db, first.UserID)

		ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitShort)
		defer cancel()

		_, err := client.UpdateToken(ctx, codersdk.Me, legacyKey.TokenName, codersdk.UpdateTokenRequest{
			Scope: ptr.Ref(codersdk.APIKeyScopeApplicationConnect),
		})
		require.NoErrorf(t, err, "update token failed: %v", err)

		refreshed, err := client.APIKeyByName(ctx, codersdk.Me, legacyKey.TokenName)
		require.NoErrorf(t, err, "fetch token failed: %v", err)
		require.Equal(t, codersdk.APIKeyScopeApplicationConnect, refreshed.Scope)
		require.Contains(t, refreshed.Scopes, codersdk.APIKeyScopeCoderApplicationConnect)
	})
}
