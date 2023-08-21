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
	_ = coderdtest.CreateFirstUser(t, client)

	ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancelFunc()

	// helpful empty response
	inv, root := clitest.New(t, "tokens", "ls")
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

	inv, root = clitest.New(t, "tokens", "ls")
	clitest.SetupConfig(t, client, root)
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
	require.Contains(t, res, id)

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

	inv, root = clitest.New(t, "tokens", "rm", "token-one")
	clitest.SetupConfig(t, client, root)
	buf = new(bytes.Buffer)
	inv.Stdout = buf
	err = inv.WithContext(ctx).Run()
	require.NoError(t, err)
	res = buf.String()
	require.NotEmpty(t, res)
	require.Contains(t, res, "deleted")
}
