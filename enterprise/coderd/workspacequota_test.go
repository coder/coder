package coderd_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/enterprise/coderd/license"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"
)

func verifyQuota(ctx context.Context, t *testing.T, client *codersdk.Client, consumed, total int) {
	t.Helper()

	got, err := client.WorkspaceQuota(ctx, codersdk.Me)
	require.NoError(t, err)
	require.EqualValues(t, codersdk.WorkspaceQuota{
		Budget:          total,
		CreditsConsumed: consumed,
	}, got)
}

func TestWorkspaceQuota(t *testing.T) {
	// TODO: refactor for new impl

	t.Parallel()

	t.Run("BlocksBuild", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		max := 1
		client, _, api, user := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			UserWorkspaceQuota: max,
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			},
		})
		coderdtest.NewProvisionerDaemon(t, api.AGPL)
		coderdtest.NewProvisionerDaemon(t, api.AGPL)
		coderdtest.NewProvisionerDaemon(t, api.AGPL)

		verifyQuota(ctx, t, client, 0, 0)

		// Add user to two groups, granting them a total budget of 3.
		group1, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name:           "test-1",
			QuotaAllowance: 1,
		})
		require.NoError(t, err)

		group2, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name:           "test-2",
			QuotaAllowance: 2,
		})
		require.NoError(t, err)

		_, err = client.PatchGroup(ctx, group1.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user.UserID.String()},
		})
		require.NoError(t, err)

		_, err = client.PatchGroup(ctx, group2.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user.UserID.String()},
		})
		require.NoError(t, err)

		verifyQuota(ctx, t, client, 0, 3)

		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionApply: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name:      "example",
							Type:      "aws_instance",
							DailyCost: 1,
							Agents: []*proto.Agent{{
								Id:   uuid.NewString(),
								Name: "example",
								Auth: &proto.Agent_Token{
									Token: authToken,
								},
							}},
						}},
					},
				},
			}},
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		// Spin up three workspaces fine
		var wg sync.WaitGroup
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
				build := coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
				assert.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
			}()
		}
		wg.Wait()
		verifyQuota(ctx, t, client, 3, 3)

		// Next one must fail
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		// Consumed shouldn't bump
		verifyQuota(ctx, t, client, 3, 3)
		require.Equal(t, codersdk.WorkspaceStatusFailed, build.Status)
		require.Contains(t, build.Job.Error, "quota")

		// Delete one random workspace, then quota should recover.
		workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
		require.NoError(t, err)
		for _, w := range workspaces.Workspaces {
			if w.LatestBuild.Status != codersdk.WorkspaceStatusRunning {
				continue
			}
			build, err := client.CreateWorkspaceBuild(ctx, w.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionDelete,
			})
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJob(t, client, build.ID)
			verifyQuota(ctx, t, client, 2, 3)
			break
		}

		// Next one should now succeed
		workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		build = coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		verifyQuota(ctx, t, client, 3, 3)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
	})
}
