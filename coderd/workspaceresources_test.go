package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"
)

func TestWorkspaceResource(t *testing.T) {
	t.Parallel()
	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "beta",
							Type: "example",
							Icon: "/icon/server.svg",
							Agents: []*proto.Agent{{
								Id:   "something",
								Name: "b",
								Auth: &proto.Agent_Token{},
							}, {
								Id:   "another",
								Name: "a",
								Auth: &proto.Agent_Token{},
							}},
						}, {
							Name: "alpha",
							Type: "example",
						}},
					},
				},
			}},
		})
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resources, err := client.WorkspaceResourcesByBuild(ctx, workspace.LatestBuild.ID)
		require.NoError(t, err)
		// Ensure it's sorted alphabetically!
		require.Equal(t, "alpha", resources[0].Name)
		require.Equal(t, "beta", resources[1].Name)
		resource, err := client.WorkspaceResource(ctx, resources[1].ID)
		require.NoError(t, err)
		require.Len(t, resource.Agents, 2)
		// Ensure agents are sorted alphabetically!
		require.Equal(t, "a", resource.Agents[0].Name)
		require.Equal(t, "b", resource.Agents[1].Name)
		// Ensure Icon is present
		require.Equal(t, "/icon/server.svg", resources[1].Icon)
	})

	t.Run("Apps", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		apps := []*proto.App{
			{
				Name:    "code-server",
				Command: "some-command",
				Url:     "http://localhost:3000",
				Icon:    "/code.svg",
			},
			{
				Name:                 "code-server-2",
				Command:              "some-command",
				Url:                  "http://localhost:3000",
				Icon:                 "/code.svg",
				HealthcheckEnabled:   true,
				HealthcheckUrl:       "http://localhost:3000",
				HealthcheckInterval:  5,
				HealthcheckThreshold: 6,
			},
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
								Apps: apps,
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

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resources, err := client.WorkspaceResourcesByBuild(ctx, workspace.LatestBuild.ID)
		require.NoError(t, err)
		resource, err := client.WorkspaceResource(ctx, resources[0].ID)
		require.NoError(t, err)
		require.Len(t, resource.Agents, 1)
		agent := resource.Agents[0]
		require.Len(t, agent.Apps, 2)
		got := agent.Apps[0]
		app := apps[0]
		require.EqualValues(t, app.Command, got.Command)
		require.EqualValues(t, app.Icon, got.Icon)
		require.EqualValues(t, app.Name, got.Name)
		require.EqualValues(t, codersdk.WorkspaceAppHealthDisabled, got.Health)
		require.EqualValues(t, app.HealthcheckEnabled, got.HealthcheckEnabled)
		require.EqualValues(t, app.HealthcheckUrl, got.HealthcheckURL)
		require.EqualValues(t, app.HealthcheckInterval, got.HealthcheckInterval)
		require.EqualValues(t, app.HealthcheckThreshold, got.HealthcheckThreshold)
		got = agent.Apps[1]
		app = apps[1]
		require.EqualValues(t, app.Command, got.Command)
		require.EqualValues(t, app.Icon, got.Icon)
		require.EqualValues(t, app.Name, got.Name)
		require.EqualValues(t, codersdk.WorkspaceAppHealthInitializing, got.Health)
		require.EqualValues(t, app.HealthcheckEnabled, got.HealthcheckEnabled)
		require.EqualValues(t, app.HealthcheckUrl, got.HealthcheckURL)
		require.EqualValues(t, app.HealthcheckInterval, got.HealthcheckInterval)
		require.EqualValues(t, app.HealthcheckThreshold, got.HealthcheckThreshold)
	})

	t.Run("Metadata", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
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
							Metadata: []*proto.Resource_Metadata{{
								Key:   "foo",
								Value: "bar",
							}, {
								Key:    "null",
								IsNull: true,
							}, {
								Key: "empty",
							}, {
								Key:       "secret",
								Value:     "squirrel",
								Sensitive: true,
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

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resources, err := client.WorkspaceResourcesByBuild(ctx, workspace.LatestBuild.ID)
		require.NoError(t, err)
		resource, err := client.WorkspaceResource(ctx, resources[0].ID)
		require.NoError(t, err)
		metadata := resource.Metadata
		require.Equal(t, []codersdk.WorkspaceResourceMetadata{{
			Key: "empty",
		}, {
			Key:   "foo",
			Value: "bar",
		}, {
			Key:       "secret",
			Value:     "squirrel",
			Sensitive: true,
		}, {
			Key:   "type",
			Value: "example",
		}}, metadata)
	})
}
