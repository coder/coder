package cli_test

import (
	"bytes"
	"context"
	"database/sql"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agentcontainers"
	"github.com/coder/coder/v2/agent/agentcontainers/watcher"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/coder/v2/testutil/expecter"
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
	_ = coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).Wait()

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
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			inv, root := clitest.New(t, append([]string{"open", "vscode"}, tt.args...)...)
			clitest.SetupConfig(t, client, root)
			var stdout *expecter.Expecter
			stdout, inv.Stdout = expecter.NewPiped(t)

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

			line := stdout.ReadLine(ctx)
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
	_ = coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).Wait()

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
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			inv, root := clitest.New(t, append([]string{"open", "vscode"}, tt.args...)...)
			clitest.SetupConfig(t, client, root)
			var stdout *expecter.Expecter
			stdout, inv.Stdout = expecter.NewPiped(t)

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

			line := stdout.ReadLine(ctx)
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

type fakeContainerCLI struct {
	resp codersdk.WorkspaceAgentListContainersResponse
}

func (f *fakeContainerCLI) List(ctx context.Context) (codersdk.WorkspaceAgentListContainersResponse, error) {
	return f.resp, nil
}

func (*fakeContainerCLI) DetectArchitecture(ctx context.Context, containerID string) (string, error) {
	return runtime.GOARCH, nil
}

func (*fakeContainerCLI) Copy(ctx context.Context, containerID, src, dst string) error {
	return nil
}

func (*fakeContainerCLI) ExecAs(ctx context.Context, containerID, user string, args ...string) ([]byte, error) {
	return nil, nil
}

func (*fakeContainerCLI) Stop(ctx context.Context, containerID string) error {
	return nil
}

func (*fakeContainerCLI) Remove(ctx context.Context, containerID string) error {
	return nil
}

type fakeDevcontainerCLI struct {
	config    agentcontainers.DevcontainerConfig
	execAgent func(ctx context.Context, token string) error
}

func (f *fakeDevcontainerCLI) ReadConfig(ctx context.Context, workspaceFolder, configFile string, env []string, opts ...agentcontainers.DevcontainerCLIReadConfigOptions) (agentcontainers.DevcontainerConfig, error) {
	return f.config, nil
}

func (f *fakeDevcontainerCLI) Exec(ctx context.Context, workspaceFolder, configFile string, name string, args []string, opts ...agentcontainers.DevcontainerCLIExecOptions) error {
	var opt agentcontainers.DevcontainerCLIExecConfig
	for _, o := range opts {
		o(&opt)
	}
	var token string
	for _, arg := range opt.Args {
		if strings.HasPrefix(arg, "CODER_AGENT_TOKEN=") {
			token = strings.TrimPrefix(arg, "CODER_AGENT_TOKEN=")
			break
		}
	}
	if token == "" {
		return xerrors.New("no agent token provided in args")
	}
	if f.execAgent == nil {
		return nil
	}
	return f.execAgent(ctx, token)
}

func (*fakeDevcontainerCLI) Up(ctx context.Context, workspaceFolder, configFile string, opts ...agentcontainers.DevcontainerCLIUpOptions) (string, error) {
	return "", nil
}

