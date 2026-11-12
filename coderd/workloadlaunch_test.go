package coderd_test

import (
	"context"
	"errors"
	"net/http"
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

// TestLaunchWorkload covers a workload. Its identity is an account of its
// own that owns the workload's workspace, shares it with the sponsor and
// has nothing of the sponsor's. The workload acts with a token of its own.
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

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	//nolint:gocritic // The test sets up and looks at what no user may.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	// The template is open to the sponsor and to nobody else: not to all
	// members, and so not to the workload's identity either.
	adminUser, err := adminClient.User(ctx, codersdk.Me)
	require.NoError(t, err)
	require.NoError(t, db.UpdateTemplateACLByID(dbauthz.As(ctx, coderdtest.AuthzUserSubject(adminUser)), database.UpdateTemplateACLByIDParams{
		ID:       template.ID,
		GroupACL: database.TemplateACL{},
		UserACL:  database.TemplateACL{sponsor.ID.String(): {policy.ActionRead, policy.ActionUse}},
	}))

	// A workspace of the sponsor's own, which the workload must not reach.
	sponsorBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: admin.OrganizationID,
		OwnerID:        sponsor.ID,
	}).WithAgent().Do()

	requireStatus := func(err error, status int) {
		t.Helper()
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, status, sdkErr.StatusCode())
	}

	// What is asked for is checked before anything is made.
	_, err = sponsorClient.LaunchWorkload(ctx, codersdk.LaunchWorkloadRequest{TemplateID: template.ID})
	requireStatus(err, http.StatusBadRequest)
	_, err = sponsorClient.LaunchWorkload(ctx, codersdk.LaunchWorkloadRequest{
		TemplateID: template.ID, Command: "true", Scopes: []codersdk.APIKeyScope{codersdk.APIKeyScopeAll},
	})
	requireStatus(err, http.StatusBadRequest)
	_, err = sponsorClient.LaunchWorkload(ctx, codersdk.LaunchWorkloadRequest{TemplateID: uuid.New(), Command: "true"})
	requireStatus(err, http.StatusNotFound)
	// Somebody who may not use the template cannot sponsor a workload in
	// a workspace of it.
	_, err = otherClient.LaunchWorkload(ctx, codersdk.LaunchWorkloadRequest{TemplateID: template.ID, Command: "true"})
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
			Name:       "solo",
			Command:    "echo solo",
			Scopes:     []codersdk.APIKeyScope{codersdk.APIKeyScopeWorkspaceRead, codersdk.APIKeyScopeTemplateRead},
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
		Logger:      testutil.Logger(t).Named("workloads"),
		Runtime:     runtime,
		Pending:     daemonClient.PendingWorkloads,
		ReportStart: daemonClient.ReportWorkloadStart,
		ReportExit:  daemonClient.ReportWorkloadExit,
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
	// cannot sign in. Its one API key is the workload's token, which names
	// the identity: the build gave the account no session token.
	require.Equal(t, *solo.UserID, workspace.OwnerID)
	require.NotEqual(t, sponsor.ID, workspace.OwnerID)
	identityUser, err := adminClient.User(ctx, solo.UserID.String())
	require.NoError(t, err)
	require.True(t, identityUser.IsServiceAccount)
	require.Equal(t, codersdk.LoginTypeNone, identityUser.LoginType)
	require.Equal(t, workspace.OwnerName, identityUser.Username)
	identityKeys := func() []database.APIKey {
		var all []database.APIKey
		for _, loginType := range []database.LoginType{database.LoginTypeNone, database.LoginTypeToken, database.LoginTypePassword} {
			keys, err := db.GetAPIKeysByUserID(sysCtx, database.GetAPIKeysByUserIDParams{
				LoginType: loginType, UserID: *solo.UserID, IncludeExpired: true,
			})
			require.NoError(t, err)
			all = append(all, keys...)
		}
		return all
	}
	keys := identityKeys()
	require.Len(t, keys, 1)
	require.Equal(t, codersdk.WorkloadTokenNamePrefix+solo.ID.String(), keys[0].TokenName)
	require.ElementsMatch(t, database.APIKeyScopes{database.ApiKeyScopeWorkspaceRead, database.ApiKeyScopeTemplateRead}, keys[0].Scopes)

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

	// The workload holds a token of its own account, narrowed to its
	// scopes. It can read its own workspace. It cannot read its sponsor's,
	// which the sponsor could, nor anything its scopes do not name.
	sandbox := runtime.sandbox(solo.ID)
	require.NotNil(t, sandbox)
	require.NotEmpty(t, sandbox.token)
	asSolo := codersdk.New(adminClient.URL, codersdk.WithSessionToken(sandbox.token))
	identity, err := asSolo.WorkloadIdentity(ctx)
	require.NoError(t, err)
	require.Equal(t, codersdk.WorkloadIdentityKindWorkload, identity.Kind)
	require.Equal(t, solo.ID, identity.IdentityID)
	require.Equal(t, sponsor.ID, identity.SponsorID)
	require.Equal(t, workspace.OwnerID, identity.OwnerID)
	require.Equal(t, identityUser.Username, identity.OwnerUsername)
	// Reading a workspace takes reading its template, and the identity is
	// not among those the template is open to.
	_, err = asSolo.Workspace(ctx, workspace.ID)
	requireStatus(err, http.StatusForbidden)
	require.NoError(t, db.UpdateTemplateACLByID(dbauthz.As(ctx, coderdtest.AuthzUserSubject(adminUser)), database.UpdateTemplateACLByIDParams{
		ID:       template.ID,
		GroupACL: database.TemplateACL{admin.OrganizationID.String(): {policy.ActionRead, policy.ActionUse}},
		UserACL:  database.TemplateACL{},
	}))
	own, err := asSolo.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	require.Equal(t, workspace.ID, own.ID)
	_, err = asSolo.Workspace(ctx, sponsorBuild.Workspace.ID)
	requireStatus(err, http.StatusNotFound)
	_, err = asSolo.User(ctx, sponsor.ID.String())
	require.Error(t, err)
	// A workload does not sponsor workloads.
	_, err = asSolo.LaunchWorkload(ctx, codersdk.LaunchWorkloadRequest{TemplateID: template.ID, Command: "true"})
	requireStatus(err, http.StatusForbidden)
	// The daemon's own credential is no user credential, and neither is
	// somebody's token that merely names the identity.
	_, err = codersdk.New(adminClient.URL, codersdk.WithSessionToken(agentToken)).WorkloadIdentity(ctx)
	requireStatus(err, http.StatusUnauthorized)
	impostor, err := otherClient.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		TokenName: codersdk.WorkloadTokenNamePrefix + solo.ID.String(),
	})
	require.NoError(t, err)
	_, err = codersdk.New(adminClient.URL, codersdk.WithSessionToken(impostor.Key)).WorkloadIdentity(ctx)
	requireStatus(err, http.StatusUnauthorized)

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
	// The workload's token went with it.
	_, err = asSolo.WorkloadIdentity(ctx)
	requireStatus(err, http.StatusUnauthorized)
	require.Empty(t, identityKeys())
	finished, err := sponsorClient.Workloads(ctx, codersdk.WorkspaceWorkloadsOptions{Finished: true})
	require.NoError(t, err)
	require.Len(t, finished, 1)
	require.Equal(t, codersdk.WorkloadStatusRemoved, finished[0].Status)
}
