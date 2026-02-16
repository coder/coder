package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestTokens(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	adminUser := coderdtest.CreateFirstUser(t, client)

	secondUserClient, secondUser := coderdtest.CreateAnotherUser(t, client, adminUser.OrganizationID)
	thirdUserClient, thirdUser := coderdtest.CreateAnotherUser(t, client, adminUser.OrganizationID)

	ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancelFunc()

	// helpful empty response
	inv, root := clitest.New(t, "tokens", "ls")
	//nolint:gocritic // This should be run as the owner user.
	clitest.SetupConfig(t, client, root)
	buf := new(bytes.Buffer)
	inv.Stdout = buf
	inv.Stderr = buf
	err := inv.WithContext(ctx).Run()
	require.NoError(t, err)
	res := buf.String()
	require.Contains(t, res, "tokens found")

	inv, root = clitest.New(t, "tokens", "create", "--name", "token-one")
	clitest.SetupConfig(t, client, root)
	buf = new(bytes.Buffer)
	inv.Stdout = buf
	err = inv.WithContext(ctx).Run()
	require.NoError(t, err)
	res = buf.String()
	require.NotEmpty(t, res)
	id := res[:10]

	allowWorkspaceID := uuid.New()
	allowSpec := fmt.Sprintf("workspace:%s", allowWorkspaceID.String())
	inv, root = clitest.New(t, "tokens", "create", "--name", "scoped-token", "--scope", string(codersdk.APIKeyScopeWorkspaceRead), "--allow", allowSpec)
	clitest.SetupConfig(t, client, root)
	buf = new(bytes.Buffer)
	inv.Stdout = buf
	err = inv.WithContext(ctx).Run()
	require.NoError(t, err)
	res = buf.String()
	require.NotEmpty(t, res)
	scopedTokenID := res[:10]

	// Test creating a token for second user from first user's (admin) session
	inv, root = clitest.New(t, "tokens", "create", "--name", "token-two", "--user", secondUser.ID.String())
	clitest.SetupConfig(t, client, root)
	buf = new(bytes.Buffer)
	inv.Stdout = buf
	err = inv.WithContext(ctx).Run()
	// Test should succeed in creating token for second user
	require.NoError(t, err)
	res = buf.String()
	require.NotEmpty(t, res)
	secondTokenID := res[:10]

	// Test listing tokens from the first user's (admin) session
	inv, root = clitest.New(t, "tokens", "ls")
	clitest.SetupConfig(t, client, root)
	buf = new(bytes.Buffer)
	inv.Stdout = buf
	err = inv.WithContext(ctx).Run()
	require.NoError(t, err)
	res = buf.String()
	require.NotEmpty(t, res)
	// Result should only contain the tokens created for the admin user
	require.Contains(t, res, "ID")
	require.Contains(t, res, "EXPIRES AT")
	require.Contains(t, res, "CREATED AT")
	require.Contains(t, res, "LAST USED")
	require.Contains(t, res, id)
	// Result should not contain the token created for the second user
	require.NotContains(t, res, secondTokenID)

	inv, root = clitest.New(t, "tokens", "view", "scoped-token")
	clitest.SetupConfig(t, client, root)
	buf = new(bytes.Buffer)
	inv.Stdout = buf
	err = inv.WithContext(ctx).Run()
	require.NoError(t, err)
	res = buf.String()
	require.Contains(t, res, string(codersdk.APIKeyScopeWorkspaceRead))
	require.Contains(t, res, allowSpec)

	// Test listing tokens from the second user's session
	inv, root = clitest.New(t, "tokens", "ls")
	clitest.SetupConfig(t, secondUserClient, root)
	buf = new(bytes.Buffer)
	inv.Stdout = buf
	err = inv.WithContext(ctx).Run()
	require.NoError(t, err)
	res = buf.String()
	require.NotEmpty(t, res)
	require.Contains(t, res, "ID")
	require.Contains(t, res, "EXPIRES AT")
	require.Contains(t, res, "CREATED AT")
	require.Contains(t, res, "LAST USED")
	// Result should contain the token created for the second user
	require.Contains(t, res, secondTokenID)

	// Test creating a token for third user from second user's (non-admin) session
	inv, root = clitest.New(t, "tokens", "create", "--name", "failed-token", "--user", thirdUser.ID.String())
	clitest.SetupConfig(t, secondUserClient, root)
	buf = new(bytes.Buffer)
	inv.Stdout = buf
	err = inv.WithContext(ctx).Run()
	// User (non-admin) should not be able to create a token for another user
	require.Error(t, err)

	inv, root = clitest.New(t, "tokens", "create", "--name", "invalid-allow", "--allow", "badvalue")
	clitest.SetupConfig(t, client, root)
	buf = new(bytes.Buffer)
	inv.Stdout = buf
	err = inv.WithContext(ctx).Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid allow_list entry")

	inv, root = clitest.New(t, "tokens", "ls", "--output=json")
	clitest.SetupConfig(t, client, root)
	buf = new(bytes.Buffer)
	inv.Stdout = buf
	err = inv.WithContext(ctx).Run()
	require.NoError(t, err)

	var tokens []codersdk.APIKey
	require.NoError(t, json.Unmarshal(buf.Bytes(), &tokens))
	require.Len(t, tokens, 2)
	tokenByName := make(map[string]codersdk.APIKey, len(tokens))
	for _, tk := range tokens {
		tokenByName[tk.TokenName] = tk
	}
	require.Contains(t, tokenByName, "token-one")
	require.Contains(t, tokenByName, "scoped-token")
	scopedToken := tokenByName["scoped-token"]
	require.Contains(t, scopedToken.Scopes, codersdk.APIKeyScopeWorkspaceRead)
	require.Len(t, scopedToken.AllowList, 1)
	require.Equal(t, allowSpec, scopedToken.AllowList[0].String())

	// Delete by name (default behavior is now expire)
	inv, root = clitest.New(t, "tokens", "rm", "token-one")
	clitest.SetupConfig(t, client, root)
	buf = new(bytes.Buffer)
	inv.Stdout = buf
	err = inv.WithContext(ctx).Run()
	require.NoError(t, err)
	res = buf.String()
	require.NotEmpty(t, res)
	require.Contains(t, res, "expired")

	// Regular users cannot expire other users' tokens (expire is default now).
	inv, root = clitest.New(t, "tokens", "rm", secondTokenID)
	clitest.SetupConfig(t, thirdUserClient, root)
	buf = new(bytes.Buffer)
	inv.Stdout = buf
	err = inv.WithContext(ctx).Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")

	// Only admin users can expire other users' tokens (expire is default now).
	inv, root = clitest.New(t, "tokens", "rm", secondTokenID)
	clitest.SetupConfig(t, client, root)
	buf = new(bytes.Buffer)
	inv.Stdout = buf
	err = inv.WithContext(ctx).Run()
	require.NoError(t, err)
	// Validate that token was expired
	if token, err := client.APIKeyByName(ctx, secondUser.ID.String(), "token-two"); assert.NoError(t, err) {
		require.True(t, token.ExpiresAt.Before(time.Now()))
	}

	// Delete by ID (explicit delete flag)
	inv, root = clitest.New(t, "tokens", "rm", "--delete", secondTokenID)
	clitest.SetupConfig(t, client, root)
	buf = new(bytes.Buffer)
	inv.Stdout = buf
	err = inv.WithContext(ctx).Run()
	require.NoError(t, err)
	res = buf.String()
	require.NotEmpty(t, res)
	require.Contains(t, res, "deleted")

	// Delete scoped token by ID (explicit delete flag)
	inv, root = clitest.New(t, "tokens", "rm", "--delete", scopedTokenID)
	clitest.SetupConfig(t, client, root)
	buf = new(bytes.Buffer)
	inv.Stdout = buf
	err = inv.WithContext(ctx).Run()
	require.NoError(t, err)
	res = buf.String()
	require.NotEmpty(t, res)
	require.Contains(t, res, "deleted")

	// Create third token
	inv, root = clitest.New(t, "tokens", "create", "--name", "token-three")
	clitest.SetupConfig(t, client, root)
	buf = new(bytes.Buffer)
	inv.Stdout = buf
	err = inv.WithContext(ctx).Run()
	require.NoError(t, err)
	res = buf.String()
	require.NotEmpty(t, res)
	fourthToken := res

	// Delete by token (explicit delete flag)
	inv, root = clitest.New(t, "tokens", "rm", "--delete", fourthToken)
	clitest.SetupConfig(t, client, root)
	buf = new(bytes.Buffer)
	inv.Stdout = buf
	err = inv.WithContext(ctx).Run()
	require.NoError(t, err)
	res = buf.String()
	require.NotEmpty(t, res)
	require.Contains(t, res, "deleted")
}

