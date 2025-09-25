package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
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
	require.Greater(t, keys[0].ExpiresAt, time.Now().Add(time.Hour*24*6))
	require.Less(t, keys[0].ExpiresAt, time.Now().Add(time.Hour*24*8))
	require.Equal(t, codersdk.APIKeyScopeAll, keys[0].Scope)
	require.Len(t, keys[0].AllowList, 1)
	require.Equal(t, "*:*", keys[0].AllowList[0].String())

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
	require.Len(t, keys[0].AllowList, 1)
	require.Equal(t, "*:*", keys[0].AllowList[0].String())
}

// Ensure backward-compat: when a token is created using the legacy singular
// scope names ("all" or "application_connect"), the API returns the same
// legacy value in the deprecated singular Scope field while also supporting
// the new multi-scope field.
func TestTokenLegacySingularScopeCompat(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		scope  codersdk.APIKeyScope
		scopes []codersdk.APIKeyScope
	}{
		{
			name:   "all",
			scope:  codersdk.APIKeyScopeAll,
			scopes: []codersdk.APIKeyScope{codersdk.APIKeyScopeCoderAll},
		},
		{
			name:   "application_connect",
			scope:  codersdk.APIKeyScopeApplicationConnect,
			scopes: []codersdk.APIKeyScope{codersdk.APIKeyScopeCoderApplicationConnect},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			defer cancel()
			client := coderdtest.New(t, nil)
			_ = coderdtest.CreateFirstUser(t, client)

			// Create with legacy singular scope.
			_, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
				Scope: tc.scope,
			})
			require.NoError(t, err)

			// Read back and ensure the deprecated singular field matches exactly.
			keys, err := client.Tokens(ctx, codersdk.Me, codersdk.TokensFilter{})
			require.NoError(t, err)
			require.Len(t, keys, 1)
			require.Equal(t, tc.scope, keys[0].Scope)
			require.ElementsMatch(t, keys[0].Scopes, tc.scopes)
			require.Len(t, keys[0].AllowList, 1)
			require.Equal(t, "*:*", keys[0].AllowList[0].String())
		})
	}
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
	require.Greater(t, keys[0].ExpiresAt, time.Now().Add(time.Hour*24*6))
	require.Less(t, keys[0].ExpiresAt, time.Now().Add(time.Hour*24*8))
}

func TestTokenUserSetMaxLifetime(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	dc := coderdtest.DeploymentValues(t)
	dc.Sessions.MaximumTokenDuration = serpent.Duration(time.Hour * 24 * 7)
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

func TestTokenAdminSetMaxLifetime(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	dc := coderdtest.DeploymentValues(t)
	dc.Sessions.MaximumTokenDuration = serpent.Duration(time.Hour * 24 * 7)
	dc.Sessions.MaximumAdminTokenDuration = serpent.Duration(time.Hour * 24 * 14)
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: dc,
	})
	adminUser := coderdtest.CreateFirstUser(t, client)
	nonAdminClient, _ := coderdtest.CreateAnotherUser(t, client, adminUser.OrganizationID)

	// Admin should be able to create a token with a lifetime longer than the non-admin max.
	_, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		Lifetime: time.Hour * 24 * 10,
	})
	require.NoError(t, err)

	// Admin should NOT be able to create a token with a lifetime longer than the admin max.
	_, err = client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		Lifetime: time.Hour * 24 * 15,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "lifetime must be less")

	// Non-admin should NOT be able to create a token with a lifetime longer than the non-admin max.
	_, err = nonAdminClient.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		Lifetime: time.Hour * 24 * 8,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "lifetime must be less")

	// Non-admin should be able to create a token with a lifetime shorter than the non-admin max.
	_, err = nonAdminClient.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		Lifetime: time.Hour * 24 * 6,
	})
	require.NoError(t, err)
}

func TestTokenAdminSetMaxLifetimeShorter(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	dc := coderdtest.DeploymentValues(t)
	dc.Sessions.MaximumTokenDuration = serpent.Duration(time.Hour * 24 * 14)
	dc.Sessions.MaximumAdminTokenDuration = serpent.Duration(time.Hour * 24 * 7)
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: dc,
	})
	adminUser := coderdtest.CreateFirstUser(t, client)
	nonAdminClient, _ := coderdtest.CreateAnotherUser(t, client, adminUser.OrganizationID)

	// Admin should NOT be able to create a token with a lifetime longer than the admin max.
	_, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		Lifetime: time.Hour * 24 * 8,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "lifetime must be less")

	// Admin should be able to create a token with a lifetime shorter than the admin max.
	_, err = client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		Lifetime: time.Hour * 24 * 6,
	})
	require.NoError(t, err)

	// Non-admin should be able to create a token with a lifetime longer than the admin max.
	_, err = nonAdminClient.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		Lifetime: time.Hour * 24 * 10,
	})
	require.NoError(t, err)

	// Non-admin should NOT be able to create a token with a lifetime longer than the non-admin max.
	_, err = nonAdminClient.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		Lifetime: time.Hour * 24 * 15,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "lifetime must be less")
}

