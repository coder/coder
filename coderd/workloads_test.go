package coderd_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/agent/agentworkload"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

// fakeRuntime stands in for a sandbox runtime. Its sandboxes run nothing:
// the test plays the workload by using the channel the runtime was given.
type fakeRuntime struct {
	t         testing.TB
	mu        sync.Mutex
	sandboxes map[uuid.UUID]*fakeSandbox
}

type fakeSandbox struct {
	id      string
	command string
	// server serves the sandbox's channel, as a runtime does on a socket
	// or a private network.
	server *httptest.Server
	once   sync.Once
	done   chan struct{}
	code   int
}

func (*fakeRuntime) Name() string { return "fake" }

func (f *fakeRuntime) Start(_ context.Context, spec agentworkload.Spec, channel http.Handler) (agentworkload.Sandbox, error) {
	sandbox := &fakeSandbox{
		id:      "sandbox-" + spec.Name,
		command: spec.Command,
		server:  httptest.NewServer(channel),
		done:    make(chan struct{}),
	}
	f.t.Cleanup(sandbox.server.Close)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sandboxes[spec.ID] = sandbox
	return sandbox, nil
}

func (*fakeRuntime) Sweep(context.Context) error { return nil }

func (f *fakeRuntime) sandbox(id uuid.UUID) *fakeSandbox {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sandboxes[id]
}

func (s *fakeSandbox) ID() string { return s.id }

func (s *fakeSandbox) exit(code int) {
	s.once.Do(func() {
		s.code = code
		close(s.done)
	})
}

func (s *fakeSandbox) Logs(ctx context.Context, w io.Writer) error {
	if _, err := io.WriteString(w, "ran: "+s.command+"\n"); err != nil {
		return err
	}
	select {
	case <-s.done:
	case <-ctx.Done():
	}
	return nil
}

func (s *fakeSandbox) Wait(ctx context.Context) (int, error) {
	select {
	case <-s.done:
		return s.code, nil
	case <-ctx.Done():
		return -1, ctx.Err()
	}
}

func (s *fakeSandbox) Close(context.Context) error {
	s.exit(137)
	s.server.Close()
	return nil
}

