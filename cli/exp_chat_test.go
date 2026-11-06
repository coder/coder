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
		snapshotError string
		resyncErr     error
		wantOutput    string
		wantErr       string
	}{
		{
			name:       "Startup",
			wantOutput: "Context will be collected after startup scripts finish. Chats were not refreshed.\n",
		},
		{
			name:       "Ready",
			ready:      true,
			wantOutput: "Agent context resolved and stored by coderd (2 resources).\nRefreshed 2 drifted chat(s).\n",
		},
		{
			name:          "SnapshotError",
			ready:         true,
			snapshotError: "unreadable file",
			wantOutput:    "Agent context resolved and stored by coderd (2 resources).\nSnapshot reported an error: unreadable file\nRefreshed 2 drifted chat(s).\n",
		},
		{
			name:      "PushUnavailable",
			ready:     true,
			resyncErr: agentcontext.ErrPushUnavailable,
			wantErr:   "workspace context sync is disabled on this deployment",
		},
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

			cm := &refreshContextManager{err: tc.resyncErr}
			if tc.ready {
				cm.snapshot = agentcontext.Snapshot{
					Version:       2,
					SnapshotError: tc.snapshotError,
					Resources: []agentcontext.Resource{
						{Kind: agentcontext.KindInstructionFile},
						{Kind: agentcontext.KindMCPServer},
					},
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
			err = inv.WithContext(testutil.Context(t, testutil.WaitLong)).Run()
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantOutput, out.String())
			if tc.ready && tc.wantErr == "" {
				require.EqualValues(t, 1, requests.Load())
			} else {
				require.Zero(t, requests.Load(), "an unavailable snapshot must not refresh chats")
			}
		})
	}
}

type refreshContextManager struct {
	agentsocket.ContextManager
	snapshot agentcontext.Snapshot
	err      error
}

func (m *refreshContextManager) Snapshot() agentcontext.Snapshot { return m.snapshot }

func (m *refreshContextManager) Resync(context.Context) (agentcontext.Snapshot, error) {
	return m.snapshot, m.err
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
