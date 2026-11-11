package coderd_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/agent/agentworkload"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
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

// AttachCommand stands in for a sandbox's terminal with a shell that says
// where it is and what it was told.
func (s *fakeSandbox) AttachCommand() (string, []string) {
	return "sh", []string{"-c", "echo terminal of " + s.id + "; read line; echo heard $line"}
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
	auditor := audit.NewMock()
	adminClient, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{DeploymentValues: dv, Auditor: auditor})
	admin := coderdtest.CreateFirstUser(t, adminClient)
	ownerClient, owner := coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)
	otherClient, _ := coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)

	// The workspace has two daemons. Only one of them runs workloads, and
	// coderd finds out which.
	build := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: admin.OrganizationID,
		OwnerID:        owner.ID,
	}).WithAgent(func(agents []*sdkproto.Agent) []*sdkproto.Agent {
		agents[0].Name = "sandbox"
		return append(agents, &sdkproto.Agent{
			Id:   uuid.NewString(),
			Name: "main",
			Auth: &sdkproto.Agent_Token{Token: uuid.NewString()},
		})
	}).Do()
	workspaceID := build.Workspace.ID
	// A second workspace, with its own daemon, that only an administrator
	// can see.
	adminBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: admin.OrganizationID,
		OwnerID:        admin.UserID,
	}).WithAgent().Do()

	// What the template lets the workspace share with workloads, and
	// things it does not.
	shared, private := t.TempDir(), t.TempDir()
	project := filepath.Join(shared, "project")
	require.NoError(t, os.Mkdir(project, 0o700))
	require.NoError(t, os.Symlink(private, filepath.Join(shared, "way-out")))

	runtime := &fakeRuntime{t: t, sandboxes: make(map[uuid.UUID]*fakeSandbox)}
	supervisor, err := agentworkload.New(context.Background(), agentworkload.Options{
		Logger:  testutil.Logger(t).Named("workloads"),
		Runtime: runtime,
		Policy: agentworkload.Policy{
			DefaultImage:   "registry.example.com/agent:1",
			AllowedImages:  []string{"registry.example.com/tools/*"},
			DefaultNetwork: codersdk.WorkloadNetworkNone,
			AllowedMounts:  []string{shared},
		},
		CoderURL:     adminClient.URL,
		SessionToken: func() string { return build.AgentToken },
		ReportExit:   agentsdk.New(adminClient.URL, agentsdk.WithFixedToken(build.AgentToken)).ReportWorkloadExit,
	})
	require.NoError(t, err)
	_ = agenttest.New(t, adminClient.URL, build.AgentToken, func(o *agent.Options) {
		o.Workloads = supervisor
	})
	coderdtest.NewWorkspaceAgentWaiter(t, adminClient, workspaceID).AgentNames([]string{"sandbox"}).Wait()

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
	// A workload exists to have less than its sponsor.
	for _, everything := range []codersdk.APIKeyScope{codersdk.APIKeyScopeAll, codersdk.APIKeyScopeCoderAll} {
		_, err = ownerClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{
			Command: "true", Scopes: []codersdk.APIKeyScope{everything},
		})
		requireStatus(err, http.StatusBadRequest)
	}
	// The template, through the daemon's policy, decides what a sandbox
	// may be. A refused workload is on record as failed.
	_, err = ownerClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{
		Name: "refused", Command: "true", Image: "docker.io/evil/image",
	})
	requireStatus(err, http.StatusBadGateway)
	_, err = ownerClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{
		Name: "refused-too", Command: "true", Network: codersdk.WorkloadNetworkOutbound,
	})
	requireStatus(err, http.StatusBadGateway)
	// A workload sees nothing of the workspace unless a directory the
	// template allows is shared with it. A link does not lead out.
	for _, path := range []string{private, filepath.Join(shared, "way-out"), filepath.Join(shared, "..", filepath.Base(private)), "relative"} {
		_, err = ownerClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{
			Command: "true", Mounts: []codersdk.WorkloadMount{{Path: path}},
		})
		requireStatus(err, http.StatusBadGateway)
	}

	// alpha is given no scopes. beta may read workspaces and templates.
	alpha, err := ownerClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{
		Name: "alpha", Command: "echo alpha",
	})
	require.NoError(t, err)
	require.Equal(t, codersdk.WorkloadStatusRunning, alpha.Status)
	require.Equal(t, "sandbox", alpha.AgentName, "coderd picked the daemon that runs workloads")
	require.Equal(t, "registry.example.com/agent:1", alpha.Image)
	require.Equal(t, codersdk.WorkloadNetworkNone, alpha.Network)
	require.Equal(t, owner.ID, alpha.SponsorID)
	require.Empty(t, alpha.Scopes)
	require.Equal(t, &codersdk.WorkloadSandbox{Runtime: "fake", ID: "sandbox-alpha"}, alpha.Sandbox)
	require.Equal(t, workspaceID.String()+"/workload/"+alpha.ID.String(), alpha.Identity)
	_, err = ownerClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{Name: "alpha", Command: "true"})
	requireStatus(err, http.StatusConflict)

	betaScopes := []codersdk.APIKeyScope{codersdk.APIKeyScopeWorkspaceRead, codersdk.APIKeyScopeTemplateRead}
	beta, err := ownerClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{
		Name: "beta", Command: "echo beta", Scopes: betaScopes, Image: "registry.example.com/tools/git:2",
		Mounts: []codersdk.WorkloadMount{{Path: project, Writable: true}, {Path: shared}},
	})
	require.NoError(t, err)
	require.Equal(t, "registry.example.com/tools/git:2", beta.Image)
	require.Equal(t, []codersdk.WorkloadMount{{Path: project, Writable: true}, {Path: shared}}, beta.Mounts)
	require.Empty(t, alpha.Mounts)
	asAlpha, asBeta := asWorkload(alpha), asWorkload(beta)

	listed, err := ownerClient.WorkspaceWorkloads(ctx, workspaceID, codersdk.WorkspaceWorkloadsOptions{})
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
	follow := func(id uuid.UUID, onOutput func(string)) ([]string, int) {
		t.Helper()
		var output []string
		exitCode := -1
		streamed := make(chan error, 1)
		go func() {
			streamed <- ownerClient.WorkspaceWorkloadLogs(ctx, workspaceID, id, func(event codersdk.WorkloadLogEvent) error {
				if event.ExitCode != nil {
					exitCode = *event.ExitCode
					return nil
				}
				output = append(output, event.Output)
				if onOutput != nil {
					onOutput(event.Output)
				}
				return nil
			})
		}()
		require.NoError(t, testutil.TryReceive(ctx, t, streamed))
		return output, exitCode
	}
	output, exitCode := follow(alpha.ID, func(string) { runtime.sandbox(alpha.ID).exit(3) })
	require.Equal(t, []string{"ran: echo alpha"}, output)
	require.Equal(t, 3, exitCode)
	_, err = asAlpha.WorkloadIdentity(ctx)
	requireStatus(err, http.StatusUnauthorized)
	_, err = asAlpha.Workspaces(ctx, codersdk.WorkspaceFilter{})
	requireStatus(err, http.StatusUnauthorized)
	listed, err = ownerClient.WorkspaceWorkloads(ctx, workspaceID, codersdk.WorkspaceWorkloadsOptions{})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, beta.ID, listed[0].ID)

	// A workload with a write scope acts as its sponsor, and the audit log
	// says that the workload did it.
	writer, err := ownerClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{
		Name: "writer", Command: "true", Scopes: []codersdk.APIKeyScope{codersdk.APIKeyScopeWorkspaceUpdate, codersdk.APIKeyScopeWorkspaceRead},
	})
	require.NoError(t, err)
	_ = asWorkload(writer).UpdateWorkspaceAutostart(ctx, workspaceID, codersdk.UpdateWorkspaceAutostartRequest{})
	_ = ownerClient.UpdateWorkspaceAutostart(ctx, workspaceID, codersdk.UpdateWorkspaceAutostartRequest{})

	// The sponsor can have a terminal in an interactive workload. It is
	// the workload's terminal: nobody else gets one.
	shell, err := ownerClient.CreateWorkspaceWorkload(ctx, workspaceID, codersdk.CreateWorkloadRequest{
		Name: "shell", Command: "sh", Interactive: true,
	})
	require.NoError(t, err)
	_, err = adminClient.AttachWorkspaceWorkload(ctx, workspaceID, shell.ID, 24, 80)
	requireStatus(err, http.StatusForbidden)
	_, err = ownerClient.AttachWorkspaceWorkload(ctx, workspaceID, beta.ID, 24, 80)
	requireStatus(err, http.StatusBadGateway)
	terminal, err := ownerClient.AttachWorkspaceWorkload(ctx, workspaceID, shell.ID, 24, 80)
	require.NoError(t, err)
	t.Cleanup(func() { _ = terminal.Close() })
	require.NoError(t, json.NewEncoder(terminal).Encode(map[string]string{"data": "hello\r"}))
	var seen strings.Builder
	buf := make([]byte, 1024)
	for !strings.Contains(seen.String(), "heard hello") {
		require.NoError(t, terminal.SetReadDeadline(time.Now().Add(testutil.WaitShort)))
		n, err := terminal.Read(buf)
		require.NoError(t, err, "terminal output so far: %q", seen.String())
		_, _ = seen.Write(buf[:n])
	}
	require.Contains(t, seen.String(), "terminal of sandbox-shell")
	require.NoError(t, ownerClient.DeleteWorkspaceWorkload(ctx, workspaceID, shell.ID))

	// A workload has its sponsor's permissions only while the sponsor has
	// them: suspending the sponsor stops the workload's access at once.
	_, err = adminClient.UpdateUserStatus(ctx, owner.ID.String(), codersdk.UserStatusSuspended)
	require.NoError(t, err)
	_, err = asBeta.Workspaces(ctx, codersdk.WorkspaceFilter{})
	requireStatus(err, http.StatusUnauthorized)
	_, err = adminClient.UpdateUserStatus(ctx, owner.ID.String(), codersdk.UserStatusActive)
	require.NoError(t, err)
	_, err = asBeta.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)

	// A workload that ends without anyone following it is still recorded:
	// its daemon reports the end.
	runtime.sandbox(writer.ID).exit(0)
	require.Eventually(t, func() bool {
		return direct(workspaces, build.AgentToken, writer.ID.String()) == http.StatusUnauthorized
	}, testutil.WaitShort, testutil.IntervalFast)

	// Removing a workload ends its identity and stops its sandbox.
	require.NoError(t, ownerClient.DeleteWorkspaceWorkload(ctx, workspaceID, beta.ID))
	select {
	case <-runtime.sandbox(beta.ID).done:
	case <-ctx.Done():
		t.Fatal("the sandbox was not stopped")
	}
	require.Equal(t, http.StatusUnauthorized, direct(workspaces, build.AgentToken, beta.ID.String()))
	listed, err = ownerClient.WorkspaceWorkloads(ctx, workspaceID, codersdk.WorkspaceWorkloadsOptions{})
	require.NoError(t, err)
	require.Empty(t, listed)

	// What ran stays on record, with what became of it.
	var finished map[string]codersdk.Workload
	require.Eventually(t, func() bool {
		all, err := ownerClient.WorkspaceWorkloads(ctx, workspaceID, codersdk.WorkspaceWorkloadsOptions{Finished: true})
		if err != nil {
			return false
		}
		finished = make(map[string]codersdk.Workload, len(all))
		for _, workload := range all {
			finished[workload.Name] = workload
		}
		// The daemon's report carries the exit code of the removed one.
		return len(finished) == 10 && finished["beta"].ExitCode != nil
	}, testutil.WaitShort, testutil.IntervalFast)
	require.Equal(t, codersdk.WorkloadStatusExited, finished["alpha"].Status)
	require.Equal(t, 3, *finished["alpha"].ExitCode)
	require.NotNil(t, finished["alpha"].FinishedAt)
	require.Equal(t, "echo alpha", finished["alpha"].Command)
	require.Equal(t, codersdk.WorkloadStatusExited, finished["writer"].Status)
	require.Equal(t, 0, *finished["writer"].ExitCode)
	require.Equal(t, codersdk.WorkloadStatusRemoved, finished["beta"].Status)
	require.Equal(t, betaScopes, finished["beta"].Scopes)
	require.Equal(t, codersdk.WorkloadStatusFailed, finished["refused"].Status)
	require.Equal(t, codersdk.WorkloadStatusFailed, finished["refused-too"].Status)
	// The output of a finished workload can still be read.
	output, exitCode = follow(writer.ID, nil)
	require.Equal(t, []string{"ran: true"}, output)
	require.Equal(t, 0, exitCode)

	// The audit log tells what ran, for whom, with which permissions, and
	// how it ended.
	type entry struct {
		action database.AuditAction
		status int32
		fields map[string]any
		user   uuid.UUID
	}
	workloadEntries := map[string][]entry{}
	var workspaceWrites []entry
	for _, alog := range auditor.AuditLogs() {
		var fields map[string]any
		require.NoError(t, json.Unmarshal(alog.AdditionalFields, &fields))
		e := entry{action: alog.Action, status: alog.StatusCode, fields: fields, user: alog.UserID}
		switch alog.ResourceType {
		case database.ResourceTypeWorkloadIdentity:
			require.Equal(t, admin.OrganizationID, alog.OrganizationID)
			workloadEntries[alog.ResourceTarget] = append(workloadEntries[alog.ResourceTarget], e)
		case database.ResourceTypeWorkspace:
			workspaceWrites = append(workspaceWrites, e)
		default:
		}
	}
	require.Len(t, workloadEntries["alpha"], 2)
	started, stopped := workloadEntries["alpha"][0], workloadEntries["alpha"][1]
	require.Equal(t, database.AuditActionStart, started.action)
	require.EqualValues(t, http.StatusCreated, started.status)
	require.Equal(t, owner.ID, started.user)
	require.Equal(t, "sandbox", started.fields["workspace_agent"])
	require.Equal(t, alpha.ID.String(), started.fields["workload_identity_id"])
	require.Equal(t, database.AuditActionStop, stopped.action)
	require.Equal(t, "exited", stopped.fields["reason"])
	require.EqualValues(t, 3, stopped.fields["exit_code"])
	require.Equal(t, false, stopped.fields["ended_by_user"])
	require.Equal(t, owner.ID, stopped.user, "an ending nobody caused is recorded under the sponsor")

	require.Len(t, workloadEntries["beta"], 2)
	require.Equal(t, "removed", workloadEntries["beta"][1].fields["reason"])
	require.Equal(t, true, workloadEntries["beta"][1].fields["ended_by_user"])

	// So is the fact that a person had a terminal in a workload.
	require.Len(t, workloadEntries["shell"], 3)
	require.Equal(t, database.AuditActionConnect, workloadEntries["shell"][1].action)
	require.Equal(t, owner.ID, workloadEntries["shell"][1].user)

	// A workload that was refused is on record too.
	require.Len(t, workloadEntries["refused"], 2)
	require.EqualValues(t, http.StatusBadGateway, workloadEntries["refused"][0].status)
	require.Equal(t, "failed", workloadEntries["refused"][1].fields["reason"])

	// The workspace was changed twice: once by the workload, once by its
	// sponsor in person. Only the first says so.
	require.Len(t, workspaceWrites, 2)
	require.Equal(t, writer.ID.String(), workspaceWrites[0].fields[audit.WorkloadIdentityField])
	require.Equal(t, owner.ID, workspaceWrites[0].user)
	require.NotContains(t, workspaceWrites[1].fields, audit.WorkloadIdentityField)
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
