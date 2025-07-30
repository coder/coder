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
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestBuildsList(t *testing.T) {
	t.Parallel()

	t.Run("Table", func(t *testing.T) {
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		member, memberUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Create a workspace with a build
		r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: owner.OrganizationID,
			OwnerID:        memberUser.ID,
		}).WithAgent().Do()

		inv, root := clitest.New(t, "builds", "list", r.Workspace.Name)
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

		pty.ExpectMatch("1")                 // Build number
		pty.ExpectMatch(r.Build.ID.String()) // Build ID
		cancelFunc()
		<-done
	})

	t.Run("JSON", func(t *testing.T) {
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		member, memberUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Create a workspace with a build
		r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: owner.OrganizationID,
			OwnerID:        memberUser.ID,
		}).WithAgent().Do()

		inv, root := clitest.New(t, "builds", "list", r.Workspace.Name, "--output=json")
		clitest.SetupConfig(t, member, root)

		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancelFunc()

		out := bytes.NewBuffer(nil)
		inv.Stdout = out
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		var builds []codersdk.WorkspaceBuild
		require.NoError(t, json.Unmarshal(out.Bytes(), &builds))
		require.Len(t, builds, 1)
		assert.Equal(t, r.Build.ID, builds[0].ID)
	})

	t.Run("WorkspaceNotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		inv, root := clitest.New(t, "builds", "list", "non-existent-workspace")
		clitest.SetupConfig(t, member, root)

		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancelFunc()

		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get workspace")
	})
}
