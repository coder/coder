package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestTokens(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	adminUser := coderdtest.CreateFirstUser(t, client)

	secondUserClient, secondUser := coderdtest.CreateAnotherUser(t, client, adminUser.OrganizationID)
	_, thirdUser := coderdtest.CreateAnotherUser(t, client, adminUser.OrganizationID)

	ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancelFunc()

	// helpful empty response
	inv, root := clitest.New(t, "tokens", "ls")
	//nolint:gocritic // This should be run as the owner user.
	clitest.SetupConfig(t, client, root)
	buf := new(bytes.Buffer)
	inv.Stdout = buf
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
	// Result should only contain the token created for the admin user
	require.Contains(t, res, "ID")
	require.Contains(t, res, "EXPIRES AT")
	require.Contains(t, res, "CREATED AT")
	require.Contains(t, res, "LAST USED")
	require.Contains(t, res, id)
	// Result should not contain the token created for the second user
	require.NotContains(t, res, secondTokenID)

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

	inv, root = clitest.New(t, "tokens", "ls", "--output=json")
	clitest.SetupConfig(t, client, root)
	buf = new(bytes.Buffer)
	inv.Stdout = buf
	err = inv.WithContext(ctx).Run()
	require.NoError(t, err)

	var tokens []codersdk.APIKey
	require.NoError(t, json.Unmarshal(buf.Bytes(), &tokens))
	require.Len(t, tokens, 1)
	require.Equal(t, id, tokens[0].ID)

	// Delete by name
	inv, root = clitest.New(t, "tokens", "rm", "token-one")
	clitest.SetupConfig(t, client, root)
	buf = new(bytes.Buffer)
	inv.Stdout = buf
	err = inv.WithContext(ctx).Run()
	require.NoError(t, err)
	res = buf.String()
	require.NotEmpty(t, res)
	require.Contains(t, res, "deleted")

	// Delete by ID
	inv, root = clitest.New(t, "tokens", "rm", secondTokenID)
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

	// Delete by token
	inv, root = clitest.New(t, "tokens", "rm", fourthToken)
	clitest.SetupConfig(t, client, root)
	buf = new(bytes.Buffer)
	inv.Stdout = buf
	err = inv.WithContext(ctx).Run()
	require.NoError(t, err)
	res = buf.String()
	require.NotEmpty(t, res)
	require.Contains(t, res, "deleted")
}
