package cli_test

import (
	"bytes"
	"testing"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"

	"github.com/stretchr/testify/require"
)

func TestFavoriteUnfavorite(t *testing.T) {
	t.Parallel()

	var (
		client, db           = coderdtest.NewWithDatabase(t, nil)
		owner                = coderdtest.CreateFirstUser(t, client)
		memberClient, member = coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		ws                   = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{OwnerID: member.ID, OrganizationID: owner.OrganizationID}).Do()
	)

	inv, root := clitest.New(t, "favorite", ws.Workspace.Name)
	clitest.SetupConfig(t, memberClient, root)

	var buf bytes.Buffer
	inv.Stdout = &buf
	err := inv.Run()
	require.NoError(t, err)

	updated := coderdtest.MustWorkspace(t, memberClient, ws.Workspace.ID)
	require.True(t, updated.Favorite)

	buf.Reset()

	inv, root = clitest.New(t, "unfavorite", ws.Workspace.Name)
	clitest.SetupConfig(t, memberClient, root)
	inv.Stdout = &buf
	err = inv.Run()
	require.NoError(t, err)
	updated = coderdtest.MustWorkspace(t, memberClient, ws.Workspace.ID)
	require.False(t, updated.Favorite)
}
