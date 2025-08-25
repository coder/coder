package cli_test

import (
	"testing"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/testutil"
	"github.com/stretchr/testify/assert"
)

func TestSharingShare(t *testing.T) {
	t.Parallel()

	t.Run("ShareWithUsers+Simple", func(t *testing.T) {
		t.Parallel()

		var (
			client, db                           = coderdtest.NewWithDatabase(t, nil)
			orgOwner                             = coderdtest.CreateFirstUser(t, client)
			workspaceOwnerClient, workspaceOwner = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID)
			_, toShareWithUser                   = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID)
			workspace                            = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OwnerID:        workspaceOwner.ID,
				OrganizationID: orgOwner.OrganizationID,
			}).Do().Workspace
		)

		var _ = testutil.Context(t, testutil.WaitMedium)
		var _, root = clitest.New(t, "sharing", "share", workspace.Name, "--user", toShareWithUser.Username)
		clitest.SetupConfig(t, workspaceOwnerClient, root)

		// TODO: Test output of the command

		// TODO: Test updated ACL

		assert.True(t, false)
	})

	t.Run("ShareWithUsers+Multiple", func(t *testing.T) {
		t.Parallel()

		assert.True(t, false)
	})

	t.Run("ShareWithUsers+UseRole", func(t *testing.T) {
		t.Parallel()

		assert.True(t, false)
	})

	t.Run("ShareWithUsers+AdminRole", func(t *testing.T) {
		t.Parallel()

		assert.True(t, false)
	})

	t.Run("ShareWithGroups+Simple", func(t *testing.T) {
		t.Parallel()

		assert.True(t, false)
	})

	t.Run("ShareWithGroups+Mutliple", func(t *testing.T) {
		t.Parallel()

		assert.True(t, false)
	})

	t.Run("ShareWithGroups+UseRole", func(t *testing.T) {
		t.Parallel()

		assert.True(t, false)
	})

	t.Run("ShareWithGroups+AdminRole", func(t *testing.T) {
		t.Parallel()

		assert.True(t, false)
	})
}
