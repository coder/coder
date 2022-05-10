package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestProvisionerJobLogs(t *testing.T) {
	t.Parallel()
	t.Run("StreamAfterComplete", func(t *testing.T) {
		t.Parallel()
		api := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, api.Client)
		coderdtest.NewProvisionerDaemon(t, api.Client)
		version := coderdtest.CreateTemplateVersion(t, api.Client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_INFO,
						Output: "log-output",
					},
				},
			}, {
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{},
				},
			}},
		})
		template := coderdtest.CreateTemplate(t, api.Client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, api.Client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, api.Client, user.OrganizationID, template.ID)
		before := time.Now().UTC()
		coderdtest.AwaitWorkspaceBuildJob(t, api.Client, workspace.LatestBuild.ID)

		ctx, cancelFunc := context.WithCancel(context.Background())
		t.Cleanup(cancelFunc)
		logs, err := api.Client.WorkspaceBuildLogsAfter(ctx, workspace.LatestBuild.ID, before)
		require.NoError(t, err)
		for {
			_, ok := <-logs
			if !ok {
				return
			}
		}
	})

	t.Run("StreamWhileRunning", func(t *testing.T) {
		t.Parallel()
		api := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, api.Client)
		coderdtest.NewProvisionerDaemon(t, api.Client)
		version := coderdtest.CreateTemplateVersion(t, api.Client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_INFO,
						Output: "log-output",
					},
				},
			}, {
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{},
				},
			}},
		})
		template := coderdtest.CreateTemplate(t, api.Client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, api.Client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, api.Client, user.OrganizationID, template.ID)
		before := database.Now()
		ctx, cancelFunc := context.WithCancel(context.Background())
		t.Cleanup(cancelFunc)
		logs, err := api.Client.WorkspaceBuildLogsAfter(ctx, workspace.LatestBuild.ID, before)
		require.NoError(t, err)
		for {
			_, ok := <-logs
			if !ok {
				return
			}
		}
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		api := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, api.Client)
		coderdtest.NewProvisionerDaemon(t, api.Client)
		version := coderdtest.CreateTemplateVersion(t, api.Client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_INFO,
						Output: "log-output",
					},
				},
			}, {
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{},
				},
			}},
		})
		template := coderdtest.CreateTemplate(t, api.Client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, api.Client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, api.Client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, api.Client, workspace.LatestBuild.ID)
		logs, err := api.Client.WorkspaceBuildLogsBefore(context.Background(), workspace.LatestBuild.ID, time.Now())
		require.NoError(t, err)
		require.Greater(t, len(logs), 1)
	})
}
