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
	t.Parallel()
	t.Run("Disabled", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		client := coderdenttest.New(t, &coderdenttest.Options{})
		_ = coderdtest.CreateFirstUser(t, client)
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			WorkspaceQuota: true,
		})
		q1, err := client.WorkspaceQuota(ctx, codersdk.Me)
		require.NoError(t, err)
		require.EqualValues(t, q1.UserWorkspaceLimit, 0)
	})
	t.Run("Enabled", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		max := 3
		client := coderdenttest.New(t, &coderdenttest.Options{
			UserWorkspaceQuota: max,
		})
		user := coderdtest.CreateFirstUser(t, client)
		coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			WorkspaceQuota: true,
		})
		q1, err := client.WorkspaceQuota(ctx, codersdk.Me)
		require.NoError(t, err)
		require.EqualValues(t, q1.UserWorkspaceLimit, max)

		// ensure other user IDs work too
		u2, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
			Email:          "whatever@yo.com",
			Username:       "haha",
			Password:       "laskjdnvkaj",
			OrganizationID: user.OrganizationID,
		})
		require.NoError(t, err)
		q2, err := client.WorkspaceQuota(ctx, u2.ID.String())
		require.NoError(t, err)
		require.EqualValues(t, q1, q2)
	})
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
		require.EqualValues(t, q1.UserWorkspaceCount, 0)
		require.EqualValues(t, q1.UserWorkspaceLimit, max)

		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "example",
							Type: "aws_instance",
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
		_, err = client.CreateWorkspace(context.Background(), user.OrganizationID, codersdk.Me, codersdk.CreateWorkspaceRequest{
			TemplateID:        template.ID,
			Name:              "ajksdnvksjd",
			AutostartSchedule: ptr.Ref("CRON_TZ=US/Central 30 9 * * 1-5"),
			TTLMillis:         ptr.Ref((8 * time.Hour).Milliseconds()),
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "User workspace limit")

		// ensure count increments
		q1, err = client.WorkspaceQuota(ctx, codersdk.Me)
		require.NoError(t, err)
		require.EqualValues(t, q1.UserWorkspaceCount, 1)
		require.EqualValues(t, q1.UserWorkspaceLimit, max)
	})
}
