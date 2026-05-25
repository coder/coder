package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
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

	_ = agenttest.New(t, client.URL, agentToken)
	coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).Wait()

	readOnlyClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.ScopedRoleOrgTemplateAdmin(user.OrganizationID))
	_, err := codersdk.NewExperimentalClient(readOnlyClient).WorkspaceSkills(ctx, workspace.ID)
	requireWorkspaceSkillsSDKError(t, err, http.StatusForbidden, "", "")

	expectedSkills := []codersdk.WorkspaceSkillMetadata{{
		Name:        "review-code",
		Description: "Review code",
	}}
	for _, tt := range []struct {
		name        string
		provider    func(t *testing.T, releaseCalled *bool) workspaceSkillsAgentProvider
		wantSkills  []codersdk.WorkspaceSkillMetadata
		wantStatus  int
		wantMessage string
		wantDetail  string
		wantRelease bool
	}{
		{
			name: "dial failure",
			provider: func(t *testing.T, releaseCalled *bool) workspaceSkillsAgentProvider {
				return workspaceSkillsAgentProvider{
					agentConn: func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
						return nil, nil, xerrors.New("dial failure")
					},
				}
			},
			wantStatus:  http.StatusBadGateway,
			wantMessage: "Failed to connect to workspace agent.",
			wantDetail:  "dial failure",
		},
		{
			name: "context config failure",
			provider: func(t *testing.T, releaseCalled *bool) workspaceSkillsAgentProvider {
				conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
				conn.EXPECT().ContextConfig(gomock.Any()).Return(workspacesdk.ContextConfigResponse{}, xerrors.New("context config failure"))
				return workspaceSkillsAgentProvider{
					agentConn: func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
						return conn, func() { *releaseCalled = true }, nil
					},
				}
			},
			wantStatus:  http.StatusBadGateway,
			wantMessage: "Failed to fetch workspace skills from agent.",
			wantDetail:  "context config failure",
			wantRelease: true,
		},
		{
			name: "success",
			provider: func(t *testing.T, releaseCalled *bool) workspaceSkillsAgentProvider {
				conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
				conn.EXPECT().ContextConfig(gomock.Any()).Return(workspacesdk.ContextConfigResponse{
					Parts: []codersdk.ChatMessagePart{{
						Type:             codersdk.ChatMessagePartTypeSkill,
						SkillName:        "review-code",
						SkillDescription: "Review code",
					}},
				}, nil)
				return workspaceSkillsAgentProvider{
					agentConn: func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
						return conn, func() { *releaseCalled = true }, nil
					},
				}
			},
			wantSkills:  expectedSkills,
			wantRelease: true,
		},
	} {
		releaseCalled := false
		restore := coderd.SetAgentProviderForTest(api, tt.provider(t, &releaseCalled))
		skills, err := expClient.WorkspaceSkills(ctx, workspace.ID)
		restore()

		if tt.wantStatus != 0 {
			requireWorkspaceSkillsSDKError(t, err, tt.wantStatus, tt.wantMessage, tt.wantDetail)
		} else {
			require.NoError(t, err, tt.name)
			require.Equal(t, tt.wantSkills, skills, tt.name)
		}
		require.Equal(t, tt.wantRelease, releaseCalled, tt.name)
	}
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
	if message != "" {
		require.Equal(t, message, sdkErr.Message)
	}
	if detail != "" {
		require.Equal(t, detail, sdkErr.Detail)
	}
}
