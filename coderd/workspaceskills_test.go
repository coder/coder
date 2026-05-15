package coderd_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontextconfig"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
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
	client := coderdtest.New(t, &coderdtest.Options{
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

	_ = agenttest.New(t, client.URL, agentToken, agenttest.WithContextConfigFromEnv())
	coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).Wait()

	writeWorkspaceSkill(t, skillsDir, "review-code", "Review code", "Read the diff.")
	skills, err := expClient.WorkspaceSkills(ctx, workspace.ID)
	require.NoError(t, err)
	require.Equal(t, []codersdk.WorkspaceSkillMetadata{{
		Name:        "review-code",
		Description: "Review code",
	}}, skills)

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
