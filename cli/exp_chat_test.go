package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agentcontextconfig"
	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/agent/agenttest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
)

type chatContextEnvInfo struct {
	usershell.SystemEnvInfo
	home string
}

func (e chatContextEnvInfo) HomeDir() (string, error) { return e.home, nil }

type chatContextPush struct {
	api     *chatContextAgentAPI
	request *agentproto.PushContextStateRequest
}

type chatContextAgentClient struct {
	*agentsdk.Client
	connections chan *chatContextAgentAPI
	pushes      chan chatContextPush
}

func (c *chatContextAgentClient) ConnectRPC216WithRole(ctx context.Context, role string) (agentproto.DRPCAgentClient216, tailnetproto.DRPCTailnetClient28, error) {
	api, tailnet, err := c.Client.ConnectRPC216WithRole(ctx, role)
	if err != nil {
		return nil, nil, err
	}
	observed := &chatContextAgentAPI{DRPCAgentClient216: api, pushes: c.pushes, manifest: make(chan struct{})}
	c.connections <- observed
	return observed, tailnet, nil
}

type chatContextAgentAPI struct {
	agentproto.DRPCAgentClient216
	pushes   chan chatContextPush
	manifest chan struct{}
}

func (a *chatContextAgentAPI) GetManifest(ctx context.Context, req *agentproto.GetManifestRequest) (*agentproto.Manifest, error) {
	manifest, err := a.DRPCAgentClient216.GetManifest(ctx, req)
	if err == nil {
		close(a.manifest)
	}
	return manifest, err
}

func (a *chatContextAgentAPI) PushContextState(ctx context.Context, req *agentproto.PushContextStateRequest) (*agentproto.PushContextStateResponse, error) {
	response, err := a.DRPCAgentClient216.PushContextState(ctx, req)
	if err == nil {
		select {
		case a.pushes <- chatContextPush{api: a, request: req}:
		case <-ctx.Done():
		}
	}
	return response, err
}

// TestExpChatContextStartupHelper runs the CLI from the agent's startup script.
func TestExpChatContextStartupHelper(t *testing.T) {
	t.Parallel()
	socket := os.Getenv("CODER_TEST_CONTEXT_STARTUP_SOCKET")
	if socket == "" {
		t.Skip("startup-script subprocess helper")
	}
	inv, _ := clitest.New(t, "exp", "chat", "context", "remove", os.Getenv("CODER_TEST_CONTEXT_STARTUP_DIR"), "--socket-path", socket)
	inv.Environ.Set("CODER_URL", "")
	inv.Environ.Set("CODER_SESSION_TOKEN", "")
	require.NoError(t, inv.WithContext(testutil.Context(t, testutil.WaitLong)).Run())
}

