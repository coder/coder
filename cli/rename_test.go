package cli_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/testutil"
)

func TestRename(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	want := workspace.Name + "-test"
	cmd, root := clitest.New(t, "rename", workspace.Name, want)
	clitest.SetupConfig(t, client, root)
	pty := ptytest.New(t)
	cmd.SetIn(pty.Input())
	cmd.SetOut(pty.Output())

	err := cmd.ExecuteContext(ctx)
	assert.NoError(t, err)

	ws, err := client.Workspace(ctx, workspace.ID)
	assert.NoError(t, err)

	got := ws.Name
	assert.Equal(t, want, got, "workspace name did not change")
}
