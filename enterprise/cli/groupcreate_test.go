package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
)

func TestCreateGroup(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		var (
			groupName = "test"
			avatarURL = "https://example.com"
		)

		cmd, root := clitest.New(t, "groups",
			"create", groupName,
			"--avatar-url", avatarURL,
		)

		clitest.SetupConfig(t, client, root)

		err := cmd.Execute()
		require.NoError(t, err)
	})
}
