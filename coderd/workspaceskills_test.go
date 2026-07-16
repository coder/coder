package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

func TestGetWorkspaceSkills(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues:         coderdtest.DeploymentValues(t),
		IncludeProvisionerDaemon: true,
	})
	user := coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)

	agentToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ApplyComplete,
		ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken),
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	ws, err := client.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	require.NotEmpty(t, ws.LatestBuild.Resources)
	require.NotEmpty(t, ws.LatestBuild.Resources[0].Agents)
	agentID := ws.LatestBuild.Resources[0].Agents[0].ID

	memberClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
	_, err = codersdk.NewExperimentalClient(memberClient).WorkspaceSkills(ctx, workspace.ID)
	requireWorkspaceSkillsSDKError(t, err, http.StatusNotFound)

	// Read access without SSH access is not enough.
	templateAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.ScopedRoleOrgTemplateAdmin(user.OrganizationID))
	_, err = codersdk.NewExperimentalClient(templateAdminClient).WorkspaceSkills(ctx, workspace.ID)
	requireWorkspaceSkillsSDKError(t, err, http.StatusForbidden)

	// The agent has not pushed a context snapshot yet.
	skills, err := expClient.WorkspaceSkills(ctx, workspace.ID)
	require.NoError(t, err)
	require.Empty(t, skills)

	insertWorkspaceSkillsSnapshot(ctx, t, api.Database, agentID)

	skills, err = expClient.WorkspaceSkills(ctx, workspace.ID)
	require.NoError(t, err)
	require.Equal(t, []codersdk.WorkspaceSkillMetadata{{
		Name:        "review-code",
		Description: "Review code",
	}}, skills)

	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)
	skills, err = expClient.WorkspaceSkills(ctx, workspace.ID)
	require.NoError(t, err)
	require.Empty(t, skills)

	badVersion := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ApplyFailed,
		ProvisionGraph: echo.GraphComplete,
	}, func(req *codersdk.CreateTemplateVersionRequest) {
		req.TemplateID = template.ID
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, badVersion.ID)
	coderdtest.UpdateActiveTemplateVersion(t, client, template.ID, badVersion.ID)
	failedBuild := coderdtest.CreateWorkspaceBuild(t, client, workspace, database.WorkspaceTransitionStart, func(req *codersdk.CreateWorkspaceBuildRequest) {
		req.TemplateVersionID = badVersion.ID
	})
	failedBuild = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, failedBuild.ID)
	require.Equal(t, codersdk.ProvisionerJobFailed, failedBuild.Job.Status)
	skills, err = expClient.WorkspaceSkills(ctx, workspace.ID)
	require.NoError(t, err)
	require.Empty(t, skills)
}

// insertWorkspaceSkillsSnapshot stores a pushed context snapshot with one
// healthy skill, one failed skill, and one instruction file. Only the
// healthy skill should surface through the endpoint.
func insertWorkspaceSkillsSnapshot(ctx context.Context, t *testing.T, db database.Store, agentID uuid.UUID) {
	t.Helper()
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	now := dbtime.Now()

	_, err := db.UpsertWorkspaceAgentContextSnapshot(sysCtx, database.UpsertWorkspaceAgentContextSnapshotParams{
		WorkspaceAgentID: agentID,
		Version:          1,
		AggregateHash:    []byte("aggregate-hash"),
		ReceivedAt:       now,
	})
	require.NoError(t, err)

	for _, resource := range []database.UpsertWorkspaceAgentContextResourceParams{
		{
			WorkspaceAgentID: agentID,
			Source:           "/workspace/.agents/skills/review-code",
			BodyKind:         database.WorkspaceAgentContextBodyKindSkill,
			Body:             json.RawMessage(`{"name":"review-code","description":"Review code"}`),
			ContentHash:      []byte("hash-review-code"),
			SizeBytes:        64,
			Status:           database.WorkspaceAgentContextResourceStatusOk,
			Now:              now,
		},
		{
			WorkspaceAgentID: agentID,
			Source:           "/workspace/.agents/skills/broken",
			BodyKind:         database.WorkspaceAgentContextBodyKindSkill,
			Body:             json.RawMessage(`{}`),
			ContentHash:      []byte("hash-broken"),
			SizeBytes:        0,
			Status:           database.WorkspaceAgentContextResourceStatusUnreadable,
			Error:            "read failed",
			Now:              now,
		},
		{
			WorkspaceAgentID: agentID,
			Source:           "/workspace/AGENTS.md",
			BodyKind:         database.WorkspaceAgentContextBodyKindInstructionFile,
			Body:             json.RawMessage(`{"content":"cnVsZXM="}`),
			ContentHash:      []byte("hash-agents-md"),
			SizeBytes:        5,
			Status:           database.WorkspaceAgentContextResourceStatusOk,
			Now:              now,
		},
	} {
		_, err := db.UpsertWorkspaceAgentContextResource(sysCtx, resource)
		require.NoError(t, err)
	}
}

func requireWorkspaceSkillsSDKError(t testing.TB, err error, statusCode int) {
	t.Helper()
	require.Error(t, err)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, statusCode, sdkErr.StatusCode())
}
