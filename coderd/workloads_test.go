package coderd_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/x/workloads/workloadstest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// TestRuntimeWorkloads drives a workload from `coder run` to a daemon that
// authenticates as its pod, with a test standing in for the operator and for
// the cluster that signs pod tokens.
func TestRuntimeWorkloads(t *testing.T) {
	t.Parallel()

	const operatorName = "test"

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentRuntimeWorkloads)}
	adminClient, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{DeploymentValues: dv})
	admin := coderdtest.CreateFirstUser(t, adminClient)
	ownerClient, owner := coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)
	otherClient, _ := coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)

	// The workspace has the human's daemon and a shape, as a template with
	// two coder_agent resources would produce.
	shapeToken := uuid.NewString()
	build := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: admin.OrganizationID,
		OwnerID:        owner.ID,
	}).WithAgent(func(agents []*sdkproto.Agent) []*sdkproto.Agent {
		agents[0].Name = "main"
		return append(agents, &sdkproto.Agent{
			Id:   uuid.NewString(),
			Name: "workload",
			Auth: &sdkproto.Agent_Token{Token: shapeToken},
			Env:  map[string]string{codersdk.WorkloadShapeEnv: "true"},
			Scripts: []*sdkproto.Script{{
				DisplayName: "Shape setup",
				Script:      "echo shape",
				RunOnStart:  true,
			}},
		})
	}).Do()
	workspaceID := build.Workspace.ID
	// A second workspace that only an administrator can see.
	dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: admin.OrganizationID,
		OwnerID:        admin.UserID,
	}).Do()
	var shapeAgent database.WorkspaceAgent
	for _, agent := range build.Agents {
		if agent.Name == "workload" {
			shapeAgent = agent
		}
	}
	require.NotEqual(t, uuid.Nil, shapeAgent.ID)

	ctx := testutil.Context(t, testutil.WaitLong)
	cluster := workloadstest.NewCluster(t)
	syncAs := func(client *codersdk.Client, token string, pods ...codersdk.WorkloadOperatorPod) (codersdk.WorkloadOperatorSyncResponse, error) {
		return client.SyncWorkloadOperator(ctx, admin.OrganizationID, operatorName, codersdk.WorkloadOperatorSyncRequest{
			Namespace: workloadstest.Namespace,
			Issuer:    workloadstest.Issuer,
			Audience:  workloadstest.Audience,
			JWKS:      cluster.JWKS(),
			Shapes:    []codersdk.WorkloadOperatorShape{{Deployment: "shape", AgentToken: token}},
			Pods:      pods,
		})
	}
	sync := func(pods ...codersdk.WorkloadOperatorPod) codersdk.WorkloadOperatorSyncResponse {
		t.Helper()
		resp, err := syncAs(adminClient, shapeToken, pods...)
		require.NoError(t, err)
		return resp
	}
	identityOf := func(token string) (codersdk.WorkloadIdentity, error) {
		return agentsdk.New(adminClient.URL, agentsdk.WithFixedToken(token)).Identity(ctx)
	}
	requireStatus := func(err error, status int) {
		t.Helper()
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, status, sdkErr.StatusCode())
	}
	podToken := func(name, uid string) string {
		return cluster.Sign(workloadstest.ValidPodToken(time.Now(), name, uid))
	}

	// Without an operator the workspace has nothing to clone.
	_, err := ownerClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{})
	requireStatus(err, http.StatusConflict)

	// Only an organization administrator may act as an operator, and only
	// an agent that the template marked can be a shape.
	_, err = syncAs(ownerClient, shapeToken)
	requireStatus(err, http.StatusForbidden)
	resp, err := syncAs(adminClient, build.AgentToken)
	require.NoError(t, err)
	require.Len(t, resp.Shapes, 1)
	require.Contains(t, resp.Shapes[0].Error, codersdk.WorkloadShapeEnv)

	resp = sync()
	require.Len(t, resp.Shapes, 1)
	require.Empty(t, resp.Shapes[0].Error)
	require.Equal(t, shapeAgent.ID, resp.Shapes[0].AgentID)
	require.Empty(t, resp.Workloads)

	// A user who cannot update the workspace cannot run workloads in it.
	// Neither can an administrator: a workload acts with its sponsor's
	// permissions, so only the owner may sponsor one in their workspace.
	_, err = otherClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{})
	requireStatus(err, http.StatusNotFound)
	_, err = adminClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{})
	requireStatus(err, http.StatusForbidden)
	_, err = ownerClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{
		Scopes: []codersdk.APIKeyScope{"workspace:fly"},
	})
	requireStatus(err, http.StatusBadRequest)

	// alpha is given no scopes.
	alpha, err := ownerClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{
		Name:    "alpha",
		Command: "echo alpha",
	})
	require.NoError(t, err)
	require.Equal(t, codersdk.WorkloadStatusPending, alpha.Status)
	require.Equal(t, "workload", alpha.Shape)
	require.Equal(t, owner.ID, alpha.RequestedByID)
	require.NotEqual(t, uuid.Nil, alpha.IdentityID, "the identity exists before anything runs")
	require.Equal(t, owner.ID, alpha.SponsorID)
	require.Empty(t, alpha.Scopes)
	_, err = ownerClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{Name: "alpha"})
	requireStatus(err, http.StatusConflict)

	// The operator learns about the workload and has no pod for it yet.
	resp = sync()
	require.Len(t, resp.Workloads, 1)
	require.Equal(t, alpha.ID, resp.Workloads[0].ID)
	require.Empty(t, resp.Workloads[0].PodUID)

	// A pod that merely carries the workload's labels gets no identity.
	const foreignUID, alphaUID = "pod-uid-foreign", "pod-uid-alpha"
	resp = sync(codersdk.WorkloadOperatorPod{WorkloadID: alpha.ID, Name: "impostor", UID: foreignUID})
	require.Empty(t, resp.Workloads[0].PodUID)
	_, err = identityOf(podToken("impostor", foreignUID))
	requireStatus(err, http.StatusUnauthorized)

	// The pod the operator created does.
	alphaPod := codersdk.WorkloadOperatorPod{
		WorkloadID: alpha.ID, Name: "workload-alpha", UID: alphaUID,
		ServiceAccount: workloadstest.ServiceAccount, Created: true,
	}
	resp = sync(alphaPod)
	require.Equal(t, alphaUID, resp.Workloads[0].PodUID)

	listed, err := ownerClient.WorkspaceWorkloads(ctx, workspaceID)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, codersdk.WorkloadStatusStarting, listed[0].Status)
	require.Equal(t, workspaceID.String()+"/pod/"+alphaUID, listed[0].Identity)
	require.True(t, alpha.CreatedAt.Equal(listed[0].CreatedAt), "the launch time does not move when the pod is bound")

	alphaToken := podToken("workload-alpha", alphaUID)
	identity, err := identityOf(alphaToken)
	require.NoError(t, err)
	require.Equal(t, workspaceID.String()+"/pod/"+alphaUID, identity.ID)
	require.Equal(t, codersdk.WorkloadIdentityKindRuntime, identity.Kind)
	require.Equal(t, codersdk.WorkloadAttestationKubernetesPod, identity.Attestation)
	require.Equal(t, alpha.ID, identity.AgentID)
	require.Equal(t, "alpha", identity.AgentName)
	require.Equal(t, owner.ID, identity.OwnerID, "the identity is associated with the workspace owner")
	require.Equal(t, owner.ID, identity.RequestedByID)
	require.Equal(t, "workload", identity.Shape)
	require.Equal(t, alphaUID, identity.Pod.UID)
	require.Equal(t, alpha.IdentityID, identity.IdentityID)
	require.Equal(t, owner.ID, identity.SponsorID)
	require.Empty(t, identity.Scopes)

	// The human's daemon is untouched: its agent token still works and its
	// identity is the declared one.
	human, err := identityOf(build.AgentToken)
	require.NoError(t, err)
	require.Equal(t, codersdk.WorkloadIdentityKindDeclared, human.Kind)
	require.Equal(t, codersdk.WorkloadAttestationAgentToken, human.Attestation)
	require.Equal(t, "main", human.AgentName)
	require.NotEqual(t, identity.ID, human.ID)

	// The daemon connects with the pod token and receives the shape's
	// scripts and environment plus its own command.
	conn, err := agentsdk.New(adminClient.URL, agentsdk.WithFixedToken(alphaToken)).ConnectRPC(ctx)
	require.NoError(t, err)
	defer conn.Close()
	manifest, err := agentproto.NewDRPCAgentClient(conn).GetManifest(ctx, &agentproto.GetManifestRequest{})
	require.NoError(t, err)
	require.Equal(t, "alpha", manifest.AgentName)
	require.Equal(t, shapeAgent.ID[:], manifest.ParentId)
	require.Equal(t, "true", manifest.EnvironmentVariables[codersdk.WorkloadShapeEnv])
	scripts := make(map[string]string, len(manifest.Scripts))
	for _, script := range manifest.Scripts {
		scripts[script.DisplayName] = script.Script
	}
	require.Equal(t, map[string]string{"Shape setup": "echo shape", "Workload command": "echo alpha"}, scripts)

	// Tokens that do not prove alpha's pod are refused.
	for name, token := range map[string]string{
		"UnknownPod":      podToken("workload-beta", "pod-uid-beta"),
		"ForgedSignature": workloadstest.NewCluster(t).Sign(workloadstest.ValidPodToken(time.Now(), "workload-alpha", alphaUID)),
		"WrongAudience": cluster.Sign(func() workloadstest.PodToken {
			tok := workloadstest.ValidPodToken(time.Now(), "workload-alpha", alphaUID)
			tok.Audience = "https://kubernetes.default.svc"
			return tok
		}()),
		"Expired": cluster.Sign(func() workloadstest.PodToken {
			tok := workloadstest.ValidPodToken(time.Now(), "workload-alpha", alphaUID)
			tok.Expiry = time.Now().Add(-time.Minute)
			return tok
		}()),
		"NotPodBound": cluster.Sign(func() workloadstest.PodToken {
			tok := workloadstest.ValidPodToken(time.Now(), "workload-alpha", alphaUID)
			tok.NotPodBound = true
			return tok
		}()),
	} {
		_, err = identityOf(token)
		require.Error(t, err, name)
		requireStatus(err, http.StatusUnauthorized)
	}

	// A second workload gets its own identity, and neither token yields
	// the other. beta may read its sponsor's workspaces and templates.
	betaScopes := []codersdk.APIKeyScope{codersdk.APIKeyScopeWorkspaceRead, codersdk.APIKeyScopeTemplateRead}
	beta, err := ownerClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{
		Name:   "beta",
		Scopes: betaScopes,
	})
	require.NoError(t, err)
	betaPod := codersdk.WorkloadOperatorPod{WorkloadID: beta.ID, Name: "workload-beta", UID: "pod-uid-beta", Created: true}
	sync(alphaPod, betaPod)
	betaToken := podToken("workload-beta", "pod-uid-beta")
	betaIdentity, err := identityOf(betaToken)
	require.NoError(t, err)
	require.Equal(t, beta.ID, betaIdentity.AgentID)
	require.NotEqual(t, alpha.IdentityID, betaIdentity.IdentityID)
	require.Equal(t, betaScopes, betaIdentity.Scopes)
	identity, err = identityOf(alphaToken)
	require.NoError(t, err)
	require.Equal(t, alpha.ID, identity.AgentID)

	// On the user API a workload acts as its sponsor, within its scopes.
	// The sponsor can see one workspace and one template; an administrator
	// can see the second workspace too.
	asAlpha := codersdk.New(adminClient.URL, codersdk.WithSessionToken(alphaToken))
	asBeta := codersdk.New(adminClient.URL, codersdk.WithSessionToken(betaToken))
	sponsorView, err := ownerClient.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	require.Equal(t, 1, sponsorView.Count)
	adminView, err := adminClient.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	require.Equal(t, 2, adminView.Count)
	sponsorTemplates, err := ownerClient.Templates(ctx, codersdk.TemplateFilter{})
	require.NoError(t, err)
	require.NotEmpty(t, sponsorTemplates)

	// Without scopes, alpha can do nothing.
	_, err = asAlpha.User(ctx, codersdk.Me)
	require.Error(t, err)
	alphaView, err := asAlpha.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	require.Zero(t, alphaView.Count)
	_, err = asAlpha.Workspace(ctx, workspaceID)
	requireStatus(err, http.StatusNotFound)
	alphaTemplates, err := asAlpha.Templates(ctx, codersdk.TemplateFilter{})
	require.NoError(t, err)
	require.Empty(t, alphaTemplates)
	_, err = agentsdk.New(adminClient.URL, agentsdk.WithFixedToken(alphaToken)).GitSSHKey(ctx)
	require.Error(t, err, "a workload does not get its sponsor's Git SSH key")

	// With its scopes, beta sees exactly what its sponsor sees: not the
	// administrator's workspace. It can do nothing the scopes do not cover.
	betaView, err := asBeta.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	require.Len(t, betaView.Workspaces, 1)
	require.Equal(t, workspaceID, betaView.Workspaces[0].ID)
	betaTemplates, err := asBeta.Templates(ctx, codersdk.TemplateFilter{})
	require.NoError(t, err)
	require.Len(t, betaTemplates, len(sponsorTemplates))
	_, err = asBeta.User(ctx, codersdk.Me)
	require.Error(t, err)
	err = asBeta.DeleteWorkspaceWorkload(ctx, workspaceID, alpha.ID)
	require.Error(t, err)

	// A workload cannot sponsor another workload, whatever its scopes.
	_, err = asBeta.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{})
	require.Error(t, err)

	// When a pod disappears its identity goes with it, at once.
	resp = sync(alphaPod)
	require.Len(t, resp.Workloads, 1)
	require.Equal(t, alpha.ID, resp.Workloads[0].ID)
	_, err = identityOf(betaToken)
	requireStatus(err, http.StatusUnauthorized)
	_, err = asBeta.Workspaces(ctx, codersdk.WorkspaceFilter{})
	requireStatus(err, http.StatusUnauthorized)

	// Removing a workload revokes it before the operator deletes the pod.
	err = ownerClient.DeleteWorkspaceWorkload(ctx, workspaceID, alpha.ID)
	require.NoError(t, err)
	_, err = identityOf(alphaToken)
	requireStatus(err, http.StatusUnauthorized)
	resp = sync(alphaPod)
	require.Empty(t, resp.Workloads, "the operator is told to delete the pod")
	listed, err = ownerClient.WorkspaceWorkloads(ctx, workspaceID)
	require.NoError(t, err)
	require.Empty(t, listed)
}

