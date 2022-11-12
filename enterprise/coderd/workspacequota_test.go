package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"
)

func TestWorkspaceQuota(t *testing.T) {
	// TODO: refactor for new impl

	t.Parallel()

	t.Run("BlocksBuild", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		max := 1
		client := coderdenttest.New(t, &coderdenttest.Options{
			UserWorkspaceQuota: max,
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
		})

		user := coderdtest.CreateFirstUser(t, client)
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			WorkspaceQuota: true,
		})
		q1, err := client.WorkspaceQuota(ctx, codersdk.Me)
		require.NoError(t, err)
		require.EqualValues(t, q1.CreditsConsumed, 0)
		require.EqualValues(t, q1.TotalCredits, max)

		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionApply: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "example",
							Type: "aws_instance",
							Cost: 1,
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
		_ = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		workspace, err := client.CreateWorkspace(context.Background(), user.OrganizationID, codersdk.Me, codersdk.CreateWorkspaceRequest{
			TemplateID:        template.ID,
			Name:              "ajksdnvksjd",
			AutostartSchedule: ptr.Ref("CRON_TZ=US/Central 30 9 * * 1-5"),
			TTLMillis:         ptr.Ref((8 * time.Hour).Milliseconds()),
		})
		require.NoError(t, err)

		// ensure count increments
		// q1, err = client.WorkspaceQuota(ctx, codersdk.Me)
		// require.NoError(t, err)
		// require.EqualValues(t, q1.CreditsConsumed, 1)
		// require.EqualValues(t, q1.TotalCredits, max)

		build := coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		require.Equal(t, codersdk.WorkspaceStatusFailed, build.Status)
		require.Contains(t, build.Job.Error, "quota")
	})
}