func TestTokensListExpiredFiltering(t *testing.T) {
	t.Parallel()

	client, _, api := coderdtest.NewWithAPI(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)

	// Create a valid (non-expired) token
	validToken, _ := dbgen.APIKey(t, api.Database, database.APIKey{
		UserID:    owner.UserID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		LoginType: database.LoginTypeToken,
		TokenName: "valid-token",
	})

	// Create an expired token
	expiredToken, _ := dbgen.APIKey(t, api.Database, database.APIKey{
		UserID:    owner.UserID,
		ExpiresAt: time.Now().Add(-24 * time.Hour),
		LoginType: database.LoginTypeToken,
		TokenName: "expired-token",
	})

	t.Run("HidesExpiredByDefault", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		inv, root := clitest.New(t, "tokens", "ls")
		clitest.SetupConfig(t, client, root)
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		res := buf.String()
		require.Contains(t, res, validToken.ID)
		require.Contains(t, res, "valid-token")
		require.NotContains(t, res, expiredToken.ID)
		require.NotContains(t, res, "expired-token")
	})

	t.Run("ShowsExpiredWithFlag", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		inv, root := clitest.New(t, "tokens", "ls", "--include-expired")
		clitest.SetupConfig(t, client, root)
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		res := buf.String()
		require.Contains(t, res, validToken.ID)
		require.Contains(t, res, "valid-token")
		require.Contains(t, res, expiredToken.ID)
		require.Contains(t, res, "expired-token")
	})

	t.Run("JSONOutputRespectsFilter", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Default (no expired)
		inv, root := clitest.New(t, "tokens", "ls", "--output=json")
		clitest.SetupConfig(t, client, root)
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		res := buf.String()
		require.Contains(t, res, "valid-token")
		require.NotContains(t, res, "expired-token")

		// With --include-expired
		inv, root = clitest.New(t, "tokens", "ls", "--output=json", "--include-expired")
		clitest.SetupConfig(t, client, root)
		buf = new(bytes.Buffer)
		inv.Stdout = buf
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		res = buf.String()
		require.Contains(t, res, "valid-token")
		require.Contains(t, res, "expired-token")
	})

	t.Run("AllUsersWithIncludeExpired", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		inv, root := clitest.New(t, "tokens", "ls", "--all", "--include-expired")
		clitest.SetupConfig(t, client, root)
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		res := buf.String()
		// Should show both valid and expired tokens
		require.Contains(t, res, validToken.ID)
		require.Contains(t, res, "valid-token")
		require.Contains(t, res, expiredToken.ID)
		require.Contains(t, res, "expired-token")
	})
}
