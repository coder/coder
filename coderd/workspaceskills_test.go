package coderd_test

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
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
		name               string
		provider           func(t *testing.T, releaseCalled *bool, deadlines *workspaceSkillsDeadlineRecorder) workspaceSkillsAgentProvider
		wantSkills         []codersdk.WorkspaceSkillMetadata
		wantStatus         int
		wantMessage        string
		wantDetail         string
		wantRelease        bool
		wantConfigDeadline bool
	}{
		{
			name: "dial failure",
			provider: func(t *testing.T, releaseCalled *bool, deadlines *workspaceSkillsDeadlineRecorder) workspaceSkillsAgentProvider {
				return workspaceSkillsAgentProvider{
					agentConn: func(ctx context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
						deadlines.recordDial(ctx)
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
			provider: func(t *testing.T, releaseCalled *bool, deadlines *workspaceSkillsDeadlineRecorder) workspaceSkillsAgentProvider {
				conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
				conn.EXPECT().ContextConfig(gomock.Any()).DoAndReturn(func(ctx context.Context) (workspacesdk.ContextConfigResponse, error) {
					deadlines.recordConfig(ctx)
					return workspacesdk.ContextConfigResponse{}, xerrors.New("context config failure")
				})
				return workspaceSkillsAgentProvider{
					agentConn: func(ctx context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
						deadlines.recordDial(ctx)
						return conn, func() { *releaseCalled = true }, nil
					},
				}
			},
			wantStatus:         http.StatusBadGateway,
			wantMessage:        "Failed to fetch workspace skills from agent.",
			wantDetail:         "context config failure",
			wantRelease:        true,
			wantConfigDeadline: true,
		},
		{
			name: "success",
			provider: func(t *testing.T, releaseCalled *bool, deadlines *workspaceSkillsDeadlineRecorder) workspaceSkillsAgentProvider {
				conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
				conn.EXPECT().ContextConfig(gomock.Any()).DoAndReturn(func(ctx context.Context) (workspacesdk.ContextConfigResponse, error) {
					deadlines.recordConfig(ctx)
					return workspacesdk.ContextConfigResponse{
						Parts: []codersdk.ChatMessagePart{{
							Type:             codersdk.ChatMessagePartTypeSkill,
							SkillName:        "review-code",
							SkillDescription: "Review code",
						}},
					}, nil
				})
				return workspaceSkillsAgentProvider{
					agentConn: func(ctx context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
						deadlines.recordDial(ctx)
						return conn, func() { *releaseCalled = true }, nil
					},
				}
			},
			wantSkills:         expectedSkills,
			wantRelease:        true,
			wantConfigDeadline: true,
		},
	} {
		releaseCalled := false
		deadlines := &workspaceSkillsDeadlineRecorder{}
		restore := coderd.SetAgentProviderForTest(api, tt.provider(t, &releaseCalled, deadlines))
		skills, err := expClient.WorkspaceSkills(ctx, workspace.ID)
		restore()

		if tt.wantStatus != 0 {
			requireWorkspaceSkillsSDKError(t, err, tt.wantStatus, tt.wantMessage, tt.wantDetail)
		} else {
			require.NoError(t, err, tt.name)
			require.Equal(t, tt.wantSkills, skills, tt.name)
		}
		require.Equal(t, tt.wantRelease, releaseCalled, tt.name)
		deadlines.requireDial(t, tt.name, 30*time.Second)
		if tt.wantConfigDeadline {
			deadlines.requireConfig(t, tt.name, 5*time.Second)
		} else {
			deadlines.requireNoConfig(t, tt.name)
		}
	}

	workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)
	requireWorkspaceSkillsEmptyWithoutDial(ctx, t, expClient, api, workspace.ID)

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
	requireWorkspaceSkillsEmptyWithoutDial(ctx, t, expClient, api, workspace.ID)
}

type workspaceSkillsAgentProvider struct {
	workspaceapps.AgentProvider
	agentConn func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error)
}

func (p workspaceSkillsAgentProvider) AgentConn(ctx context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
	return p.agentConn(ctx, agentID)
}

type workspaceSkillsDeadlineRecorder struct {
	mu     sync.Mutex
	dial   workspaceSkillsDeadlineObservation
	config workspaceSkillsDeadlineObservation
}

type workspaceSkillsDeadlineObservation struct {
	seen     bool
	ok       bool
	observed time.Time
	deadline time.Time
}

func (r *workspaceSkillsDeadlineRecorder) recordDial(ctx context.Context) {
	r.record(ctx, &r.dial)
}

func (r *workspaceSkillsDeadlineRecorder) recordConfig(ctx context.Context) {
	r.record(ctx, &r.config)
}

func (r *workspaceSkillsDeadlineRecorder) record(ctx context.Context, observation *workspaceSkillsDeadlineObservation) {
	deadline, ok := ctx.Deadline()
	r.mu.Lock()
	defer r.mu.Unlock()
	*observation = workspaceSkillsDeadlineObservation{
		seen:     true,
		ok:       ok,
		observed: time.Now(),
		deadline: deadline,
	}
}

func (r *workspaceSkillsDeadlineRecorder) requireDial(t testing.TB, name string, want time.Duration) {
	t.Helper()
	r.requireDeadline(t, name, "dial", &r.dial, want)
}

func (r *workspaceSkillsDeadlineRecorder) requireConfig(t testing.TB, name string, want time.Duration) {
	t.Helper()
	r.requireDeadline(t, name, "context config", &r.config, want)
}

func (r *workspaceSkillsDeadlineRecorder) requireNoConfig(t testing.TB, name string) {
	t.Helper()
	r.mu.Lock()
	observation := r.config
	r.mu.Unlock()
	require.False(t, observation.seen, "%s: context config deadline recorded", name)
}

func (r *workspaceSkillsDeadlineRecorder) requireDeadline(t testing.TB, name string, label string, observed *workspaceSkillsDeadlineObservation, want time.Duration) {
	t.Helper()
	r.mu.Lock()
	observation := *observed
	r.mu.Unlock()
	require.True(t, observation.seen, "%s: %s deadline was not recorded", name, label)
	require.True(t, observation.ok, "%s: %s context has no deadline", name, label)
	remaining := observation.deadline.Sub(observation.observed)
	require.Greater(t, remaining, want-2*time.Second, "%s: %s deadline too short", name, label)
	require.LessOrEqual(t, remaining, want, "%s: %s deadline too long", name, label)
}

func requireWorkspaceSkillsEmptyWithoutDial(ctx context.Context, t testing.TB, expClient *codersdk.ExperimentalClient, api *coderd.API, workspaceID uuid.UUID) {
	t.Helper()
	var called atomic.Bool
	restore := coderd.SetAgentProviderForTest(api, workspaceSkillsAgentProvider{
		agentConn: func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			called.Store(true)
			return nil, nil, xerrors.New("workspace skills should not dial agent")
		},
	})
	skills, err := expClient.WorkspaceSkills(ctx, workspaceID)
	restore()
	require.NoError(t, err)
	require.Empty(t, skills)
	require.False(t, called.Load())
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
