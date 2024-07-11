package coderdtest_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestNew(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
	_, _ = coderdtest.NewGoogleInstanceIdentity(t, "example", false)
	_, _ = coderdtest.NewAWSInstanceIdentity(t, "an-instance")
}

// TestOrganizationMember checks the coderdtest helper can add organization members
// to multiple orgs.
func TestOrganizationMember(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{})
	owner := coderdtest.CreateFirstUser(t, client)

	second := coderdtest.CreateOrganization(t, client, coderdtest.CreateOrganizationOptions{})
	third := coderdtest.CreateOrganization(t, client, coderdtest.CreateOrganizationOptions{})

	// Assign the user to 3 orgs in this 1 statement
	_, user := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgMember(second.ID), rbac.ScopedRoleOrgMember(third.ID))
	require.Len(t, user.OrganizationIDs, 3)
	require.ElementsMatch(t, user.OrganizationIDs, []uuid.UUID{owner.OrganizationID, second.ID, third.ID})
}
