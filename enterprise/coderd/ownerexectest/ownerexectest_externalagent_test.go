package ownerexectest_test

import (
	"database/sql"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// This test is in its own package because DisableOwnerWorkspaceExec reloads
// the global RBAC roles, which would affect other tests in the same package.
//
//nolint:paralleltest // Mutates global RBAC roles.
func TestExternalAgentCredentialsOwnerExecDisabled(t *testing.T) {
	dv := coderdtest.DeploymentValues(t)
	dv.DisableOwnerWorkspaceExec = true
	ownerClient, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
		Options: &coderdtest.Options{DeploymentValues: dv},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{codersdk.FeatureWorkspaceExternalAgent: 1},
		},
	})
	memberClient, member := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
	r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: owner.OrganizationID,
		OwnerID:        member.ID,
	}).Seed(database.WorkspaceBuild{
		HasExternalAgent: sql.NullBool{Bool: true, Valid: true},
	}).Resource(&proto.Resource{
		Name: "test-agent",
		Type: "coder_external_agent",
	}).WithAgent(func(a []*proto.Agent) []*proto.Agent {
		a[0].Name = "test-agent"
		a[0].OperatingSystem = "linux"
		a[0].Architecture = "amd64"
		return a
	}).Do()
	ctx := testutil.Context(t, testutil.WaitShort)

	// The owner can still read the workspace but cannot connect to it.
	_, err := ownerClient.Workspace(ctx, r.Workspace.ID)
	require.NoError(t, err)
	_, err = ownerClient.WorkspaceExternalAgentCredentials(ctx, r.Workspace.ID, "test-agent")
	var apiErr *codersdk.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusNotFound, apiErr.StatusCode())

	credentials, err := memberClient.WorkspaceExternalAgentCredentials(ctx, r.Workspace.ID, "test-agent")
	require.NoError(t, err)
	require.Equal(t, r.AgentToken, credentials.AgentToken)
}
