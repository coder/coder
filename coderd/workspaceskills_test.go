package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentcontextconfig"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

func TestGetWorkspaceSkills(t *testing.T) {
	fakeHome := t.TempDir()
	instructionsDir := filepath.Join(fakeHome, "instructions")
	skillsDir := filepath.Join(fakeHome, "skills")
	require.NoError(t, os.MkdirAll(instructionsDir, 0o755))
	require.NoError(t, os.MkdirAll(skillsDir, 0o755))

	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)
	t.Setenv(agentcontextconfig.EnvInstructionsDirs, instructionsDir)
	t.Setenv(agentcontextconfig.EnvInstructionsFile, "AGENTS.md")
	t.Setenv(agentcontextconfig.EnvSkillsDirs, skillsDir)
	t.Setenv(agentcontextconfig.EnvSkillMetaFile, "SKILL.md")
	t.Setenv(agentcontextconfig.EnvMCPConfigFiles, filepath.Join(fakeHome, "missing-mcp.json"))

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues:         coderdtest.DeploymentValues(t),
		IncludeProvisionerDaemon: true,
	})
	db := api.Database
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

	_ = agenttest.New(t, client.URL, agentToken, agenttest.WithContextConfigFromEnv())
	coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).Wait()

	readOnlyClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.ScopedRoleOrgTemplateAdmin(user.OrganizationID))
	readOnlyExpClient := codersdk.NewExperimentalClient(readOnlyClient)
	_, err := readOnlyExpClient.WorkspaceSkills(ctx, workspace.ID)
	require.Error(t, err)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())

	restore := coderd.SetAgentProviderForTest(api, workspaceSkillsAgentProvider{
		agentConn: func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			return nil, nil, xerrors.New("dial failure")
		},
	})
	_, err = expClient.WorkspaceSkills(ctx, workspace.ID)
	restore()
	requireWorkspaceSkillsSDKError(t, err, http.StatusBadGateway, "Failed to connect to workspace agent.", "dial failure")

	conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
	conn.EXPECT().ContextConfig(gomock.Any()).Return(workspacesdk.ContextConfigResponse{}, xerrors.New("context config failure"))
	restore = coderd.SetAgentProviderForTest(api, workspaceSkillsAgentProvider{
		agentConn: func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			return conn, func() {}, nil
		},
	})
	_, err = expClient.WorkspaceSkills(ctx, workspace.ID)
	restore()
	requireWorkspaceSkillsSDKError(t, err, http.StatusBadGateway, "Failed to fetch workspace skills from agent.", "context config failure")

	writeWorkspaceSkill(t, skillsDir, "review-code", "Review code", "Read the diff.")
	expectedSkills := []codersdk.WorkspaceSkillMetadata{{
		Name:        "review-code",
		Description: "Review code",
	}}
	var skills []codersdk.WorkspaceSkillMetadata
	var skillsErr error
	require.Eventuallyf(t, func() bool {
		skills, skillsErr = expClient.WorkspaceSkills(ctx, workspace.ID)
		return skillsErr == nil && reflect.DeepEqual(expectedSkills, skills)
	}, testutil.WaitLong, testutil.IntervalFast,
		"expected workspace skills %v, got %v, error %v",
		expectedSkills, skills, skillsErr,
	)

	res, err := expClient.Request(ctx, http.MethodGet, "/api/experimental/workspaces/"+workspace.ID.String()+"/skills", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	var rawList []map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(res.Body).Decode(&rawList))
	require.Len(t, rawList, 1)
	require.Contains(t, rawList[0], "name")
	require.Contains(t, rawList[0], "description")
	require.NotContains(t, rawList[0], "content")
	require.NotContains(t, rawList[0], "files")
	require.NotContains(t, rawList[0], "dir")

	require.NoError(t, os.RemoveAll(skillsDir))
	skills, err = expClient.WorkspaceSkills(ctx, workspace.ID)
	require.NoError(t, err)
	require.Empty(t, skills)
	require.NotNil(t, skills)

	require.NoError(t, os.MkdirAll(skillsDir, 0o755))
	writeWorkspaceSkill(t, skillsDir, "valid-sibling", "Valid sibling", "Do valid work.")
	writeWorkspaceSkill(t, skillsDir, "mismatched-dir", "Mismatch", "Skip this.", "actual-name")
	skills, err = expClient.WorkspaceSkills(ctx, workspace.ID)
	require.NoError(t, err)
	require.Equal(t, []codersdk.WorkspaceSkillMetadata{{
		Name:        "valid-sibling",
		Description: "Valid sibling",
	}}, skills)

	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)
	skills, err = expClient.WorkspaceSkills(ctx, workspace.ID)
	require.NoError(t, err)
	require.Empty(t, skills)
	require.NotNil(t, skills)

	require.NoError(t, db.UpdateWorkspaceDeletedByID(dbauthz.AsSystemRestricted(ctx), database.UpdateWorkspaceDeletedByIDParams{
		ID:      workspace.ID,
		Deleted: true,
	}))
	skills, err = expClient.WorkspaceSkills(ctx, workspace.ID)
	require.NoError(t, err)
	require.Empty(t, skills)
	require.NotNil(t, skills)
}

type workspaceSkillsAgentProvider struct {
	workspaceapps.AgentProvider
	agentConn func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error)
}

func (p workspaceSkillsAgentProvider) AgentConn(ctx context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
	return p.agentConn(ctx, agentID)
}

func requireWorkspaceSkillsSDKError(t testing.TB, err error, statusCode int, message string, detail string) {
	t.Helper()
	require.Error(t, err)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, statusCode, sdkErr.StatusCode())
	require.Equal(t, message, sdkErr.Message)
	require.Equal(t, detail, sdkErr.Detail)
}

func writeWorkspaceSkill(t testing.TB, skillsDir string, dirName string, description string, body string, frontmatterName ...string) {
	t.Helper()
	name := dirName
	if len(frontmatterName) > 0 {
		name = frontmatterName[0]
	}
	dir := filepath.Join(skillsDir, dirName)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n" + body + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o600))
}
