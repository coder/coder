package cli_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
)

func TestReadCommand(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	user := coderdtest.CreateFirstUser(t, client)

	inv, root := clitest.New(t, "read", "users/me")
	clitest.SetupConfig(t, client, root)

	var sb strings.Builder
	inv.Stdout = &sb

	err := inv.Run()
	require.NoError(t, err)
	output := sb.String()
	require.Contains(t, output, user.UserID.String())
	// Check for pretty-printed JSON (indented)
	require.Contains(t, output, "  \"") // at least one indented JSON key
}

func TestReadCommand_NonJSON(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	inv, root := clitest.New(t, "read", "/healthz")
	clitest.SetupConfig(t, client, root)

	var sb strings.Builder
	inv.Stdout = &sb

	err := inv.Run()
	require.NoError(t, err)
	output := sb.String()
	// Should not be pretty-printed JSON (no two-space indent at start)
	require.NotContains(t, output, "  \"")
	// Should contain the plain text OK
	require.Contains(t, output, "OK")
}
