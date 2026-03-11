package cli_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestExpOpenPort(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client, store := coderdtest.NewWithDatabase(t, &coderdtest.Options{
			AppHostname: "*.test.coder.com",
		})
		client.SetLogger(testutil.Logger(t).Named("client"))
		first := coderdtest.CreateFirstUser(t, client)
		userClient, user := coderdtest.CreateAnotherUserMutators(t, client, first.OrganizationID, nil, func(r *codersdk.CreateUserRequestWithOrgs) {
			r.Username = "myuser"
		})
		r := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
			Name:           "myworkspace",
			OrganizationID: first.OrganizationID,
			OwnerID:        user.ID,
		}).WithAgent().Do()

		inv, root := clitest.New(t, "exp", "open", "port", r.Workspace.Name, "12345", "--test.open-error")
		clitest.SetupConfig(t, userClient, root)
		var sb strings.Builder
		inv.Stdout = &sb
		inv.Stderr = &sb

		w := clitest.StartWithWaiter(t, inv)
		w.RequireError()
		w.RequireContains("test.open-error")
		require.Contains(t, sb.String(), "12345--dev--myworkspace--myuser.test.coder.com")
	})
}
