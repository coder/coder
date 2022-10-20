package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

func TestTokens(t *testing.T) {
	t.Parallel()

	t.Run("CRUD", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		keys, err := client.GetTokens(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Empty(t, keys)

		res, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{})
		require.NoError(t, err)
		require.Greater(t, len(res.Key), 2)

		keys, err = client.GetTokens(ctx, codersdk.Me)
		require.NoError(t, err)
		require.EqualValues(t, len(keys), 1)
		require.Contains(t, res.Key, keys[0].ID)
		// expires_at must be greater than 50 years
		require.Greater(t, keys[0].ExpiresAt, time.Now().Add(time.Hour*438300))
		require.Equal(t, codersdk.APIKeyScopeAll, keys[0].Scope)

		// no update

		err = client.DeleteAPIKey(ctx, codersdk.Me, keys[0].ID)
		require.NoError(t, err)
		keys, err = client.GetTokens(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Empty(t, keys)
	})

	t.Run("Scoped", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		res, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
			Scope: codersdk.APIKeyScopeApplicationConnect,
		})
		require.NoError(t, err)
		require.Greater(t, len(res.Key), 2)

		keys, err := client.GetTokens(ctx, codersdk.Me)
		require.NoError(t, err)
		require.EqualValues(t, len(keys), 1)
		require.Contains(t, res.Key, keys[0].ID)
		// expires_at must be greater than 50 years
		require.Greater(t, keys[0].ExpiresAt, time.Now().Add(time.Hour*438300))
		require.Equal(t, keys[0].Scope, codersdk.APIKeyScopeApplicationConnect)
	})
}

func TestAPIKey(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	_ = coderdtest.CreateFirstUser(t, client)

	res, err := client.CreateAPIKey(ctx, codersdk.Me)
	require.NoError(t, err)
	require.Greater(t, len(res.Key), 2)
}
