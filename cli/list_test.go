package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestList(t *testing.T) {
	t.Parallel()
	t.Run("Single", func(t *testing.T) {
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		member, memberUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		// setup template
		r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: owner.OrganizationID,
			OwnerID:        memberUser.ID,
		}).WithAgent().Do()

		inv, root := clitest.New(t, "ls")
		clitest.SetupConfig(t, member, root)
		pty := ptytest.New(t).Attach(inv)

		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancelFunc()
		done := make(chan any)
		go func() {
			errC := inv.WithContext(ctx).Run()
			assert.NoError(t, errC)
			close(done)
		}()
		pty.ExpectMatch(r.Workspace.Name)
		pty.ExpectMatch("Started")
		cancelFunc()
		<-done
	})

	t.Run("JSON", func(t *testing.T) {
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		member, memberUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		_ = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: owner.OrganizationID,
			OwnerID:        memberUser.ID,
		}).WithAgent().Do()

		inv, root := clitest.New(t, "list", "--output=json")
		clitest.SetupConfig(t, member, root)

		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancelFunc()

		out := bytes.NewBuffer(nil)
		inv.Stdout = out
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		var workspaces []codersdk.Workspace
		require.NoError(t, json.Unmarshal(out.Bytes(), &workspaces))
		require.Len(t, workspaces, 1)
	})

	t.Run("NoWorkspacesJSON", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		inv, root := clitest.New(t, "list", "--output=json")
		clitest.SetupConfig(t, member, root)

		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancelFunc()

		stdout := bytes.NewBuffer(nil)
		stderr := bytes.NewBuffer(nil)
		inv.Stdout = stdout
		inv.Stderr = stderr
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		var workspaces []codersdk.Workspace
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &workspaces))
		require.Len(t, workspaces, 0)

		require.Len(t, stderr.Bytes(), 0)
	})

	t.Run("SharedWorkspaces", func(t *testing.T) {
		t.Parallel()

		var (
			client, db = coderdtest.NewWithDatabase(t, &coderdtest.Options{
				DeploymentValues: coderdtest.DeploymentValues(t, func(dv *codersdk.DeploymentValues) {
					dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}
				}),
			})
			orgOwner             = coderdtest.CreateFirstUser(t, client)
			memberClient, member = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID, rbac.ScopedRoleOrgAuditor(orgOwner.OrganizationID))
			sharedWorkspace      = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				Name:           "wibble",
				OwnerID:        orgOwner.UserID,
				OrganizationID: orgOwner.OrganizationID,
			}).Do().Workspace
			_ = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				Name:           "wobble",
				OwnerID:        orgOwner.UserID,
				OrganizationID: orgOwner.OrganizationID,
			}).Do().Workspace
		)

		ctx := testutil.Context(t, testutil.WaitMedium)

		client.UpdateWorkspaceACL(ctx, sharedWorkspace.ID, codersdk.UpdateWorkspaceACL{
			UserRoles: map[string]codersdk.WorkspaceRole{
				member.ID.String(): codersdk.WorkspaceRoleUse,
			},
		})

		inv, root := clitest.New(t, "list", "--shared-with-me", "--output=json")
		clitest.SetupConfig(t, memberClient, root)

		stdout := new(bytes.Buffer)
		inv.Stdout = stdout
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		var workspaces []codersdk.Workspace
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &workspaces))
		require.Len(t, workspaces, 1)
		require.Equal(t, sharedWorkspace.ID, workspaces[0].ID)
	})
}
