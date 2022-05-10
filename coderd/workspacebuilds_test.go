package coderd_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestWorkspaceBuild(t *testing.T) {
	t.Parallel()
	api := coderdtest.New(t, nil)
	user := coderdtest.CreateFirstUser(t, api.Client)
	coderdtest.NewProvisionerDaemon(t, api.Client)
	version := coderdtest.CreateTemplateVersion(t, api.Client, user.OrganizationID, nil)
	template := coderdtest.CreateTemplate(t, api.Client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, api.Client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, api.Client, user.OrganizationID, template.ID)
	_, err := api.Client.WorkspaceBuild(context.Background(), workspace.LatestBuild.ID)
	require.NoError(t, err)
}

func TestPatchCancelWorkspaceBuild(t *testing.T) {
	t.Parallel()
	api := coderdtest.New(t, nil)
	user := coderdtest.CreateFirstUser(t, api.Client)
	coderdtest.NewProvisionerDaemon(t, api.Client)
	version := coderdtest.CreateTemplateVersion(t, api.Client, user.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		Provision: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Log{
				Log: &proto.Log{},
			},
		}},
		ProvisionDryRun: echo.ProvisionComplete,
	})
	coderdtest.AwaitTemplateVersionJob(t, api.Client, version.ID)
	template := coderdtest.CreateTemplate(t, api.Client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, api.Client, user.OrganizationID, template.ID)
	var build codersdk.WorkspaceBuild
	require.Eventually(t, func() bool {
		var err error
		build, err = api.Client.WorkspaceBuild(context.Background(), workspace.LatestBuild.ID)
		require.NoError(t, err)
		return build.Job.Status == codersdk.ProvisionerJobRunning
	}, 5*time.Second, 25*time.Millisecond)
	err := api.Client.CancelWorkspaceBuild(context.Background(), build.ID)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		var err error
		build, err = api.Client.WorkspaceBuild(context.Background(), build.ID)
		require.NoError(t, err)
		return build.Job.Status == codersdk.ProvisionerJobCanceled
	}, 5*time.Second, 25*time.Millisecond)
}

func TestWorkspaceBuildResources(t *testing.T) {
	t.Parallel()
	t.Run("ListRunning", func(t *testing.T) {
		t.Parallel()
		api := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, api.Client)
		closeDaemon := coderdtest.NewProvisionerDaemon(t, api.Client)
		version := coderdtest.CreateTemplateVersion(t, api.Client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, api.Client, version.ID)
		closeDaemon.Close()
		template := coderdtest.CreateTemplate(t, api.Client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, api.Client, user.OrganizationID, template.ID)
		_, err := api.Client.WorkspaceResourcesByBuild(context.Background(), workspace.LatestBuild.ID)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
	})
	t.Run("List", func(t *testing.T) {
		t.Parallel()
		api := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, api.Client)
		coderdtest.NewProvisionerDaemon(t, api.Client)
		version := coderdtest.CreateTemplateVersion(t, api.Client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "some",
							Type: "example",
							Agents: []*proto.Agent{{
								Id:   "something",
								Auth: &proto.Agent_Token{},
							}},
						}, {
							Name: "another",
							Type: "example",
						}},
					},
				},
			}},
		})
		coderdtest.AwaitTemplateVersionJob(t, api.Client, version.ID)
		template := coderdtest.CreateTemplate(t, api.Client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, api.Client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, api.Client, workspace.LatestBuild.ID)
		resources, err := api.Client.WorkspaceResourcesByBuild(context.Background(), workspace.LatestBuild.ID)
		require.NoError(t, err)
		require.NotNil(t, resources)
		require.Len(t, resources, 2)
		require.Equal(t, "some", resources[0].Name)
		require.Equal(t, "example", resources[0].Type)
		require.Len(t, resources[0].Agents, 1)
	})
}

func TestWorkspaceBuildLogs(t *testing.T) {
	t.Parallel()
	api := coderdtest.New(t, nil)
	user := coderdtest.CreateFirstUser(t, api.Client)
	coderdtest.NewProvisionerDaemon(t, api.Client)
	before := time.Now()
	version := coderdtest.CreateTemplateVersion(t, api.Client, user.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		Provision: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Log{
				Log: &proto.Log{
					Level:  proto.LogLevel_INFO,
					Output: "example",
				},
			},
		}, {
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "some",
						Type: "example",
						Agents: []*proto.Agent{{
							Id:   "something",
							Auth: &proto.Agent_Token{},
						}},
					}, {
						Name: "another",
						Type: "example",
					}},
				},
			},
		}},
	})
	coderdtest.AwaitTemplateVersionJob(t, api.Client, version.ID)
	template := coderdtest.CreateTemplate(t, api.Client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, api.Client, user.OrganizationID, template.ID)
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	logs, err := api.Client.WorkspaceBuildLogsAfter(ctx, workspace.LatestBuild.ID, before)
	require.NoError(t, err)
	for {
		log, ok := <-logs
		if !ok {
			break
		}
		if log.Output == "example" {
			return
		}
	}
	require.Fail(t, "example message never happened")
}

func TestWorkspaceBuildState(t *testing.T) {
	t.Parallel()
	api := coderdtest.New(t, nil)
	user := coderdtest.CreateFirstUser(t, api.Client)
	coderdtest.NewProvisionerDaemon(t, api.Client)
	wantState := []byte("some kinda state")
	version := coderdtest.CreateTemplateVersion(t, api.Client, user.OrganizationID, &echo.Responses{
		Parse:           echo.ParseComplete,
		ProvisionDryRun: echo.ProvisionComplete,
		Provision: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					State: wantState,
				},
			},
		}},
	})
	coderdtest.AwaitTemplateVersionJob(t, api.Client, version.ID)
	template := coderdtest.CreateTemplate(t, api.Client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, api.Client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, api.Client, workspace.LatestBuild.ID)
	gotState, err := api.Client.WorkspaceBuildState(context.Background(), workspace.LatestBuild.ID)
	require.NoError(t, err)
	require.Equal(t, wantState, gotState)
}
