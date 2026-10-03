package cli_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/coder/v2/testutil/expecter"
	"github.com/coder/pretty"
)

func TestCreateGroup(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client, admin := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID, rbac.RoleUserAdmin())

		var (
			groupName = "test"
			avatarURL = "https://example.com"
		)

		inv, conf := newCLI(t, "groups",
			"create", groupName,
			"--avatar-url", avatarURL,
		)

		stdout := expecter.NewAttachedToInvocation(t, inv)
		clitest.SetupConfig(t, anotherClient, conf)
		ctx := testutil.Context(t, testutil.WaitMedium)

		err := inv.Run()
		require.NoError(t, err)

		stdout.ExpectMatch(ctx, fmt.Sprintf("Successfully created group %s!", pretty.Sprint(cliui.DefaultStyles.Keyword, groupName)))
	})
}