func TestTokenCustomDefaultLifetime(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	dc := coderdtest.DeploymentValues(t)
	dc.Sessions.DefaultTokenDuration = serpent.Duration(time.Hour * 12)
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: dc,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	_, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{})
	require.NoError(t, err)

	tokens, err := client.Tokens(ctx, codersdk.Me, codersdk.TokensFilter{})
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	require.EqualValues(t, dc.Sessions.DefaultTokenDuration.Value().Seconds(), tokens[0].LifetimeSeconds)
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
	dc.Sessions.DefaultDuration = serpent.Duration(time.Second)

	userClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)

	// Find the session cookie, and ensure it has the correct expiry.
	token := userClient.SessionToken()
	apiKey, err := db.GetAPIKeyByID(ctx, strings.Split(token, "-")[0])
	require.NoError(t, err)

	require.EqualValues(t, dc.Sessions.DefaultDuration.Value().Seconds(), apiKey.LifetimeSeconds)
	require.WithinDuration(t, apiKey.CreatedAt.Add(dc.Sessions.DefaultDuration.Value()), apiKey.ExpiresAt, 2*time.Second)

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

	// Given: a deployment with auditing enabled
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	auditor := audit.NewMock()
	client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
	owner := coderdtest.CreateFirstUser(t, client)
	auditor.ResetLogs()

	// When: an API key is created
	res, err := client.CreateAPIKey(ctx, codersdk.Me)
	require.NoError(t, err)
	require.Greater(t, len(res.Key), 2)

	// Then: an audit log is generated
	als := auditor.AuditLogs()
	require.Len(t, als, 1)
	al := als[0]
	assert.Equal(t, owner.UserID, al.UserID)
	assert.Equal(t, database.AuditActionCreate, al.Action)
	assert.Equal(t, database.ResourceTypeApiKey, al.ResourceType)

	// Then: the diff MUST NOT contain the generated key.
	raw, err := json.Marshal(al)
	require.NoError(t, err)
	require.NotContains(t, res.Key, string(raw))
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

func TestAPIKey_SetDefault(t *testing.T) {
	t.Parallel()

	db, pubsub := dbtestutil.NewDB(t)
	dc := coderdtest.DeploymentValues(t)
	dc.Sessions.DefaultTokenDuration = serpent.Duration(time.Hour * 12)
	client := coderdtest.New(t, &coderdtest.Options{
		Database:         db,
		Pubsub:           pubsub,
		DeploymentValues: dc,
	})
	owner := coderdtest.CreateFirstUser(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	token, err := client.CreateAPIKey(ctx, owner.UserID.String())
	require.NoError(t, err)
	split := strings.Split(token.Key, "-")
	apiKey1, err := db.GetAPIKeyByID(ctx, split[0])
	require.NoError(t, err)
	require.EqualValues(t, dc.Sessions.DefaultTokenDuration.Value().Seconds(), apiKey1.LifetimeSeconds)
}

func TestAPIKey_PrebuildsNotAllowed(t *testing.T) {
	t.Parallel()

	db, pubsub := dbtestutil.NewDB(t)
	dc := coderdtest.DeploymentValues(t)
	dc.Sessions.DefaultTokenDuration = serpent.Duration(time.Hour * 12)
	client := coderdtest.New(t, &coderdtest.Options{
		Database:         db,
		Pubsub:           pubsub,
		DeploymentValues: dc,
	})

	ctx := testutil.Context(t, testutil.WaitLong)

	// Given: an existing api token for the prebuilds user
	_, prebuildsToken := dbgen.APIKey(t, db, database.APIKey{
		UserID: database.PrebuildsSystemUserID,
	})
	client.SetSessionToken(prebuildsToken)

	// When: the prebuilds user tries to create an API key
	_, err := client.CreateAPIKey(ctx, database.PrebuildsSystemUserID.String())
	// Then: denied.
	require.ErrorContains(t, err, httpapi.ResourceForbiddenResponse.Message)

	// When: the prebuilds user tries to create a token
	_, err = client.CreateToken(ctx, database.PrebuildsSystemUserID.String(), codersdk.CreateTokenRequest{})
	// Then: also denied.
	require.ErrorContains(t, err, httpapi.ResourceForbiddenResponse.Message)
}
