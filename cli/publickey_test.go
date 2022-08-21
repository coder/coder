package cli_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
)

func TestPublicKey(t *testing.T) {
	t.Parallel()
	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		cmd, root := clitest.New(t, "publickey")
		clitest.SetupConfig(t, client, root)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		err := cmd.Execute()
		require.NoError(t, err)
		publicKey := buf.String()
		require.NotEmpty(t, publicKey)
	})
}
