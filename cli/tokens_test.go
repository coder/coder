package cli_test

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
)

func TestTokens(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	// helpful empty response
	cmd, root := clitest.New(t, "tokens", "ls")
	clitest.SetupConfig(t, client, root)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	err := cmd.Execute()
	require.NoError(t, err)
	res := buf.String()
	require.Contains(t, res, "tokens found")

	cmd, root = clitest.New(t, "tokens", "create")
	clitest.SetupConfig(t, client, root)
	buf = new(bytes.Buffer)
	cmd.SetOut(buf)
	err = cmd.Execute()
	require.NoError(t, err)
	res = buf.String()
	require.NotEmpty(t, res)
	// find API key in format "XXXXXXXXXX-XXXXXXXXXXXXXXXXXXXXXX"
	r := regexp.MustCompile("[a-zA-Z0-9]{10}-[a-zA-Z0-9]{22}")
	require.Regexp(t, r, res)
	key := r.FindString(res)
	id := key[:10]

	cmd, root = clitest.New(t, "tokens", "ls")
	clitest.SetupConfig(t, client, root)
	buf = new(bytes.Buffer)
	cmd.SetOut(buf)
	err = cmd.Execute()
	require.NoError(t, err)
	res = buf.String()
	require.NotEmpty(t, res)
	require.Contains(t, res, "ID")
	require.Contains(t, res, "EXPIRES AT")
	require.Contains(t, res, "CREATED AT")
	require.Contains(t, res, "LAST USED")
	require.Contains(t, res, id)

	cmd, root = clitest.New(t, "tokens", "rm", id)
	clitest.SetupConfig(t, client, root)
	buf = new(bytes.Buffer)
	cmd.SetOut(buf)
	err = cmd.Execute()
	require.NoError(t, err)
	res = buf.String()
	require.NotEmpty(t, res)
	require.Contains(t, res, "deleted")
}
