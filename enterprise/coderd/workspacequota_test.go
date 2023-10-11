package coderd_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
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
	t.Parallel()

	// This first test verifies the behavior of creating and deleting workspaces.
	// It also tests multi-group quota stacking and the everyone group.
	t.Run("CreateDelete", func(t *testing.T) {
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

		verifyQuota(ctx, t, client, 0, 0)

		// Patch the 'Everyone' group to verify its quota allowance is being accounted for.
		_, err := client.PatchGroup(ctx, user.OrganizationID, codersdk.PatchGroupRequest{
			QuotaAllowance: ptr.Ref(1),
		})
		require.NoError(t, err)
		verifyQuota(ctx, t, client, 0, 1)

		// Add user to two groups, granting them a total budget of 4.
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

		verifyQuota(ctx, t, client, 0, 4)

		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionApply: []*proto.Response{{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
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
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		// Spin up three workspaces fine
		var wg sync.WaitGroup
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
				build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
				assert.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
			}()
		}
		wg.Wait()
		verifyQuota(ctx, t, client, 4, 4)

		// Next one must fail
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		// Consumed shouldn't bump
		verifyQuota(ctx, t, client, 4, 4)
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
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)
			verifyQuota(ctx, t, client, 3, 4)
			break
		}

		// Next one should now succeed
		workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		build = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		verifyQuota(ctx, t, client, 4, 4)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
	})

	t.Run("StartStop", func(t *testing.T) {
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

		verifyQuota(ctx, t, client, 0, 0)

		// Patch the 'Everyone' group to verify its quota allowance is being accounted for.
		_, err := client.PatchGroup(ctx, user.OrganizationID, codersdk.PatchGroupRequest{
			QuotaAllowance: ptr.Ref(4),
		})
		require.NoError(t, err)
		verifyQuota(ctx, t, client, 0, 4)

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionPlanMap: map[proto.WorkspaceTransition][]*proto.Response{
				proto.WorkspaceTransition_START: planWithCost(2),
				proto.WorkspaceTransition_STOP:  planWithCost(1),
			},
			ProvisionApplyMap: map[proto.WorkspaceTransition][]*proto.Response{
				proto.WorkspaceTransition_START: applyWithCost(2),
				proto.WorkspaceTransition_STOP:  applyWithCost(1),
			},
		})

		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		// Spin up two workspaces.
		var wg sync.WaitGroup
		var workspaces []codersdk.Workspace
		for i := 0; i < 2; i++ {
			workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
			workspaces = append(workspaces, workspace)
			build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
			assert.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
		}
		wg.Wait()
		verifyQuota(ctx, t, client, 4, 4)

		// Next one must fail
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
		require.Contains(t, build.Job.Error, "quota")

		// Consumed shouldn't bump
		verifyQuota(ctx, t, client, 4, 4)
		require.Equal(t, codersdk.WorkspaceStatusFailed, build.Status)

		build = coderdtest.CreateWorkspaceBuild(t, client, workspaces[0], database.WorkspaceTransitionStop)
		build = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)

		// Quota goes down one
		verifyQuota(ctx, t, client, 3, 4)
		require.Equal(t, codersdk.WorkspaceStatusStopped, build.Status)

		build = coderdtest.CreateWorkspaceBuild(t, client, workspaces[0], database.WorkspaceTransitionStart)
		build = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)

		// Quota goes back up
		verifyQuota(ctx, t, client, 4, 4)
		require.Equal(t, codersdk.WorkspaceStatusRunning, build.Status)
	})
}

func planWithCost(cost int32) []*proto.Response {
	return []*proto.Response{{
		Type: &proto.Response_Plan{
			Plan: &proto.PlanComplete{
				Resources: []*proto.Resource{{
					Name:      "example",
					Type:      "aws_instance",
					DailyCost: cost,
				}},
			},
		},
	}}
}

func applyWithCost(cost int32) []*proto.Response {
	return []*proto.Response{{
		Type: &proto.Response_Apply{
			Apply: &proto.ApplyComplete{
				Resources: []*proto.Resource{{
					Name:      "example",
					Type:      "aws_instance",
					DailyCost: cost,
				}},
			},
		},
	}}
}
