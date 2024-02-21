package coderd_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func TestTokenCRUD(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	auditor := audit.NewMock()
	numLogs := len(auditor.AuditLogs())
	client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
	_ = coderdtest.CreateFirstUser(t, client)
	numLogs++ // add an audit log for user creation

	keys, err := client.Tokens(ctx, codersdk.Me, codersdk.TokensFilter{})
	require.NoError(t, err)
	require.Empty(t, keys)

	res, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{})
	require.NoError(t, err)
	require.Greater(t, len(res.Key), 2)
	numLogs++ // add an audit log for token creation

	keys, err = client.Tokens(ctx, codersdk.Me, codersdk.TokensFilter{})
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
	numLogs++ // add an audit log for token deletion
	keys, err = client.Tokens(ctx, codersdk.Me, codersdk.TokensFilter{})
	require.NoError(t, err)
	require.Empty(t, keys)

	// ensure audit log count is correct
	require.Len(t, auditor.AuditLogs(), numLogs)
	require.Equal(t, database.AuditActionCreate, auditor.AuditLogs()[numLogs-2].Action)
	require.Equal(t, database.AuditActionDelete, auditor.AuditLogs()[numLogs-1].Action)
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

	keys, err := client.Tokens(ctx, codersdk.Me, codersdk.TokensFilter{})
	require.NoError(t, err)
	require.EqualValues(t, len(keys), 1)
	require.Contains(t, res.Key, keys[0].ID)
	require.Equal(t, keys[0].Scope, codersdk.APIKeyScopeApplicationConnect)
}

func TestUserSetTokenDuration(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	_, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		Lifetime: time.Hour * 24 * 7,
	})
	require.NoError(t, err)
	keys, err := client.Tokens(ctx, codersdk.Me, codersdk.TokensFilter{})
	require.NoError(t, err)
	require.Greater(t, keys[0].ExpiresAt, time.Now().Add(time.Hour*6*24))
	require.Less(t, keys[0].ExpiresAt, time.Now().Add(time.Hour*8*24))
}

func TestDefaultTokenDuration(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	_, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{})
	require.NoError(t, err)
	keys, err := client.Tokens(ctx, codersdk.Me, codersdk.TokensFilter{})
	require.NoError(t, err)
	require.Greater(t, keys[0].ExpiresAt, time.Now().Add(time.Hour*29*24))
	require.Less(t, keys[0].ExpiresAt, time.Now().Add(time.Hour*31*24))
}

func TestTokenUserSetMaxLifetime(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	dc := coderdtest.DeploymentValues(t)
	dc.MaxTokenLifetime = serpent.Duration(time.Hour * 24 * 7)
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: dc,
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

func TestSessionExpiry(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	dc := coderdtest.DeploymentValues(t)

	db, pubsub := dbtestutil.NewDB(t)
	adminClient := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: dc,
		Database:         db,
		Pubsub:           pubsub,
	})
	adminUser := coderdtest.CreateFirstUser(t, adminClient)

	// This is a hack, but we need the admin account to have a long expiry
	// otherwise the test will flake, so we only update the expiry config after
	// the admin account has been created.
	//
	// We don't support updating the deployment config after startup, but for
	// this test it works because we don't copy the value (and we use pointers).
	dc.SessionDuration = serpent.Duration(time.Second)

	userClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)

	// Find the session cookie, and ensure it has the correct expiry.
	token := userClient.SessionToken()
	apiKey, err := db.GetAPIKeyByID(ctx, strings.Split(token, "-")[0])
	require.NoError(t, err)

	require.EqualValues(t, dc.SessionDuration.Value().Seconds(), apiKey.LifetimeSeconds)
	require.WithinDuration(t, apiKey.CreatedAt.Add(dc.SessionDuration.Value()), apiKey.ExpiresAt, 2*time.Second)

	// Update the session token to be expired so we can test that it is
	// rejected for extra points.
	err = db.UpdateAPIKeyByID(ctx, database.UpdateAPIKeyByIDParams{
		ID:        apiKey.ID,
		LastUsed:  apiKey.LastUsed,
		ExpiresAt: dbtime.Now().Add(-time.Hour),
		IPAddress: apiKey.IPAddress,
	})
	require.NoError(t, err)

	_, err = userClient.User(ctx, codersdk.Me)
	require.Error(t, err)
	var sdkErr *codersdk.Error
	if assert.ErrorAs(t, err, &sdkErr) {
		require.Equal(t, http.StatusUnauthorized, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "session has expired")
	}
}

func TestAPIKey_OK(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	_ = coderdtest.CreateFirstUser(t, client)

	res, err := client.CreateAPIKey(ctx, codersdk.Me)
	require.NoError(t, err)
	require.Greater(t, len(res.Key), 2)
}

func TestAPIKey_Deleted(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)
	_, anotherUser := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
	require.NoError(t, client.DeleteUser(context.Background(), anotherUser.ID))

	// Attempt to create an API key for the deleted user. This should fail.
	_, err := client.CreateAPIKey(ctx, anotherUser.Username)
	require.Error(t, err)
	var apiErr *codersdk.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
}
