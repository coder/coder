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

func TestTokenCRUD(t *testing.T) {
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
	// expires_at should default to 30 days
	require.Greater(t, keys[0].ExpiresAt, time.Now().Add(time.Hour*29*24))
	require.Less(t, keys[0].ExpiresAt, time.Now().Add(time.Hour*31*24))
	require.Equal(t, codersdk.APIKeyScopeAll, keys[0].Scope)

	// no update

	err = client.DeleteAPIKey(ctx, codersdk.Me, keys[0].ID)
	require.NoError(t, err)
	keys, err = client.GetTokens(ctx, codersdk.Me)
	require.NoError(t, err)
	require.Empty(t, keys)
}

func TestTokenScoped(t *testing.T) {
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
	require.Equal(t, keys[0].Scope, codersdk.APIKeyScopeApplicationConnect)
}

func TestTokenDuration(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	_, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		Lifetime: time.Hour * 24 * 7,
	})
	require.NoError(t, err)
	keys, err := client.GetTokens(ctx, codersdk.Me)
	require.NoError(t, err)
	require.Greater(t, keys[0].ExpiresAt, time.Now().Add(time.Hour*6*24))
	require.Less(t, keys[0].ExpiresAt, time.Now().Add(time.Hour*8*24))
}

func TestTokenMaxLifetime(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	dc := coderdtest.DeploymentConfig(t)
	dc.MaxTokenLifetime.Value = time.Hour * 24 * 7
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentConfig: dc,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	// success
	_, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		Lifetime: time.Hour * 24 * 6,
	})
	require.NoError(t, err)

	// fail
	_, err = client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		Lifetime: time.Hour * 24 * 8,
	})
	require.ErrorContains(t, err, "lifetime must be less")
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
