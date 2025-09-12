package cli_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
)

func TestAPICommand(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	user := coderdtest.CreateFirstUser(t, client)

	// Create a regular user for testing instead of using the admin/owner
	reg, regUser := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

	inv, root := clitest.New(t, "api", "users/me")
	// Use the regular user for authentication instead of owner
	clitest.SetupConfig(t, reg, root)

	var sb strings.Builder
	inv.Stdout = &sb

	err := inv.Run()
	require.NoError(t, err)
	output := sb.String()
	require.Contains(t, output, regUser.ID.String())
	// Check for pretty-printed JSON (indented)
	require.Contains(t, output, "  \"") // at least one indented JSON key
}

func TestAPICommand_NonJSON(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	user := coderdtest.CreateFirstUser(t, client)

	// Create a regular user for testing instead of using the admin/owner
	reg, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

	inv, root := clitest.New(t, "api", "/healthz")
	// Use the regular user for authentication instead of owner
	clitest.SetupConfig(t, reg, root)

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
