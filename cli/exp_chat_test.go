package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agentcontextconfig"
	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

type chatContextEnvInfo struct {
	usershell.SystemEnvInfo
	home string
}

func (e chatContextEnvInfo) HomeDir() (string, error) { return e.home, nil }

func TestExpChatContextRemove(t *testing.T) {
	t.Parallel()

	wd := testutil.TempDirResolved(t)
	instructions := filepath.Join(wd, "instructions")
	skills := filepath.Join(wd, ".agents", "skills")
	mcpConfig := filepath.Join(wd, ".mcp.json")
	for _, dir := range []string{wd, instructions, skills} {
		require.NoError(t, os.MkdirAll(dir, 0o700))
	}
	require.NoError(t, os.WriteFile(filepath.Join(instructions, "AGENTS.md"), []byte("seeded instructions"), 0o600))
	require.NoError(t, os.WriteFile(mcpConfig, []byte(`{"mcpServers":{}}`), 0o600))

	client, workspace, agentToken := setupWorkspaceForAgent(t, func(agents []*proto.Agent) []*proto.Agent {
		agents[0].Directory = wd
		return agents
	})
	socketPath := testutil.AgentSocketPath(t)
	agenttest.New(t, client.URL, agentToken, func(o *agent.Options) {
		// Isolate ACP harness configuration from the host's home directory.
		o.EnvInfo = chatContextEnvInfo{home: t.TempDir()}
		o.SocketPath = socketPath
		o.SocketServerEnabled = true
		o.ContextConfig = agentcontextconfig.Config{
			InstructionsDirs: "instructions," + agentcontextconfig.DefaultInstructionsDir,
			SkillsDirs:       agentcontextconfig.DefaultSkillsDir,
			MCPConfigFiles:   agentcontextconfig.DefaultMCPConfigFile,
		}
	})
	coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).WaitFor(coderdtest.AgentsReady)
	ctx := testutil.Context(t, testutil.WaitLong)
	sock, err := agentsocket.NewClient(ctx, agentsocket.WithPath(socketPath))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sock.Close() })

	run := func(t *testing.T, args ...string) (string, error) {
		t.Helper()
		args = append([]string{"exp", "chat", "context", "--socket-path", socketPath}, args...)
		inv, _ := clitest.New(t, args...)
		var stdout bytes.Buffer
		inv.Stdout = &stdout
		err := inv.WithContext(testutil.Context(t, testutil.WaitLong)).Run()
		return stdout.String(), err
	}
	for _, tc := range []struct {
		name   string
		args   []string
		reason string
	}{
		{name: "Instructions", args: []string{"remove", instructions}, reason: "CODER_AGENT_EXP_* settings or their defaults"},
		{name: "DefaultSkills", args: []string{"remove", skills}, reason: "CODER_AGENT_EXP_* settings or their defaults"},
		{name: "DefaultMCPConfig", args: []string{"remove", mcpConfig}, reason: "CODER_AGENT_EXP_* settings or their defaults"},
		{name: "SeededBuiltin", args: []string{"remove", "~/.coder"}, reason: "agent configuration"},
		{name: "AddBuiltin", args: []string{"add", "~/.coder"}, reason: "built-in scan root"},
		{name: "AddWorkingDirectory", args: []string{"add", wd}, reason: "working directory"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			before, err := run(t, "list", "--output", "json")
			require.NoError(t, err)
			stdout, err := run(t, tc.args...)
			require.ErrorContains(t, err, tc.reason)
			require.Empty(t, stdout, "rejected source change must not report success")
			after, err := run(t, "list", "--output", "json")
			require.NoError(t, err)
			require.JSONEq(t, before, after)
		})
	}

	removable := filepath.Join(wd, "user-added")
	require.NoError(t, os.MkdirAll(removable, 0o700))
	removableFile := filepath.Join(removable, "AGENTS.md")
	require.NoError(t, os.WriteFile(removableFile, []byte("user instructions"), 0o600))
	_, err = run(t, "add", removable)
	require.NoError(t, err)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		snap, err := sock.GetContextSnapshot(ctx)
		if !assert.NoError(c, err) {
			return
		}
		assert.True(c, slices.ContainsFunc(snap.Resources, func(res agentsocket.ContextResource) bool {
			return res.Source == removableFile
		}), "added source must contribute its instruction file")
	}, testutil.WaitLong, testutil.IntervalFast)

	stdout, err := run(t, "remove", removable)
	require.NoError(t, err)
	require.Contains(t, stdout, "Removed context source "+removable)
	listed, err := run(t, "list", "--output", "json")
	require.NoError(t, err)
	var sources []agentsocket.ContextSource
	require.NoError(t, json.Unmarshal([]byte(listed), &sources))
	require.NotContains(t, sources, agentsocket.ContextSource{Path: removable})
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		snap, err := sock.GetContextSnapshot(ctx)
		if !assert.NoError(c, err) {
			return
		}
		assert.False(c, slices.ContainsFunc(snap.Resources, func(res agentsocket.ContextResource) bool {
			return res.Source == removableFile
		}), "removed source must stop contributing its instruction file")
	}, testutil.WaitLong, testutil.IntervalFast)
	_, err = run(t, "remove", removable)
	require.ErrorContains(t, err, "not found")
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
