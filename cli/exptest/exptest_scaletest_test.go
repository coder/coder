package exptest_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// This test validates that the scaletest CLI filters out workspaces not owned
// when disable owner workspace access is set.
// This test is in its own package because it mutates a global variable that
// can influence other tests in the same package.
// nolint:paralleltest
func TestScaleTestWorkspaceTraffic_UseHostLogin(t *testing.T) {
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	client := coderdtest.New(t, &coderdtest.Options{
		Logger:                   &log,
		IncludeProvisionerDaemon: true,
		DeploymentValues: coderdtest.DeploymentValues(t, func(dv *codersdk.DeploymentValues) {
			dv.DisableOwnerWorkspaceExec = true
		}),
	})
	owner := coderdtest.CreateFirstUser(t, client)
	tv := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, tv.ID)
	tpl := coderdtest.CreateTemplate(t, client, owner.OrganizationID, tv.ID)
	// Create a workspace owned by a different user
	memberClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	_ = coderdtest.CreateWorkspace(t, memberClient, tpl.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
		cwr.Name = "scaletest-workspace"
	})

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	// Test without --use-host-login first.g
	inv, root := clitest.New(t, "exp", "scaletest", "workspace-traffic",
		"--template", tpl.Name,
	)
	// nolint:gocritic // We are intentionally testing this as the owner.
	clitest.SetupConfig(t, client, root)
	var stdoutBuf bytes.Buffer
	inv.Stdout = &stdoutBuf

	err := inv.WithContext(ctx).Run()
	require.ErrorContains(t, err, "no scaletest workspaces exist")
	require.Contains(t, stdoutBuf.String(), `1 workspace(s) were skipped`)

	// Test once again with --use-host-login.
	inv, root = clitest.New(t, "exp", "scaletest", "workspace-traffic",
		"--template", tpl.Name,
		"--use-host-login",
	)
	// nolint:gocritic // We are intentionally testing this as the owner.
	clitest.SetupConfig(t, client, root)
	stdoutBuf.Reset()
	inv.Stdout = &stdoutBuf

	err = inv.WithContext(ctx).Run()
	require.ErrorContains(t, err, "no scaletest workspaces exist")
	require.NotContains(t, stdoutBuf.String(), `1 workspace(s) were skipped`)
}
