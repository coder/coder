package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestWorkspaceResource(t *testing.T) {
	t.Parallel()
	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
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
						}},
					},
				},
			}},
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		resources, err := client.WorkspaceResourcesByBuild(context.Background(), workspace.LatestBuild.ID)
		require.NoError(t, err)
		_, err = client.WorkspaceResource(context.Background(), resources[0].ID)
		require.NoError(t, err)
	})

	t.Run("Apps", func(t *testing.T) {
		t.Parallel()
		client, coderd := coderdtest.NewWithAPI(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		coderdtest.NewProvisionerDaemon(t, coderd)
		app := &proto.App{
			Name:    "code-server",
			Command: "some-command",
			Url:     "http://localhost:3000",
			Icon:    "/code.svg",
		}
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
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
								Apps: []*proto.App{app},
							}},
						}},
					},
				},
			}},
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		resources, err := client.WorkspaceResourcesByBuild(context.Background(), workspace.LatestBuild.ID)
		require.NoError(t, err)
		resource, err := client.WorkspaceResource(context.Background(), resources[0].ID)
		require.NoError(t, err)
		require.Len(t, resource.Agents, 1)
		agent := resource.Agents[0]
		require.Len(t, agent.Apps, 1)
		got := agent.Apps[0]
		require.Equal(t, app.Command, got.Command)
		require.Equal(t, app.Icon, got.Icon)
		require.Equal(t, app.Name, got.Name)
	})
}