// TestRuntimeWorkloadsDisabled checks that nothing changes without the
// experiment: the routes do not exist and a pod token is not a credential.
func TestRuntimeWorkloadsDisabled(t *testing.T) {
	t.Parallel()

	client, db := coderdtest.NewWithDatabase(t, nil)
	admin := coderdtest.CreateFirstUser(t, client)
	build := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: admin.OrganizationID,
		OwnerID:        admin.UserID,
	}).WithAgent().Do()

	ctx := testutil.Context(t, testutil.WaitLong)
	requireStatus := func(err error, status int) {
		t.Helper()
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, status, sdkErr.StatusCode())
	}

	_, err := client.CreateWorkspaceWorkload(ctx, build.Workspace.ID, codersdk.CreateWorkloadRequest{})
	requireStatus(err, http.StatusForbidden)
	_, err = client.SyncWorkloadOperator(ctx, admin.OrganizationID, "test", codersdk.WorkloadOperatorSyncRequest{})
	requireStatus(err, http.StatusForbidden)

	cluster := workloadstest.NewCluster(t)
	token := cluster.Sign(workloadstest.ValidPodToken(time.Now(), "workload-alpha", "pod-uid-alpha"))
	_, err = agentsdk.New(client.URL, agentsdk.WithFixedToken(token)).Identity(ctx)
	requireStatus(err, http.StatusUnauthorized)

	// An ordinary agent token keeps working exactly as before.
	conn, err := agentsdk.New(client.URL, agentsdk.WithFixedToken(build.AgentToken)).ConnectRPC(ctx)
	require.NoError(t, err)
	_ = conn.Close()
}
