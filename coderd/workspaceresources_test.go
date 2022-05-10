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
		_, err = api.Client.WorkspaceResource(context.Background(), resources[0].ID)
		require.NoError(t, err)
	})
}
