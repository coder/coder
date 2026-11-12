package coderd_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/agent/agentworkload"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

// TestLaunchWorkload covers a workload that gets a workspace to itself. Its
// identity is an account of its own that owns the workspace, shares it with
// the sponsor and has nothing of the sponsor's.
func TestLaunchWorkload(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentRuntimeWorkloads)}
	auditor := audit.NewMock()
	adminClient, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
		DeploymentValues:         dv,
		Auditor:                  auditor,
		IncludeProvisionerDaemon: true,
	})
	admin := coderdtest.CreateFirstUser(t, adminClient)
	sponsorClient, sponsor := coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)
	otherClient, _ := coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)

	// Every workspace of this template has one daemon, with this token.
	agentToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, adminClient, admin.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken),
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, adminClient, version.ID)
	template := coderdtest.CreateTemplate(t, adminClient, admin.OrganizationID, version.ID)

	// A workspace of the sponsor's own, which the workload must not reach.
	sponsorBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: admin.OrganizationID,
		OwnerID:        sponsor.ID,
	}).WithAgent().Do()

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	requireStatus := func(err error, status int) {
		t.Helper()
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, status, sdkErr.StatusCode())
	}

	// What is asked for is checked before anything is made.
	_, err := sponsorClient.LaunchWorkload(ctx, codersdk.LaunchWorkloadRequest{TemplateID: template.ID})
	requireStatus(err, http.StatusBadRequest)
	_, err = sponsorClient.LaunchWorkload(ctx, codersdk.LaunchWorkloadRequest{
		TemplateID:            template.ID,
		CreateWorkloadRequest: codersdk.CreateWorkloadRequest{Command: "true", Scopes: []codersdk.APIKeyScope{codersdk.APIKeyScopeAll}},
	})
	requireStatus(err, http.StatusBadRequest)
	_, err = sponsorClient.LaunchWorkload(ctx, codersdk.LaunchWorkloadRequest{
		TemplateID:            uuid.New(),
		CreateWorkloadRequest: codersdk.CreateWorkloadRequest{Command: "true"},
	})
	requireStatus(err, http.StatusNotFound)
	users, err := adminClient.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.Len(t, users.Users, 3, "a refused request leaves no identity behind")

	// The launch waits for the workspace's daemon, which starts once the
	// workspace is built.
	type launched struct {
		workload codersdk.Workload
		err      error
	}
	result := make(chan launched, 1)
	go func() {
		workload, err := sponsorClient.LaunchWorkload(ctx, codersdk.LaunchWorkloadRequest{
			TemplateID: template.ID,
			CreateWorkloadRequest: codersdk.CreateWorkloadRequest{
				Name:    "solo",
				Command: "echo solo",
				Scopes:  []codersdk.APIKeyScope{codersdk.APIKeyScopeWorkspaceRead, codersdk.APIKeyScopeTemplateRead},
			},
		})
		result <- launched{workload: workload, err: err}
	}()
	var workspace codersdk.Workspace
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		found, err := adminClient.Workspaces(ctx, codersdk.WorkspaceFilter{Template: template.Name})
		if err != nil || len(found.Workspaces) != 1 {
			return false
		}
		workspace = found.Workspaces[0]
		return workspace.LatestBuild.Job.Status == codersdk.ProvisionerJobSucceeded
	}, testutil.IntervalFast)

	runtime := &fakeRuntime{t: t, sandboxes: make(map[uuid.UUID]*fakeSandbox)}
	daemonClient := agentsdk.New(adminClient.URL, agentsdk.WithFixedToken(agentToken))
	supervisor, err := agentworkload.New(context.Background(), agentworkload.Options{
		Logger:       testutil.Logger(t).Named("workloads"),
		Runtime:      runtime,
		Policy:       agentworkload.Policy{DefaultImage: "registry.example.com/agent:1", DefaultNetwork: codersdk.WorkloadNetworkNone},
		CoderURL:     adminClient.URL,
		SessionToken: func() string { return agentToken },
		Pending:      daemonClient.PendingWorkloads,
		ReportStart:  daemonClient.ReportWorkloadStart,
		ReportExit:   daemonClient.ReportWorkloadExit,
	})
	require.NoError(t, err)
	_ = agenttest.New(t, adminClient.URL, agentToken, func(o *agent.Options) {
		o.Workloads = supervisor
	})

	launch := testutil.RequireReceive(ctx, t, result)
	require.NoError(t, launch.err)
	solo := launch.workload
	require.Equal(t, codersdk.WorkloadStatusRunning, solo.Status)
	require.Equal(t, workspace.ID, solo.WorkspaceID)
	require.Equal(t, sponsor.ID, solo.SponsorID)
	require.True(t, solo.OwnWorkspace)
	require.NotNil(t, solo.UserID)

	// The identity is an account of its own, which owns the workspace. It
	// cannot sign in and has no API key: the build gave it none.
	require.Equal(t, *solo.UserID, workspace.OwnerID)
	require.NotEqual(t, sponsor.ID, workspace.OwnerID)
	identityUser, err := adminClient.User(ctx, solo.UserID.String())
	require.NoError(t, err)
	require.True(t, identityUser.IsServiceAccount)
	require.Equal(t, codersdk.LoginTypeNone, identityUser.LoginType)
	require.Equal(t, workspace.OwnerName, identityUser.Username)
	//nolint:gocritic // The test looks at what no user may read.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	for _, loginType := range []database.LoginType{database.LoginTypeNone, database.LoginTypeToken, database.LoginTypePassword} {
		keys, err := db.GetAPIKeysByUserID(sysCtx, database.GetAPIKeysByUserIDParams{
			LoginType: loginType, UserID: *solo.UserID, IncludeExpired: true,
		})
		require.NoError(t, err)
		require.Empty(t, keys, "the identity has no API key of login type %s", loginType)
	}

	// The identity shares the workspace with its sponsor, and with nobody
	// else.
	shared, err := sponsorClient.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	require.Equal(t, workspace.ID, shared.ID)
	_, err = otherClient.Workspace(ctx, workspace.ID)
	requireStatus(err, http.StatusNotFound)
	acl, err := db.GetWorkspaceACLByID(sysCtx, workspace.ID)
	require.NoError(t, err)
	require.Len(t, acl.Users, 1)
	require.ElementsMatch(t,
		[]policy.Action{policy.ActionRead, policy.ActionSSH, policy.ActionApplicationConnect, policy.ActionWorkspaceStart, policy.ActionWorkspaceStop},
		acl.Users[sponsor.ID.String()].Permissions)
	require.Empty(t, acl.Groups)

	// The workload is its own account, narrowed to its scopes. It can read
	// its own workspace. It cannot read its sponsor's, which the sponsor
	// could, nor anything its scopes do not name.
	sandbox := runtime.sandbox(solo.ID)
	require.NotNil(t, sandbox)
	channel, err := url.Parse(sandbox.server.URL)
	require.NoError(t, err)
	asSolo := codersdk.New(channel)
	identity, err := asSolo.WorkloadIdentity(ctx)
	require.NoError(t, err)
	require.Equal(t, codersdk.WorkloadIdentityKindWorkload, identity.Kind)
	require.Equal(t, solo.ID, identity.IdentityID)
	require.Equal(t, sponsor.ID, identity.SponsorID)
	require.Equal(t, workspace.OwnerID, identity.OwnerID)
	require.Equal(t, identityUser.Username, identity.OwnerUsername)
	own, err := asSolo.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	require.Equal(t, workspace.ID, own.ID)
	_, err = asSolo.Workspace(ctx, sponsorBuild.Workspace.ID)
	requireStatus(err, http.StatusNotFound)
	_, err = asSolo.User(ctx, sponsor.ID.String())
	require.Error(t, err)
	// A workload does not sponsor workloads.
	_, err = asSolo.LaunchWorkload(ctx, codersdk.LaunchWorkloadRequest{
		TemplateID:            template.ID,
		CreateWorkloadRequest: codersdk.CreateWorkloadRequest{Command: "true"},
	})
	requireStatus(err, http.StatusForbidden)

	// Only one workload runs in a workspace that was made for one.
	_, err = sponsorClient.CreateWorkspaceWorkload(ctx, workspace.ID, codersdk.CreateWorkloadRequest{Command: "true"})
	requireStatus(err, http.StatusNotFound)

	// The sponsor lists it and may end it. Nobody else may.
	sponsored, err := sponsorClient.Workloads(ctx, codersdk.WorkspaceWorkloadsOptions{})
	require.NoError(t, err)
	require.Len(t, sponsored, 1)
	require.Equal(t, solo.ID, sponsored[0].ID)
	err = otherClient.DeleteWorkspaceWorkload(ctx, workspace.ID, solo.ID)
	requireStatus(err, http.StatusNotFound)
	require.NoError(t, sponsorClient.DeleteWorkspaceWorkload(ctx, workspace.ID, solo.ID))

	// The workspace goes with the workload, and the identity with the
	// workspace.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		_, err := adminClient.Workspace(ctx, workspace.ID)
		var sdkErr *codersdk.Error
		if !errors.As(err, &sdkErr) || sdkErr.StatusCode() != http.StatusGone {
			return false
		}
		//nolint:gocritic // The test looks at what no user may read.
		retired, err := db.GetUserByID(dbauthz.AsSystemRestricted(ctx), *solo.UserID)
		return err == nil && retired.Deleted
	}, testutil.IntervalMedium)
	finished, err := sponsorClient.Workloads(ctx, codersdk.WorkspaceWorkloadsOptions{Finished: true})
	require.NoError(t, err)
	require.Len(t, finished, 1)
	require.Equal(t, codersdk.WorkloadStatusRemoved, finished[0].Status)
}
