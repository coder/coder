package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspacePortShare(t *testing.T) {
	t.Parallel()

	ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureControlSharedPorts: 1,
			},
		},
	})
	client, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleTemplateAdmin())
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	// Create a template and workspace
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionApply: []*proto.Response{{
			Type: &proto.Response_Apply{
				Apply: &proto.ApplyComplete{
					Resources: []*proto.Resource{{
						Name: "example",
						Type: "aws_instance",
						Agents: []*proto.Agent{{
							Id:   uuid.NewString(),
							Name: "test-agent",
							Auth: &proto.Agent_Token{
								Token: authToken,
							},
						}},
					}},
				},
			},
		}},
	})
	template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	// try to update port share with template max port share level owner
	_, err := client.UpsertWorkspaceAgentPortShare(ctx, workspace.ID, codersdk.UpsertWorkspaceAgentPortShareRequest{
		AgentName:  "test-agent",
		Port:       8080,
		ShareLevel: codersdk.WorkspaceAgentPortShareLevelPublic,
		Protocol:   codersdk.WorkspaceAgentPortShareProtocolHTTP,
	})
	require.Error(t, err, "Port sharing level not allowed")

	// update the template max port share level to public
	var level codersdk.WorkspaceAgentPortShareLevel = codersdk.WorkspaceAgentPortShareLevelPublic
	client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
		MaxPortShareLevel: &level,
	})

	// OK
	ps, err := client.UpsertWorkspaceAgentPortShare(ctx, workspace.ID, codersdk.UpsertWorkspaceAgentPortShareRequest{
		AgentName:  "test-agent",
		Port:       8080,
		ShareLevel: codersdk.WorkspaceAgentPortShareLevelPublic,
		Protocol:   codersdk.WorkspaceAgentPortShareProtocolHTTP,
	})
	require.NoError(t, err)
	require.EqualValues(t, codersdk.WorkspaceAgentPortShareLevelPublic, ps.ShareLevel)
}

func TestWorkspacePortShareOrganization(t *testing.T) {
	t.Parallel()

	ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureControlSharedPorts:    1,
				codersdk.FeatureMultipleOrganizations: 1,
			},
		},
	})

	// Create a user in the same organization
	sameOrgClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	// Create another organization and a user in it
	otherOrg := coderdenttest.CreateOrganization(t, ownerClient, coderdenttest.CreateOrganizationOptions{})
	_, _ = coderdtest.CreateAnotherUser(t, ownerClient, otherOrg.ID)

	// Create workspace in the first organization
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, sameOrgClient, owner.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionApply: []*proto.Response{{
			Type: &proto.Response_Apply{
				Apply: &proto.ApplyComplete{
					Resources: []*proto.Resource{{
						Name: "example",
						Type: "aws_instance",
						Agents: []*proto.Agent{{
							Id:   uuid.NewString(),
							Name: "test-agent",
							Auth: &proto.Agent_Token{
								Token: authToken,
							},
						}},
					}},
				},
			},
		}},
	})
	template := coderdtest.CreateTemplate(t, sameOrgClient, owner.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJobCompleted(t, sameOrgClient, version.ID)
	workspace := coderdtest.CreateWorkspace(t, sameOrgClient, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, sameOrgClient, workspace.LatestBuild.ID)

	// Update the template max port share level to public to allow all sharing levels
	var level codersdk.WorkspaceAgentPortShareLevel = codersdk.WorkspaceAgentPortShareLevelPublic
	_, err := sameOrgClient.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
		MaxPortShareLevel: &level,
	})
	require.NoError(t, err)

	// Share port at organization level
	ps, err := sameOrgClient.UpsertWorkspaceAgentPortShare(ctx, workspace.ID, codersdk.UpsertWorkspaceAgentPortShareRequest{
		AgentName:  "test-agent",
		Port:       8080,
		ShareLevel: codersdk.WorkspaceAgentPortShareLevelOrganization,
		Protocol:   codersdk.WorkspaceAgentPortShareProtocolHTTP,
	})
	require.NoError(t, err)
	require.EqualValues(t, codersdk.WorkspaceAgentPortShareLevelOrganization, ps.ShareLevel)

	// Verify user in same organization can see the shared port
	shares, err := sameOrgClient.GetWorkspaceAgentPortShares(ctx, workspace.ID)
	require.NoError(t, err)
	require.Len(t, shares.Shares, 1)
	require.EqualValues(t, codersdk.WorkspaceAgentPortShareLevelOrganization, shares.Shares[0].ShareLevel)

	// TODO: Once the authorization middleware is fully integrated, add tests to verify:
	// - Users in the same organization can access the port
	// - Users in different organizations cannot access the port
	// - Unauthenticated users cannot access the port
}
