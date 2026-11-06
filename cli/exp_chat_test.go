package cli_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/testutil"
)

func TestExpChatContextRefreshOutput(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		ready         bool
		changed       bool
		snapshotError string
		wantOutput    string
	}{
		{name: "Startup", wantOutput: "Context will be collected after startup scripts finish. Chats were not refreshed."},
		{name: "Changed", ready: true, changed: true, wantOutput: "Agent context changed"},
		{name: "Unchanged", ready: true, wantOutput: "Agent context unchanged"},
		{name: "ErrorChanged", ready: true, snapshotError: "unreadable file", wantOutput: "Agent context changed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/api/v2/workspaceagents/me/experimental/chat-context/refresh", r.URL.Path)
				_, _ = w.Write([]byte(`{"refreshed":2}`))
			}))
			defer api.Close()

			cm := &refreshContextManager{}
			if tc.ready {
				cm.before.Version = 1
				cm.after.Version = 2
				cm.after.SnapshotError = tc.snapshotError
				if tc.changed {
					cm.after.AggregateHash[0] = 1
				}
			}
			socketPath := testutil.AgentSocketPath(t)
			server, err := agentsocket.NewServer(testutil.Logger(t),
				agentsocket.WithPath(socketPath), agentsocket.WithContextManager(cm))
			require.NoError(t, err)
			defer server.Close()
			inv, _ := clitest.New(t, "exp", "chat", "context", "refresh",
				"--socket-path", socketPath, "--agent-url", api.URL, "--agent-token", "test-agent-token")
			var out bytes.Buffer
			inv.Stdout = &out
			require.NoError(t, inv.WithContext(testutil.Context(t, testutil.WaitLong)).Run())
			require.Contains(t, out.String(), tc.wantOutput)
			if tc.ready {
				require.EqualValues(t, 1, requests.Load())
				require.Contains(t, out.String(), "Refreshed 2 drifted chat(s).")
				if tc.snapshotError != "" {
					require.Contains(t, out.String(), "Snapshot reported an error: "+tc.snapshotError)
				}
			} else {
				require.Zero(t, requests.Load(), "startup refresh must not call coderd")
			}
		})
	}
}

type refreshContextManager struct {
	agentsocket.ContextManager
	before agentcontext.Snapshot
	after  agentcontext.Snapshot
}

func (m *refreshContextManager) Snapshot() agentcontext.Snapshot { return m.before }

func (m *refreshContextManager) Resync(context.Context) (agentcontext.Snapshot, error) {
	return m.after, nil
}

func TestExpChatContextAdd(t *testing.T) {
	t.Parallel()

	t.Run("RequiresPathArgument", func(t *testing.T) {
		t.Parallel()

		// `add` registers a context source identified by <path>, so the path
		// argument is required and a bare invocation is a usage error.
		inv, _ := clitest.New(t, "exp", "chat", "context", "add")

		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "wanted 1 args but got 0")
	})

	t.Run("RequiresWorkspaceSocket", func(t *testing.T) {
		t.Parallel()

		// Source registration talks to the agent over its local socket, so
		// outside a workspace it fails to connect rather than silently doing
		// nothing. Point at a socket path that does not exist so the dial
		// fails deterministically (and never touches a real agent socket).
		missingSocket := filepath.Join(t.TempDir(), "agent.sock")
		inv, _ := clitest.New(t, "exp", "chat", "context", "add", t.TempDir(),
			"--socket-path", missingSocket)

		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "inside the workspace")
	})
}
