package cli_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestRename(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	// Only append one letter because it's easy to exceed maximum length:
	// E.g. "compassionate-chandrasekhar82" + "t".
	want := workspace.Name + "t"
	inv, root := clitest.New(t, "rename", workspace.Name, want, "--yes")
	clitest.SetupConfig(t, client, root)
	pty := ptytest.New(t)
	pty.Attach(inv)
	clitest.Start(t, inv)

	pty.ExpectMatch("confirm rename:")
	pty.WriteLine(workspace.Name)
	pty.ExpectMatch("renamed to")

	ws, err := client.Workspace(ctx, workspace.ID)
	assert.NoError(t, err)

	got := ws.Name
	assert.Equal(t, want, got, "workspace name did not change")
}
