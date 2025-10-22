package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspacePortSharePublic(t *testing.T) {
	t.Parallel()

	ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{IncludeProvisionerDaemon: true},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{codersdk.FeatureControlSharedPorts: 1},
		},
	})
	client, user := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleTemplateAdmin())
	r := setupWorkspaceAgent(t, client, codersdk.CreateFirstUserResponse{
		UserID:         user.ID,
		OrganizationID: owner.OrganizationID,
	}, 0)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	templ, err := client.Template(ctx, r.workspace.TemplateID)
	require.NoError(t, err)
	require.Equal(t, templ.MaxPortShareLevel, codersdk.WorkspaceAgentPortShareLevelOwner)

	// Try to update port share with template max port share level owner.
	_, err = client.UpsertWorkspaceAgentPortShare(ctx, r.workspace.ID, codersdk.UpsertWorkspaceAgentPortShareRequest{
		AgentName:  r.sdkAgent.Name,
		Port:       8080,
		ShareLevel: codersdk.WorkspaceAgentPortShareLevelPublic,
		Protocol:   codersdk.WorkspaceAgentPortShareProtocolHTTP,
	})
	require.Error(t, err, "Port sharing level not allowed")

	// Update the template max port share level to public
	client.UpdateTemplateMeta(ctx, r.workspace.TemplateID, codersdk.UpdateTemplateMeta{
		MaxPortShareLevel: ptr.Ref(codersdk.WorkspaceAgentPortShareLevelPublic),
	})

	// OK
	ps, err := client.UpsertWorkspaceAgentPortShare(ctx, r.workspace.ID, codersdk.UpsertWorkspaceAgentPortShareRequest{
		AgentName:  r.sdkAgent.Name,
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
		Options: &coderdtest.Options{IncludeProvisionerDaemon: true},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{codersdk.FeatureControlSharedPorts: 1},
		},
	})
	client, user := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleTemplateAdmin())
	r := setupWorkspaceAgent(t, client, codersdk.CreateFirstUserResponse{
		UserID:         user.ID,
		OrganizationID: owner.OrganizationID,
	}, 0)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	templ, err := client.Template(ctx, r.workspace.TemplateID)
	require.NoError(t, err)
	require.Equal(t, templ.MaxPortShareLevel, codersdk.WorkspaceAgentPortShareLevelOwner)

	// Try to update port share with template max port share level owner
	_, err = client.UpsertWorkspaceAgentPortShare(ctx, r.workspace.ID, codersdk.UpsertWorkspaceAgentPortShareRequest{
		AgentName:  r.sdkAgent.Name,
		Port:       8080,
		ShareLevel: codersdk.WorkspaceAgentPortShareLevelOrganization,
		Protocol:   codersdk.WorkspaceAgentPortShareProtocolHTTP,
	})
	require.Error(t, err, "Port sharing level not allowed")

	// Update the template max port share level to organization
	client.UpdateTemplateMeta(ctx, r.workspace.TemplateID, codersdk.UpdateTemplateMeta{
		MaxPortShareLevel: ptr.Ref(codersdk.WorkspaceAgentPortShareLevelOrganization),
	})

	// Try to share a port publicly with template max port share level organization
	_, err = client.UpsertWorkspaceAgentPortShare(ctx, r.workspace.ID, codersdk.UpsertWorkspaceAgentPortShareRequest{
		AgentName:  r.sdkAgent.Name,
		Port:       8080,
		ShareLevel: codersdk.WorkspaceAgentPortShareLevelPublic,
		Protocol:   codersdk.WorkspaceAgentPortShareProtocolHTTP,
	})
	require.Error(t, err, "Port sharing level not allowed")

	// OK
	ps, err := client.UpsertWorkspaceAgentPortShare(ctx, r.workspace.ID, codersdk.UpsertWorkspaceAgentPortShareRequest{
		AgentName:  r.sdkAgent.Name,
		Port:       8080,
		ShareLevel: codersdk.WorkspaceAgentPortShareLevelOrganization,
		Protocol:   codersdk.WorkspaceAgentPortShareProtocolHTTP,
	})
	require.NoError(t, err)
	require.EqualValues(t, codersdk.WorkspaceAgentPortShareLevelOrganization, ps.ShareLevel)
}
