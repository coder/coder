package cli_test

import (
	"bytes"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestUpdateGroupSync(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentMultiOrganization)}

		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitLong)
		inv, root := clitest.New(t, "organization", "settings", "set", "groupsync")
		clitest.SetupConfig(t, owner, root)

		expectedSettings := codersdk.GroupSyncSettings{
			Field: "groups",
			Mapping: map[string][]uuid.UUID{
				"test": {first.OrganizationID},
			},
			RegexFilter:       regexp.MustCompile("^foo"),
			AutoCreateMissing: true,
			LegacyNameMapping: nil,
		}
		expectedData, err := json.Marshal(expectedSettings)
		require.NoError(t, err)

		buf := new(bytes.Buffer)
		inv.Stdout = buf
		inv.Stdin = bytes.NewBuffer(expectedData)
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.JSONEq(t, string(expectedData), buf.String())

		// Now read it back
		inv, root = clitest.New(t, "organization", "settings", "show", "groupsync")
		clitest.SetupConfig(t, owner, root)

		buf = new(bytes.Buffer)
		inv.Stdout = buf
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.JSONEq(t, string(expectedData), buf.String())
	})
}
