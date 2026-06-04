package cli_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/coder/v2/testutil/expecter"
)

func TestRename(t *testing.T) {
	t.Parallel()
	logger := testutil.Logger(t)

	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, AllowWorkspaceRenames: true})
	owner := coderdtest.CreateFirstUser(t, client)
	member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, member, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	want := coderdtest.RandomUsername(t)
	inv, root := clitest.New(t, "rename", workspace.Name, want, "--yes")
	clitest.SetupConfig(t, member, root)
	stdout := expecter.NewAttachedToInvocation(t, inv)
	stdin := testutil.NewWriterAttachedToInvocation(t, logger.Named("stdin"), inv)
	clitest.Start(t, inv)

	stdout.ExpectMatch(ctx, "confirm rename:")
	stdin.WriteLine(workspace.Name)
	stdout.ExpectMatch(ctx, "renamed to")

	ws, err := client.Workspace(ctx, workspace.ID)
	assert.NoError(t, err)

	got := ws.Name
	assert.Equal(t, want, got, "workspace name did not change")
}