// TestWorkloads runs workloads in a real workspace daemon with a fake
// sandbox runtime, and plays the workloads through their channels.
func TestWorkloads(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentRuntimeWorkloads)}
	adminClient, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{DeploymentValues: dv})
	admin := coderdtest.CreateFirstUser(t, adminClient)
	ownerClient, owner := coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)
	otherClient, _ := coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)

	build := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: admin.OrganizationID,
		OwnerID:        owner.ID,
	}).WithAgent().Do()
	workspaceID := build.Workspace.ID
	// A second workspace, with its own daemon, that only an administrator
	// can see.
	adminBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: admin.OrganizationID,
		OwnerID:        admin.UserID,
	}).WithAgent().Do()

	runtime := &fakeRuntime{t: t, sandboxes: make(map[uuid.UUID]*fakeSandbox)}
	supervisor, err := agentworkload.New(context.Background(), agentworkload.Options{
		Logger:       testutil.Logger(t).Named("workloads"),
		Runtime:      runtime,
		CoderURL:     adminClient.URL,
		SessionToken: func() string { return build.AgentToken },
	})
	require.NoError(t, err)
	_ = agenttest.New(t, adminClient.URL, build.AgentToken, func(o *agent.Options) {
		o.Workloads = supervisor
	})
	coderdtest.NewWorkspaceAgentWaiter(t, adminClient, workspaceID).Wait()

	ctx := testutil.Context(t, testutil.WaitLong)
	requireStatus := func(err error, status int) {
		t.Helper()
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, status, sdkErr.StatusCode())
	}
	// asWorkload returns a client that has what a process in the sandbox
	// has: the channel, and no credential.
	asWorkload := func(workload codersdk.Workload) *codersdk.Client {
		t.Helper()
		sandbox := runtime.sandbox(workload.ID)
		require.NotNil(t, sandbox)
		u, err := url.Parse(sandbox.server.URL)
		require.NoError(t, err)
		return codersdk.New(u)
	}
	// direct sends a request straight to coderd, as something that
	// bypasses the daemon would.
	direct := func(path, token, identity string) int {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, adminClient.URL.String()+path, nil)
		require.NoError(t, err)
		if token != "" {
			req.Header.Set(codersdk.SessionTokenHeader, token)
		}
		if identity != "" {
			req.Header.Set(codersdk.WorkloadIdentityHeader, identity)
		}
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()
		return res.StatusCode
	}

	// Only the owner can run a workload in a workspace, and only with a
	// command and scopes that exist.
	_, err = otherClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{Command: "true"})
	requireStatus(err, http.StatusNotFound)
	_, err = adminClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{Command: "true"})
	requireStatus(err, http.StatusForbidden)
	_, err = ownerClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{})
	requireStatus(err, http.StatusBadRequest)
	_, err = ownerClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{
		Command: "true", Scopes: []codersdk.APIKeyScope{"workspace:fly"},
	})
	requireStatus(err, http.StatusBadRequest)

	// alpha is given no scopes. beta may read workspaces and templates.
	alpha, err := ownerClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{
		Name: "alpha", Command: "echo alpha",
	})
	require.NoError(t, err)
	require.Equal(t, codersdk.WorkloadStatusRunning, alpha.Status)
	require.Equal(t, owner.ID, alpha.SponsorID)
	require.Empty(t, alpha.Scopes)
	require.Equal(t, &codersdk.WorkloadSandbox{Runtime: "fake", ID: "sandbox-alpha"}, alpha.Sandbox)
	require.Equal(t, workspaceID.String()+"/workload/"+alpha.ID.String(), alpha.Identity)
	_, err = ownerClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{Name: "alpha", Command: "true"})
	requireStatus(err, http.StatusConflict)

	betaScopes := []codersdk.APIKeyScope{codersdk.APIKeyScopeWorkspaceRead, codersdk.APIKeyScopeTemplateRead}
	beta, err := ownerClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{
		Name: "beta", Command: "echo beta", Scopes: betaScopes,
	})
	require.NoError(t, err)
	asAlpha, asBeta := asWorkload(alpha), asWorkload(beta)

	listed, err := ownerClient.WorkspaceWorkloads(ctx, workspaceID)
	require.NoError(t, err)
	require.Len(t, listed, 2)

	// Each workload is itself and holds no credential.
	identity, err := asAlpha.WorkloadIdentity(ctx)
	require.NoError(t, err)
	require.Equal(t, codersdk.WorkloadIdentityKindWorkload, identity.Kind)
	require.Equal(t, codersdk.WorkloadAttestationWorkspaceDaemon, identity.Attestation)
	require.Equal(t, alpha.ID, identity.IdentityID)
	require.Equal(t, alpha.Identity, identity.ID)
	require.Equal(t, "alpha", identity.Name)
	require.Equal(t, owner.ID, identity.SponsorID)
	require.Empty(t, identity.Scopes)
	betaIdentity, err := asBeta.WorkloadIdentity(ctx)
	require.NoError(t, err)
	require.Equal(t, beta.ID, betaIdentity.IdentityID)
	require.Equal(t, betaScopes, betaIdentity.Scopes)

	// alpha claims to be beta every way a request can. It is still alpha.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		asAlpha.URL.String()+"/api/v2/workspaceagents/me/identity?coder_session_token="+build.AgentToken+"&access_token=x", nil)
	require.NoError(t, err)
	req.Header.Set(codersdk.WorkloadIdentityHeader, beta.ID.String())
	req.Header.Set(codersdk.SessionTokenHeader, "forged")
	req.Header.Set("Authorization", "Bearer forged")
	req.AddCookie(&http.Cookie{Name: codersdk.SessionTokenCookie, Value: "forged"})
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	var claimed codersdk.WorkloadIdentity
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.NoError(t, codersdk.ReadBodyAsJSON(res, &claimed))
	_ = res.Body.Close()
	require.Equal(t, alpha.ID, claimed.IdentityID)

	// The human's daemon is a different principal.
	human, err := agentsdk.New(adminClient.URL, agentsdk.WithFixedToken(build.AgentToken)).Identity(ctx)
	require.NoError(t, err)
	require.Equal(t, codersdk.WorkloadIdentityKindDaemon, human.Kind)
	require.Equal(t, codersdk.WorkloadAttestationAgentToken, human.Attestation)
	require.NotEqual(t, identity.ID, human.ID)

	// The sponsor sees one workspace; an administrator sees both.
	sponsorView, err := ownerClient.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	require.Len(t, sponsorView.Workspaces, 1)
	adminView, err := adminClient.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	require.Len(t, adminView.Workspaces, 2)
	sponsorTemplates, err := ownerClient.Templates(ctx, codersdk.TemplateFilter{})
	require.NoError(t, err)
	require.NotEmpty(t, sponsorTemplates)

	// Without scopes, alpha can do nothing. It does not inherit what its
	// sponsor, its daemon or beta may do.
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

	// A workload cannot use its daemon's API, although its requests carry
	// the daemon's token by the time they reach coderd.
	for _, client := range []*codersdk.Client{asAlpha, asBeta} {
		for _, path := range []string{"/api/v2/workspaceagents/me/gitsshkey", "/api/v2/workspaceagents/me/rpc"} {
			res, err := client.Request(ctx, http.MethodGet, path, nil)
			require.NoError(t, err)
			_ = res.Body.Close()
			require.Equal(t, http.StatusForbidden, res.StatusCode, path)
		}
	}

	// With its scopes, beta sees exactly what its sponsor sees, and can do
	// nothing the scopes do not cover.
	betaView, err := asBeta.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	require.Len(t, betaView.Workspaces, 1)
	require.Equal(t, workspaceID, betaView.Workspaces[0].ID)
	betaTemplates, err := asBeta.Templates(ctx, codersdk.TemplateFilter{})
	require.NoError(t, err)
	require.Len(t, betaTemplates, len(sponsorTemplates))
	_, err = asBeta.User(ctx, codersdk.Me)
	require.Error(t, err)
	_, err = asBeta.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{Command: "true"})
	require.Error(t, err)
	err = asBeta.DeleteWorkspaceWorkload(ctx, workspaceID, alpha.ID)
	require.Error(t, err)

	// Nothing but the workload's own daemon can vouch for it.
	const workspaces = "/api/v2/workspaces"
	require.Equal(t, http.StatusUnauthorized, direct(workspaces, "", beta.ID.String()), "no credential")
	require.Equal(t, http.StatusUnauthorized, direct(workspaces, uuid.NewString(), beta.ID.String()), "unknown agent token")
	require.Equal(t, http.StatusUnauthorized, direct(workspaces, adminBuild.AgentToken, beta.ID.String()), "another workspace's daemon")
	require.Equal(t, http.StatusUnauthorized, direct(workspaces, build.AgentToken, uuid.NewString()), "unknown identity")
	require.Equal(t, http.StatusUnauthorized, direct(workspaces, build.AgentToken, ""), "an agent token is not a user credential")
	// A user who sends the header is still just that user.
	require.Equal(t, http.StatusOK, direct("/api/v2/users/me", ownerClient.SessionToken(), beta.ID.String()))
	require.Equal(t, http.StatusUnauthorized,
		direct("/api/v2/workspaceagents/me/identity", ownerClient.SessionToken(), beta.ID.String()))

	// The command's output and exit code reach the sponsor. Once the
	// command has exited, the identity is gone.
	var output []string
	exitCode := -1
	streamed := make(chan error, 1)
	go func() {
		streamed <- ownerClient.WorkspaceWorkloadLogs(ctx, workspaceID, alpha.ID, func(event codersdk.WorkloadLogEvent) error {
			if event.ExitCode != nil {
				exitCode = *event.ExitCode
				return nil
			}
			output = append(output, event.Output)
			if event.Output == "ran: echo alpha" {
				runtime.sandbox(alpha.ID).exit(3)
			}
			return nil
		})
	}()
	require.NoError(t, testutil.TryReceive(ctx, t, streamed))
	require.Equal(t, []string{"ran: echo alpha"}, output)
	require.Equal(t, 3, exitCode)
	_, err = asAlpha.WorkloadIdentity(ctx)
	requireStatus(err, http.StatusUnauthorized)
	_, err = asAlpha.Workspaces(ctx, codersdk.WorkspaceFilter{})
	requireStatus(err, http.StatusUnauthorized)
	listed, err = ownerClient.WorkspaceWorkloads(ctx, workspaceID)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, beta.ID, listed[0].ID)

	// Removing a workload revokes it and stops its sandbox.
	require.NoError(t, ownerClient.DeleteWorkspaceWorkload(ctx, workspaceID, beta.ID))
	select {
	case <-runtime.sandbox(beta.ID).done:
	case <-ctx.Done():
		t.Fatal("the sandbox was not stopped")
	}
	require.Equal(t, http.StatusUnauthorized, direct(workspaces, build.AgentToken, beta.ID.String()))
	listed, err = ownerClient.WorkspaceWorkloads(ctx, workspaceID)
	require.NoError(t, err)
	require.Empty(t, listed)
}

