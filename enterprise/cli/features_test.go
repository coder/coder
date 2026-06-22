package cli_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/coder/v2/testutil/expecter"
)

func TestFeaturesList(t *testing.T) {
	t.Parallel()
	t.Run("Table", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		client, admin := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
		inv, conf := newCLI(t, "features", "list")
		clitest.SetupConfig(t, anotherClient, conf)
		stdout := expecter.NewAttachedToInvocation(t, inv)
		clitest.Start(t, inv)
		stdout.ExpectMatch(ctx, "user_limit")
		stdout.ExpectMatch(ctx, "not_entitled")
	})
	t.Run("JSON", func(t *testing.T) {
		t.Parallel()

		client, admin := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
		inv, conf := newCLI(t, "features", "list", "-o", "json")
		clitest.SetupConfig(t, anotherClient, conf)
		doneChan := make(chan struct{})

		buf := bytes.NewBuffer(nil)
		inv.Stdout = buf
		go func() {
			defer close(doneChan)
			err := inv.Run()
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
