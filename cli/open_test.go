package cli_test

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestOpenVSCode(t *testing.T) {
	t.Parallel()

	agentName := "agent1"
	agentDir, err := filepath.Abs(filepath.FromSlash("/tmp"))
	require.NoError(t, err)
	client, workspace, agentToken := setupWorkspaceForAgent(t, func(agents []*proto.Agent) []*proto.Agent {
		agents[0].Directory = agentDir
		agents[0].Name = agentName
		agents[0].OperatingSystem = runtime.GOOS
		return agents
	})

	_ = agenttest.New(t, client.URL, agentToken)
	_ = coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

	insideWorkspaceEnv := map[string]string{
		"CODER":                      "true",
		"CODER_WORKSPACE_NAME":       workspace.Name,
		"CODER_WORKSPACE_AGENT_NAME": agentName,
	}

	wd, err := os.Getwd()
	require.NoError(t, err)

	tests := []struct {
		name      string
		args      []string
		env       map[string]string
		wantDir   string
		wantToken bool
		wantError bool
	}{
		{
			name:      "no args",
			wantError: true,
		},
		{
			name:      "nonexistent workspace",
			args:      []string{"--test.open-error", workspace.Name + "bad"},
			wantError: true,
		},
		{
			name:    "ok",
			args:    []string{"--test.open-error", workspace.Name},
			wantDir: agentDir,
		},
		{
			name:      "ok relative path",
			args:      []string{"--test.open-error", workspace.Name, "my/relative/path"},
			wantDir:   filepath.Join(agentDir, filepath.FromSlash("my/relative/path")),
			wantError: false,
		},
		{
			name:    "ok with absolute path",
			args:    []string{"--test.open-error", workspace.Name, agentDir},
			wantDir: agentDir,
		},
		{
			name:      "ok with token",
			args:      []string{"--test.open-error", workspace.Name, "--generate-token"},
			wantDir:   agentDir,
			wantToken: true,
		},
		// Inside workspace, does not require --test.open-error.
		{
			name:    "ok inside workspace",
			env:     insideWorkspaceEnv,
			args:    []string{workspace.Name},
			wantDir: agentDir,
		},
		{
			name:    "ok inside workspace relative path",
			env:     insideWorkspaceEnv,
			args:    []string{workspace.Name, "foo"},
			wantDir: filepath.Join(wd, "foo"),
		},
		{
			name:      "ok inside workspace token",
			env:       insideWorkspaceEnv,
			args:      []string{workspace.Name, "--generate-token"},
			wantDir:   agentDir,
			wantToken: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			inv, root := clitest.New(t, append([]string{"open", "vscode"}, tt.args...)...)
			clitest.SetupConfig(t, client, root)
			pty := ptytest.New(t)
			inv.Stdin = pty.Input()
			inv.Stdout = pty.Output()

			ctx := testutil.Context(t, testutil.WaitLong)
			inv = inv.WithContext(ctx)
			for k, v := range tt.env {
				inv.Environ.Set(k, v)
			}

			w := clitest.StartWithWaiter(t, inv)

			if tt.wantError {
				w.RequireError()
				return
			}

			me, err := client.User(ctx, codersdk.Me)
			require.NoError(t, err)

			line := pty.ReadLine(ctx)
			u, err := url.ParseRequestURI(line)
			require.NoError(t, err, "line: %q", line)

			qp := u.Query()
			assert.Equal(t, client.URL.String(), qp.Get("url"))
			assert.Equal(t, me.Username, qp.Get("owner"))
			assert.Equal(t, workspace.Name, qp.Get("workspace"))
			assert.Equal(t, agentName, qp.Get("agent"))
			if tt.wantDir != "" {
				assert.Contains(t, qp.Get("folder"), tt.wantDir)
			} else {
				assert.Empty(t, qp.Get("folder"))
			}
			if tt.wantToken {
				assert.NotEmpty(t, qp.Get("token"))
			} else {
				assert.Empty(t, qp.Get("token"))
			}

			w.RequireSuccess()
		})
	}
}

func TestOpenVSCode_NoAgentDirectory(t *testing.T) {
	t.Parallel()

	agentName := "agent1"
	client, workspace, agentToken := setupWorkspaceForAgent(t, func(agents []*proto.Agent) []*proto.Agent {
		agents[0].Name = agentName
		agents[0].OperatingSystem = runtime.GOOS
		return agents
	})

	_ = agenttest.New(t, client.URL, agentToken)
	_ = coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

	insideWorkspaceEnv := map[string]string{
		"CODER":                      "true",
		"CODER_WORKSPACE_NAME":       workspace.Name,
		"CODER_WORKSPACE_AGENT_NAME": agentName,
	}

	wd, err := os.Getwd()
	require.NoError(t, err)

	absPath := "/home/coder"
	if runtime.GOOS == "windows" {
		absPath = "C:\\home\\coder"
	}

	tests := []struct {
		name      string
		args      []string
		env       map[string]string
		wantDir   string
		wantToken bool
		wantError bool
	}{
		{
			name: "ok",
			args: []string{"--test.open-error", workspace.Name},
		},
		{
			name:      "no agent dir error relative path",
			args:      []string{"--test.open-error", workspace.Name, "my/relative/path"},
			wantDir:   filepath.FromSlash("my/relative/path"),
			wantError: true,
		},
		{
			name:    "ok with absolute path",
			args:    []string{"--test.open-error", workspace.Name, absPath},
			wantDir: absPath,
		},
		{
			name:      "ok with token",
			args:      []string{"--test.open-error", workspace.Name, "--generate-token"},
			wantToken: true,
		},
		// Inside workspace, does not require --test.open-error.
		{
			name: "ok inside workspace",
			env:  insideWorkspaceEnv,
			args: []string{workspace.Name},
		},
		{
			name:    "ok inside workspace relative path",
			env:     insideWorkspaceEnv,
			args:    []string{workspace.Name, "foo"},
			wantDir: filepath.Join(wd, "foo"),
		},
		{
			name:      "ok inside workspace token",
			env:       insideWorkspaceEnv,
			args:      []string{workspace.Name, "--generate-token"},
			wantToken: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			inv, root := clitest.New(t, append([]string{"open", "vscode"}, tt.args...)...)
			clitest.SetupConfig(t, client, root)
			pty := ptytest.New(t)
			inv.Stdin = pty.Input()
			inv.Stdout = pty.Output()

			ctx := testutil.Context(t, testutil.WaitLong)
			inv = inv.WithContext(ctx)
			for k, v := range tt.env {
				inv.Environ.Set(k, v)
			}

			w := clitest.StartWithWaiter(t, inv)

			if tt.wantError {
				w.RequireError()
				return
			}

			me, err := client.User(ctx, codersdk.Me)
			require.NoError(t, err)

			line := pty.ReadLine(ctx)
			u, err := url.ParseRequestURI(line)
			require.NoError(t, err, "line: %q", line)

			qp := u.Query()
			assert.Equal(t, client.URL.String(), qp.Get("url"))
			assert.Equal(t, me.Username, qp.Get("owner"))
			assert.Equal(t, workspace.Name, qp.Get("workspace"))
			assert.Equal(t, agentName, qp.Get("agent"))
			if tt.wantDir != "" {
				assert.Contains(t, qp.Get("folder"), tt.wantDir)
			} else {
				assert.Empty(t, qp.Get("folder"))
			}
			if tt.wantToken {
				assert.NotEmpty(t, qp.Get("token"))
			} else {
				assert.Empty(t, qp.Get("token"))
			}

			w.RequireSuccess()
		})
	}
}
