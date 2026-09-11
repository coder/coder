package coderd_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspaceBuildSSHAutostart(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		stopped  bool
		reason   codersdk.CreateWorkspaceBuildReason
		conflict bool
	}{
		{name: "Running", reason: codersdk.CreateWorkspaceBuildReasonSSHConnection, conflict: true},
		{name: "Stopped", stopped: true, reason: codersdk.CreateWorkspaceBuildReasonSSHConnection},
		{name: "ExplicitStart", reason: codersdk.CreateWorkspaceBuildReasonDashboard},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace := coderdtest.CreateWorkspace(t, client, template.ID)
			latest := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

			if tc.stopped {
				stop, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
					Transition: codersdk.WorkspaceTransitionStop,
				})
				require.NoError(t, err)
				latest = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, stop.ID)
			}

			// An SSH client may submit a delayed autostart after another
			// client has already finished starting the workspace.
			build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStart,
				Reason:     tc.reason,
			})
			if tc.conflict {
				var apiError *codersdk.Error
				require.ErrorAs(t, err, &apiError)
				require.Equal(t, http.StatusConflict, apiError.StatusCode())
				current, err := client.Workspace(ctx, workspace.ID)
				require.NoError(t, err)
				require.Equal(t, latest.ID, current.LatestBuild.ID)
				return
			}
			require.NoError(t, err)
			require.Equal(t, latest.BuildNumber+1, build.BuildNumber)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)
		})
	}
}