func TestExpChatContextSources(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	wd := filepath.Join(root, "project")
	outside := filepath.Join(root, "outside")
	agentsSkill := filepath.Join(wd, ".agents", "skills", "project-agent")
	claudeSkill := filepath.Join(wd, ".claude", "skills", "project-claude")
	for _, dir := range []string{wd, outside, agentsSkill, claudeSkill} {
		require.NoError(t, os.MkdirAll(dir, 0o700))
	}
	projectInstructions := filepath.Join(wd, "AGENTS.md")
	require.NoError(t, os.WriteFile(projectInstructions, []byte("project instructions"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "AGENTS.md"), []byte("outside instructions"), 0o600))
	for _, skill := range []string{agentsSkill, claudeSkill} {
		content := "---\nname: " + filepath.Base(skill) + "\ndescription: project skill\n---\nSkill instructions.\n"
		require.NoError(t, os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte(content), 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(wd, ".mcp.json"), []byte(`{"mcpServers":{}}`), 0o600))
	binary, err := os.Executable()
	require.NoError(t, err)
	script := "'" + strings.ReplaceAll(binary, "'", "'\"'\"'") + "' '-test.run=^TestExpChatContextStartupHelper$'"
	if runtime.GOOS == "windows" {
		script = "& '" + strings.ReplaceAll(binary, "'", "''") + "' '-test.run=^TestExpChatContextStartupHelper$'"
	}
	socketPath := testutil.AgentSocketPath(t)
	client, workspace, agentToken := setupWorkspaceForAgent(t, func(agents []*proto.Agent) []*proto.Agent {
		agents[0].Directory = wd
		agents[0].Env["CODER_TEST_CONTEXT_STARTUP_SOCKET"] = socketPath
		agents[0].Env["CODER_TEST_CONTEXT_STARTUP_DIR"] = wd
		agents[0].Scripts = []*proto.Script{{Script: script, RunOnStart: true, TimeoutSeconds: 30, DisplayName: "exclude project context"}}
		return agents
	})
	observed := &chatContextAgentClient{
		Client:      agentsdk.New(client.URL, agentsdk.WithFixedToken(agentToken)),
		connections: make(chan *chatContextAgentAPI, 8),
		pushes:      make(chan chatContextPush, 128),
	}
	agenttest.New(t, client.URL, agentToken, func(o *agent.Options) {
		o.Client = observed
		// Isolate ACP harness configuration from the host's home directory.
		o.EnvInfo = chatContextEnvInfo{home: t.TempDir()}
		o.SocketPath = socketPath
		o.SocketServerEnabled = true
		o.ContextConfig = agentcontextconfig.Config{
			InstructionsDirs: ".," + outside,
			SkillsDirs:       agentcontextconfig.DefaultSkillsDir,
			MCPConfigFiles:   agentcontextconfig.DefaultMCPConfigFile,
		}
	})
	ctx := testutil.Context(t, testutil.WaitLong)
	firstAPI := testutil.RequireReceive(ctx, t, observed.connections)
	coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).WaitFor(coderdtest.AgentsReady)
	firstPush := testutil.RequireReceive(ctx, t, observed.pushes)
	require.True(t, firstPush.request.Initial)
	require.True(t, slices.ContainsFunc(firstPush.request.Resources, func(resource *agentproto.ContextResource) bool {
		return resource.Source == filepath.Join(outside, "AGENTS.md")
	}), "the first push must contain the completed startup inventory")
	for _, resource := range firstPush.request.Resources {
		require.False(t, strings.HasPrefix(resource.Source, wd+string(os.PathSeparator)), "startup exclusion must be in the first push")
	}
	sock, err := agentsocket.NewClient(ctx, agentsocket.WithPath(socketPath))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sock.Close() })
	run := func(args ...string) string {
		t.Helper()
		inv, _ := clitest.New(t, append([]string{"exp", "chat", "context", "--socket-path", socketPath}, args...)...)
		var stdout bytes.Buffer
		inv.Stdout = &stdout
		require.NoError(t, inv.WithContext(testutil.Context(t, testutil.WaitLong)).Run())
		return stdout.String()
	}
	projectPaths := []string{projectInstructions, agentsSkill, claudeSkill, filepath.Join(wd, ".mcp.json")}
	waitForResources := func(paths []string, included bool) agentsocket.ContextSnapshot {
		t.Helper()
		var snapshot agentsocket.ContextSnapshot
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			var err error
			snapshot, err = sock.GetContextSnapshot(ctx)
			if !assert.NoError(c, err) {
				return
			}
			for _, path := range paths {
				found := slices.ContainsFunc(snapshot.Resources, func(resource agentsocket.ContextResource) bool { return resource.Source == path })
				assert.Equal(c, included, found, "context resource %s", path)
			}
		}, testutil.WaitLong, testutil.IntervalFast)
		return snapshot
	}
	waitForResources(projectPaths, false)
	var listed []agentsocket.ContextSource
	require.NoError(t, json.Unmarshal([]byte(run("list", "--output", "json")), &listed))
	require.Contains(t, listed, agentsocket.ContextSource{Path: wd, Excluded: true})
	require.Contains(t, listed, agentsocket.ContextSource{Path: filepath.Join(wd, ".agents", "skills"), Excluded: true})
	require.Contains(t, run("list"), "EXCLUDED")

	run("add", wd)
	waitForResources(projectPaths, true)
	// User-added sources retain their ordinary add/remove lifecycle.
	removable := filepath.Join(wd, "user-added")
	require.NoError(t, os.MkdirAll(removable, 0o700))
	userInstructions := filepath.Join(removable, "AGENTS.md")
	require.NoError(t, os.WriteFile(userInstructions, []byte("user instructions"), 0o600))
	run("add", removable)
	waitForResources([]string{userInstructions}, true)
	run("remove", removable)
	waitForResources([]string{userInstructions}, false)
	require.NoError(t, json.Unmarshal([]byte(run("list", "--output", "json")), &listed))
	require.False(t, slices.ContainsFunc(listed, func(source agentsocket.ContextSource) bool { return source.Path == removable }))

	run("remove", wd)
	beforeReconnect := waitForResources(projectPaths, false)
	require.NoError(t, firstAPI.DRPCConn().Close())
	reconnected := testutil.RequireReceive(ctx, t, observed.connections)
	select {
	case <-reconnected.manifest:
	case <-ctx.Done():
		t.Fatal("agent did not fetch its manifest after reconnect")
	}
	for {
		push := testutil.RequireReceive(ctx, t, observed.pushes)
		if push.api == reconnected && push.request.Version > beforeReconnect.Version {
			break
		}
	}
	waitForResources(projectPaths, false)
	run("add", wd)
	waitForResources(projectPaths, true)
}

func TestExpChatContextAdd(t *testing.T) {
	t.Parallel()
	t.Run("RequiresPathArgument", func(t *testing.T) {
		t.Parallel()
		inv, _ := clitest.New(t, "exp", "chat", "context", "add")
		err := inv.Run()
		require.ErrorContains(t, err, "wanted 1 args but got 0")
	})
	t.Run("RequiresWorkspaceSocket", func(t *testing.T) {
		t.Parallel()
		missingSocket := filepath.Join(t.TempDir(), "agent.sock")
		inv, _ := clitest.New(t, "exp", "chat", "context", "add", t.TempDir(), "--socket-path", missingSocket)
		err := inv.Run()
		require.ErrorContains(t, err, "inside the workspace")
	})
}
