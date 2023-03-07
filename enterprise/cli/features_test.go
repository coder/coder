package cli_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/cli"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/pty/ptytest"
)

func TestFeaturesList(t *testing.T) {
	t.Parallel()
	t.Run("Table", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		cmd, root := clitest.NewWithSubcommands(t, cli.EnterpriseSubcommands(), "features", "list")
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.Stdin = pty.Input()
		cmd.Stdout = pty.Output()
		errC := make(chan error)
		go func() {
			errC <- cmd.Run()
		}()
		require.NoError(t, <-errC)
		pty.ExpectMatch("user_limit")
		pty.ExpectMatch("not_entitled")
	})
	t.Run("JSON", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		cmd, root := clitest.NewWithSubcommands(t, cli.EnterpriseSubcommands(), "features", "list", "-o", "json")
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})

		buf := bytes.NewBuffer(nil)
		cmd.Stdout = buf
		go func() {
			defer close(doneChan)
			err := cmd.Run()
			assert.NoError(t, err)
		}()

		<-doneChan

		var entitlements codersdk.Entitlements
		err := json.Unmarshal(buf.Bytes(), &entitlements)
		require.NoError(t, err, "unmarshal JSON output")
		assert.Empty(t, entitlements.Warnings)
		for _, featureName := range codersdk.FeatureNames {
			assert.Equal(t, codersdk.EntitlementNotEntitled, entitlements.Features[featureName].Entitlement)
		}
		assert.False(t, entitlements.HasLicense)
	})
}