func TestOpenVSCodeDevContainer(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "linux" {
		t.Skip("DevContainers are only supported for agents on Linux")
	}

	parentAgentName := "agent1"

	devcontainerID := uuid.New()
	devcontainerName := "wilson"
	workspaceFolder := "/home/coder/wilson"
	configFile := path.Join(workspaceFolder, ".devcontainer", "devcontainer.json")

	containerID := uuid.NewString()
	containerName := testutil.GetRandomName(t)
	containerFolder := "/workspaces/wilson"

	client, workspace, agentToken := setupWorkspaceForAgent(t, func(agents []*proto.Agent) []*proto.Agent {
		agents[0].Name = parentAgentName
		agents[0].OperatingSystem = runtime.GOOS
		return agents
	})

	fCCLI := &fakeContainerCLI{
		resp: codersdk.WorkspaceAgentListContainersResponse{
			Containers: []codersdk.WorkspaceAgentContainer{
				{
					ID:           containerID,
					CreatedAt:    dbtime.Now(),
					FriendlyName: containerName,
					Image:        "busybox:latest",
					Labels: map[string]string{
						agentcontainers.DevcontainerLocalFolderLabel: workspaceFolder,
						agentcontainers.DevcontainerConfigFileLabel:  configFile,
						agentcontainers.DevcontainerIsTestRunLabel:   "true",
						"coder.test": t.Name(),
					},
					Running: true,
					Status:  "running",
				},
			},
		},
	}
	fDCCLI := &fakeDevcontainerCLI{
		config: agentcontainers.DevcontainerConfig{
			Workspace: agentcontainers.DevcontainerWorkspace{
				WorkspaceFolder: containerFolder,
			},
		},
		execAgent: func(ctx context.Context, token string) error {
			t.Logf("Starting devcontainer subagent with token: %s", token)
			_ = agenttest.New(t, client.URL, token)
			<-ctx.Done()
			return ctx.Err()
		},
	}

	_ = agenttest.New(t, client.URL, agentToken, func(o *agent.Options) {
		o.Devcontainers = true
		o.DevcontainerAPIOptions = append(o.DevcontainerAPIOptions,
			agentcontainers.WithProjectDiscovery(false),
			agentcontainers.WithContainerCLI(fCCLI),
			agentcontainers.WithDevcontainerCLI(fDCCLI),
			agentcontainers.WithWatcher(watcher.NewNoop()),
			agentcontainers.WithDevcontainers(
				[]codersdk.WorkspaceAgentDevcontainer{{
					ID:              devcontainerID,
					Name:            devcontainerName,
					WorkspaceFolder: workspaceFolder,
					Status:          codersdk.WorkspaceAgentDevcontainerStatusStopped,
				}},
				[]codersdk.WorkspaceAgentScript{{
					ID:          devcontainerID,
					LogSourceID: uuid.New(),
				}},
			),
			agentcontainers.WithContainerLabelIncludeFilter("coder.test", t.Name()),
		)
	})
	resources := coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).AgentNames([]string{parentAgentName}).Wait()
	parentAgent := coderdtest.RequireWorkspaceAgentByName(t, resources, parentAgentName)
	parentAgentID := parentAgent.ID

	// Agent connection does not guarantee the parent agent's container API
	// has completed its first devcontainer update. Wait for that endpoint so
	// parallel open commands do not race the initial cache population.
	ctx := testutil.Context(t, testutil.WaitSuperLong)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		resp, err := client.WorkspaceAgentListContainers(ctx, parentAgentID, nil)
		if err != nil {
			t.Logf("list containers: %v", err)
			return false
		}
		var devcontainerAgentID uuid.UUID
		for _, dc := range resp.Devcontainers {
			if dc.ID != devcontainerID {
				continue
			}
			if dc.Status != codersdk.WorkspaceAgentDevcontainerStatusRunning {
				t.Logf("devcontainer %s status %q", devcontainerName, dc.Status)
				return false
			}
			if dc.Container == nil {
				t.Logf("devcontainer %s missing container", devcontainerName)
				return false
			}
			if dc.Container.ID != containerID {
				t.Logf("devcontainer %s has container %s, want %s", devcontainerName, dc.Container.ID, containerID)
				return false
			}
			if dc.Agent == nil {
				t.Logf("devcontainer %s missing subagent", devcontainerName)
				return false
			}
			if dc.Agent.Name != devcontainerName {
				t.Logf("devcontainer %s has subagent %s, want %s", devcontainerName, dc.Agent.Name, devcontainerName)
				return false
			}
			devcontainerAgentID = dc.Agent.ID
		}
		if devcontainerAgentID == uuid.Nil {
			t.Logf("devcontainer %s not found", devcontainerName)
			return false
		}

		workspace, err := client.Workspace(ctx, workspace.ID)
		if err != nil {
			t.Logf("get workspace: %v", err)
			return false
		}
		for _, resource := range workspace.LatestBuild.Resources {
			for _, workspaceAgent := range resource.Agents {
				if workspaceAgent.ID != devcontainerAgentID {
					continue
				}
				if workspaceAgent.Status != codersdk.WorkspaceAgentConnected {
					t.Logf("devcontainer subagent %s status %q", devcontainerAgentID, workspaceAgent.Status)
					return false
				}
				return true
			}
		}
		t.Logf("devcontainer subagent %s not found in workspace", devcontainerAgentID)
		return false
	}, testutil.IntervalMedium, "devcontainer did not become ready")

	insideWorkspaceEnv := map[string]string{
		"CODER":                      "true",
		"CODER_WORKSPACE_NAME":       workspace.Name,
		"CODER_WORKSPACE_AGENT_NAME": devcontainerName,
	}

	wd, err := os.Getwd()
	require.NoError(t, err)

	tests := []struct {
		name      string
		env       map[string]string
		args      []string
		wantDir   string
		wantError bool
		wantToken bool
	}{
		{
			name:      "nonexistent container",
			args:      []string{"--test.open-error", workspace.Name + "." + devcontainerName + "bad"},
			wantError: true,
		},
		{
			name:      "ok",
			args:      []string{"--test.open-error", workspace.Name + "." + devcontainerName},
			wantError: false,
		},
		{
			name:      "ok with absolute path",
			args:      []string{"--test.open-error", workspace.Name + "." + devcontainerName, containerFolder},
			wantError: false,
		},
		{
			name:      "ok with relative path",
			args:      []string{"--test.open-error", workspace.Name + "." + devcontainerName, "my/relative/path"},
			wantDir:   path.Join(containerFolder, "my/relative/path"),
			wantError: false,
		},
		{
			name:      "ok with token",
			args:      []string{"--test.open-error", workspace.Name + "." + devcontainerName, "--generate-token"},
			wantError: false,
			wantToken: true,
		},
		// Inside workspace, does not require --test.open-error
		{
			name: "ok inside workspace",
			env:  insideWorkspaceEnv,
			args: []string{workspace.Name + "." + devcontainerName},
		},
		{
			name:    "ok inside workspace relative path",
			env:     insideWorkspaceEnv,
			args:    []string{workspace.Name + "." + devcontainerName, "foo"},
			wantDir: filepath.Join(wd, "foo"),
		},
		{
			name:      "ok inside workspace token",
			env:       insideWorkspaceEnv,
			args:      []string{workspace.Name + "." + devcontainerName, "--generate-token"},
			wantToken: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			inv, root := clitest.New(t, append([]string{"open", "vscode"}, tt.args...)...)
			clitest.SetupConfig(t, client, root)
			var stdout *expecter.Expecter
			stdout, inv.Stdout = expecter.NewPiped(t)

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

			line := stdout.ReadLine(ctx)
			u, err := url.ParseRequestURI(line)
			require.NoError(t, err, "line: %q", line)

			qp := u.Query()
			assert.Equal(t, client.URL.String(), qp.Get("url"))
			assert.Equal(t, me.Username, qp.Get("owner"))
			assert.Equal(t, workspace.Name, qp.Get("workspace"))
			assert.Equal(t, parentAgentName, qp.Get("agent"))
			assert.Equal(t, containerName, qp.Get("devContainerName"))
			assert.Equal(t, workspaceFolder, qp.Get("localWorkspaceFolder"))
			assert.Equal(t, configFile, qp.Get("localConfigFile"))

			if tt.wantDir != "" {
				assert.Equal(t, tt.wantDir, qp.Get("devContainerFolder"))
			} else {
				assert.Equal(t, containerFolder, qp.Get("devContainerFolder"))
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

func TestOpenApp(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client, ws, _ := setupWorkspaceForAgent(t, func(agents []*proto.Agent) []*proto.Agent {
			agents[0].Apps = []*proto.App{
				{
					Slug: "app1",
					Url:  "https://example.com/app1",
				},
			}
			return agents
		})

		inv, root := clitest.New(t, "open", "app", ws.Name, "app1", "--test.open-error")
		clitest.SetupConfig(t, client, root)

		w := clitest.StartWithWaiter(t, inv)
		w.RequireError()
		w.RequireContains("test.open-error")
	})

	t.Run("OnlyWorkspaceName", func(t *testing.T) {
		t.Parallel()

		client, ws, _ := setupWorkspaceForAgent(t)
		inv, root := clitest.New(t, "open", "app", ws.Name)
		clitest.SetupConfig(t, client, root)
		var sb strings.Builder
		inv.Stdout = &sb
		inv.Stderr = &sb

		w := clitest.StartWithWaiter(t, inv)
		w.RequireSuccess()

		require.Contains(t, sb.String(), "Available apps in")
	})

	t.Run("WorkspaceNotFound", func(t *testing.T) {
		t.Parallel()

		client, _, _ := setupWorkspaceForAgent(t)
		inv, root := clitest.New(t, "open", "app", "not-a-workspace", "app1")
		clitest.SetupConfig(t, client, root)
		w := clitest.StartWithWaiter(t, inv)
		w.RequireError()
		w.RequireContains("Resource not found or you do not have access to this resource")
	})

	t.Run("AppNotFound", func(t *testing.T) {
		t.Parallel()

		client, ws, _ := setupWorkspaceForAgent(t)

		inv, root := clitest.New(t, "open", "app", ws.Name, "app1")
		clitest.SetupConfig(t, client, root)

		w := clitest.StartWithWaiter(t, inv)
		w.RequireError()
		w.RequireContains("app not found")
	})

	t.Run("RegionNotFound", func(t *testing.T) {
		t.Parallel()

		client, ws, _ := setupWorkspaceForAgent(t, func(agents []*proto.Agent) []*proto.Agent {
			agents[0].Apps = []*proto.App{
				{
					Slug: "app1",
					Url:  "https://example.com/app1",
				},
			}
			return agents
		})

		inv, root := clitest.New(t, "open", "app", ws.Name, "app1", "--region", "bad-region")
		clitest.SetupConfig(t, client, root)

		w := clitest.StartWithWaiter(t, inv)
		w.RequireError()
		w.RequireContains("region not found")
	})

	t.Run("ExternalAppOnTopLevelAgentSubstitutes", func(t *testing.T) {
		t.Parallel()

		// Apps on the top-level (template-defined) agent are trusted, so the
		// CLI substitutes $SESSION_TOKEN regardless of scheme.
		client, ws, _ := setupWorkspaceForAgent(t, func(agents []*proto.Agent) []*proto.Agent {
			agents[0].Apps = []*proto.App{
				{
					Slug:     "app1",
					Url:      "vscode://coder.coder-remote/open?token=$SESSION_TOKEN",
					External: true,
				},
			}
			return agents
		})
		inv, root := clitest.New(t, "open", "app", ws.Name, "app1", "--test.open-error")
		clitest.SetupConfig(t, client, root)

		w := clitest.StartWithWaiter(t, inv)
		w.RequireError()
		w.RequireContains("test.open-error")
		w.RequireContains(client.SessionToken())
	})

	t.Run("ExternalAppOnSubAgentWithPlaceholderPrintsURLAndDoesNotOpen", func(t *testing.T) {
		t.Parallel()

		// Sub-agent app URLs are attacker-influenceable through workspace
		// configuration and runtime registration. The CLI must not
		// substitute the session token, and must not hand the URL to the
		// OS open handler. The URL is printed to stdout so a user who
		// trusts the source can substitute and open it manually.
		ownerClient, store := coderdtest.NewWithDatabase(t, nil)
		ownerClient.SetLogger(testutil.Logger(t).Named("client"))
		first := coderdtest.CreateFirstUser(t, ownerClient)
		userClient, user := coderdtest.CreateAnotherUserMutators(t, ownerClient, first.OrganizationID, nil, func(r *codersdk.CreateUserRequestWithOrgs) {
			r.Username = "subagentowner"
		})
		r := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
			Name:           "subagentws",
			OrganizationID: first.OrganizationID,
			OwnerID:        user.ID,
		}).WithAgent().Do()

		require.NotEmpty(t, r.Agents, "expected at least one workspace agent")
		mainAgent := r.Agents[0]

		subAgent := dbgen.WorkspaceSubAgent(t, store, mainAgent, database.WorkspaceAgent{
			Name: "devcontainer",
		})
		_ = dbgen.WorkspaceApp(t, store, database.WorkspaceApp{
			AgentID:  subAgent.ID,
			Slug:     "subapp",
			External: true,
			Url:      sql.NullString{Valid: true, String: "vscode://coder.coder-remote/open?token=$SESSION_TOKEN"},
		})

		inv, root := clitest.New(t, "open", "app", r.Workspace.Name+".devcontainer", "subapp", "--test.open-error")
		clitest.SetupConfig(t, userClient, root)
		var stdout, stderr bytes.Buffer
		inv.Stdout = &stdout
		inv.Stderr = &stderr

		w := clitest.StartWithWaiter(t, inv)
		w.RequireSuccess()
		require.NotContains(t, stderr.String(), "test.open-error")
		require.NotContains(t, stdout.String(), "test.open-error")
		require.Contains(t, stdout.String(), "vscode://coder.coder-remote/open?token=$SESSION_TOKEN")
		require.NotContains(t, stdout.String(), userClient.SessionToken())
		require.Contains(t, stderr.String(), "substitute")
	})

	t.Run("ExternalAppOnSubAgentWithoutPlaceholderOpensAsIs", func(t *testing.T) {
		t.Parallel()

		// Sub-agent app URLs that don't reference $SESSION_TOKEN carry no
		// token to leak. The CLI auto-opens them like any other external
		// app; only placeholder-bearing URLs are gated.
		ownerClient, store := coderdtest.NewWithDatabase(t, nil)
		ownerClient.SetLogger(testutil.Logger(t).Named("client"))
		first := coderdtest.CreateFirstUser(t, ownerClient)
		userClient, user := coderdtest.CreateAnotherUserMutators(t, ownerClient, first.OrganizationID, nil, func(r *codersdk.CreateUserRequestWithOrgs) {
			r.Username = "subagentowner2"
		})
		r := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
			Name:           "subagentws2",
			OrganizationID: first.OrganizationID,
			OwnerID:        user.ID,
		}).WithAgent().Do()

		require.NotEmpty(t, r.Agents, "expected at least one workspace agent")
		mainAgent := r.Agents[0]

		subAgent := dbgen.WorkspaceSubAgent(t, store, mainAgent, database.WorkspaceAgent{
			Name: "devcontainer",
		})
		_ = dbgen.WorkspaceApp(t, store, database.WorkspaceApp{
			AgentID:  subAgent.ID,
			Slug:     "subapp",
			External: true,
			Url:      sql.NullString{Valid: true, String: "https://example.com/some/path"},
		})

		inv, root := clitest.New(t, "open", "app", r.Workspace.Name+".devcontainer", "subapp", "--test.open-error")
		clitest.SetupConfig(t, userClient, root)

		w := clitest.StartWithWaiter(t, inv)
		w.RequireError()
		w.RequireContains("test.open-error")
		w.RequireContains("https://example.com/some/path")
	})
}