// TestWorkloadsDisabled checks that nothing changes without the experiment.
func TestWorkloadsDisabled(t *testing.T) {
	t.Parallel()

	client, db := coderdtest.NewWithDatabase(t, nil)
	admin := coderdtest.CreateFirstUser(t, client)
	build := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: admin.OrganizationID,
		OwnerID:        admin.UserID,
	}).WithAgent().Do()

	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := client.CreateWorkspaceWorkload(ctx, build.Workspace.ID, codersdk.CreateWorkloadRequest{Command: "true"})
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())

	// The header is inert: an agent token is still no user credential, and
	// the daemon's API still refuses requests that carry it.
	for _, path := range []string{"/api/v2/workspaces", "/api/v2/workspaceagents/me/identity", "/api/v2/workspaceagents/me/gitsshkey"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.URL.String()+path, nil)
		require.NoError(t, err)
		req.Header.Set(codersdk.SessionTokenHeader, build.AgentToken)
		req.Header.Set(codersdk.WorkloadIdentityHeader, uuid.NewString())
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = res.Body.Close()
		require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, res.StatusCode, path)
	}

	// An ordinary agent token keeps working exactly as before.
	_, err = agentsdk.New(client.URL, agentsdk.WithFixedToken(build.AgentToken)).GitSSHKey(ctx)
	require.NoError(t, err)
}
