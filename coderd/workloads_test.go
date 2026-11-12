package coderd_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentworkload"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

// fakeRuntime stands in for a workload runtime. Its sandboxes run nothing:
// the test plays the workload with the token the runtime was given.
type fakeRuntime struct {
	t         testing.TB
	mu        sync.Mutex
	sandboxes map[uuid.UUID]*fakeSandbox
}

type fakeSandbox struct {
	id      string
	command string
	// token is the workload's own credential, as its command would find
	// it in its environment.
	token string
	once  sync.Once
	done  chan struct{}
	code  int
}

func (*fakeRuntime) Name() string { return "fake" }

func (f *fakeRuntime) Start(_ context.Context, spec agentworkload.Spec) (agentworkload.Sandbox, error) {
	sandbox := &fakeSandbox{
		id:      "sandbox-" + spec.Name,
		command: spec.Command,
		token:   spec.SessionToken,
		done:    make(chan struct{}),
	}
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

func (*fakeSandbox) Attach(context.Context, net.Conn, uint16, uint16) error {
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
	return nil
}

// TestWorkloadsDisabled checks that nothing of workloads is reachable
// without the experiment.
func TestWorkloadsDisabled(t *testing.T) {
	t.Parallel()

	client, db := coderdtest.NewWithDatabase(t, nil)
	admin := coderdtest.CreateFirstUser(t, client)
	build := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: admin.OrganizationID,
		OwnerID:        admin.UserID,
	}).WithAgent().Do()

	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := client.LaunchWorkload(ctx, codersdk.LaunchWorkloadRequest{Command: "true", TemplateID: build.Workspace.TemplateID})
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())

	// An agent token is no user credential.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.URL.String()+"/api/v2/workspaces", nil)
	require.NoError(t, err)
	req.Header.Set(codersdk.SessionTokenHeader, build.AgentToken)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = res.Body.Close()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)

	// An ordinary agent token keeps working exactly as before.
	_, err = agentsdk.New(client.URL, agentsdk.WithFixedToken(build.AgentToken)).GitSSHKey(ctx)
	require.NoError(t, err)
}
