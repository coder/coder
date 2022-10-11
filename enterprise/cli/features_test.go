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
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		errC := make(chan error)
		go func() {
			errC <- cmd.Execute()
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
		cmd.SetOut(buf)
		go func() {
			defer close(doneChan)
			err := cmd.Execute()
			assert.NoError(t, err)
		}()

		<-doneChan

		var entitlements codersdk.Entitlements
		err := json.Unmarshal(buf.Bytes(), &entitlements)
		require.NoError(t, err, "unmarshal JSON output")
		assert.Len(t, entitlements.Features, 6)
		assert.Empty(t, entitlements.Warnings)
		assert.Equal(t, codersdk.EntitlementNotEntitled,
			entitlements.Features[codersdk.FeatureUserLimit].Entitlement)
		assert.Equal(t, codersdk.EntitlementNotEntitled,
			entitlements.Features[codersdk.FeatureAuditLog].Entitlement)
		assert.Equal(t, codersdk.EntitlementNotEntitled,
			entitlements.Features[codersdk.FeatureBrowserOnly].Entitlement)
		assert.Equal(t, codersdk.EntitlementNotEntitled,
			entitlements.Features[codersdk.FeatureWorkspaceQuota].Entitlement)
		assert.Equal(t, codersdk.EntitlementNotEntitled,
			entitlements.Features[codersdk.FeatureRBAC].Entitlement)
		assert.Equal(t, codersdk.EntitlementNotEntitled,
			entitlements.Features[codersdk.FeatureSCIM].Entitlement)
		assert.False(t, entitlements.HasLicense)
		assert.False(t, entitlements.Experimental)
	})
}
