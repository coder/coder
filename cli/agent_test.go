package cli_test

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/confine"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspaceAgent(t *testing.T) {
	t.Parallel()

	t.Run("LogDirectory", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).
			WithAgent().
			Do()
		logDir := t.TempDir()
		inv, _ := clitest.New(t,
			"agent",
			"--auth", "token",
			"--agent-token", r.AgentToken,
			"--agent-url", client.URL.String(),
			"--log-dir", logDir,
			"--socket-path", testutil.AgentSocketPath(t),
		)

		clitest.Start(t, inv)

		coderdtest.AwaitWorkspaceAgents(t, client, r.Workspace.ID)

		require.Eventually(t, func() bool {
			info, err := os.Stat(filepath.Join(logDir, "coder-agent.log"))
			if err != nil {
				return false
			}
			return info.Size() > 0
		}, testutil.WaitLong, testutil.IntervalMedium)
	})

	t.Run("PostStartup", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()

		logDir := t.TempDir()
		inv, _ := clitest.New(t,
			"agent",
			"--auth", "token",
			"--agent-token", r.AgentToken,
			"--agent-url", client.URL.String(),
			"--log-dir", logDir,
			"--socket-path", testutil.AgentSocketPath(t),
		)
		// Set the subsystems for the agent.
		inv.Environ.Set(agent.EnvAgentSubsystem, fmt.Sprintf("%s,%s", codersdk.AgentSubsystemExectrace, codersdk.AgentSubsystemEnvbox))

		clitest.Start(t, inv)

		resources := coderdtest.NewWorkspaceAgentWaiter(t, client, r.Workspace.ID).
			MatchResources(matchAgentWithSubsystems).Wait()
		require.Len(t, resources, 1)
		require.Len(t, resources[0].Agents, 1)
		require.Len(t, resources[0].Agents[0].Subsystems, 2)
		// Sorted
		require.Equal(t, codersdk.AgentSubsystemEnvbox, resources[0].Agents[0].Subsystems[0])
		require.Equal(t, codersdk.AgentSubsystemExectrace, resources[0].Agents[0].Subsystems[1])
	})
	t.Run("Headers&DERPHeaders", func(t *testing.T) {
		t.Parallel()

		// Create a coderd API instance the hard way since we need to change the
		// handler to inject our custom /derp handler.
		dv := coderdtest.DeploymentValues(t)
		dv.DERP.Config.BlockDirect = true
		setHandler, cancelFunc, serverURL, newOptions := coderdtest.NewOptions(t, &coderdtest.Options{
			DeploymentValues: dv,
		})

		// We set the handler after server creation for the access URL.
		coderAPI := coderd.New(newOptions)
		setHandler(coderAPI.RootHandler)
		provisionerCloser := coderdtest.NewProvisionerDaemon(t, coderAPI)
		t.Cleanup(func() {
			_ = provisionerCloser.Close()
		})
		client := codersdk.New(serverURL, codersdk.WithHTTPClient(coderdtest.NewIsolatedHTTPClient(serverURL)))
		t.Cleanup(func() {
			cancelFunc()
			_ = provisionerCloser.Close()
			_ = coderAPI.Close()
			client.HTTPClient.CloseIdleConnections()
		})

		var (
			admin              = coderdtest.CreateFirstUser(t, client)
			member, memberUser = coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
			called             atomic.Int64
			derpCalled         atomic.Int64
		)

		setHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Ignore client requests
			if r.Header.Get("X-Testing") == "agent" {
				assert.Equal(t, "Ethan was Here!", r.Header.Get("Cool-Header"))
				assert.Equal(t, "very-wow-"+client.URL.String(), r.Header.Get("X-Process-Testing"))
				assert.Equal(t, "more-wow", r.Header.Get("X-Process-Testing2"))
				if strings.HasPrefix(r.URL.Path, "/derp") {
					derpCalled.Add(1)
				} else {
					called.Add(1)
				}
			}
			coderAPI.RootHandler.ServeHTTP(w, r)
		}))
		r := dbfake.WorkspaceBuild(t, coderAPI.Database, database.WorkspaceTable{
			OrganizationID: memberUser.OrganizationIDs[0],
			OwnerID:        memberUser.ID,
		}).WithAgent().Do()

		coderURLEnv := "$CODER_URL"
		if runtime.GOOS == "windows" {
			coderURLEnv = "%CODER_URL%"
		}

		logDir := t.TempDir()
		agentInv, _ := clitest.New(t,
			"agent",
			"--auth", "token",
			"--agent-token", r.AgentToken,
			"--agent-url", client.URL.String(),
			"--log-dir", logDir,
			"--agent-header", "X-Testing=agent",
			"--agent-header", "Cool-Header=Ethan was Here!",
			"--agent-header-command", "printf X-Process-Testing=very-wow-"+coderURLEnv+"'\\r\\n'X-Process-Testing2=more-wow",
			"--socket-path", testutil.AgentSocketPath(t),
		)
		clitest.Start(t, agentInv)
		coderdtest.NewWorkspaceAgentWaiter(t, client, r.Workspace.ID).
			MatchResources(matchAgentWithVersion).Wait()

		ctx := testutil.Context(t, testutil.WaitLong)
		clientInv, root := clitest.New(t,
			"-v",
			"--no-feature-warning",
			"--no-version-warning",
			"ping", r.Workspace.Name,
			"-n", "1",
		)
		clitest.SetupConfig(t, member, root)
		err := clientInv.WithContext(ctx).Run()
		require.NoError(t, err)

		require.Greater(t, called.Load(), int64(0), "expected coderd to be reached with custom headers")
		require.Greater(t, derpCalled.Load(), int64(0), "expected /derp to be called with custom headers")
	})

	t.Run("DisabledServers", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()

		logDir := t.TempDir()
		inv, _ := clitest.New(t,
			"agent",
			"--auth", "token",
			"--agent-token", r.AgentToken,
			"--agent-url", client.URL.String(),
			"--log-dir", logDir,
			"--pprof-address", "",
			"--prometheus-address", "",
			"--debug-address", "",
			"--socket-path", testutil.AgentSocketPath(t),
		)

		clitest.Start(t, inv)

		// Verify the agent is connected and working.
		resources := coderdtest.NewWorkspaceAgentWaiter(t, client, r.Workspace.ID).
			MatchResources(matchAgentWithVersion).Wait()
		require.Len(t, resources, 1)
		require.Len(t, resources[0].Agents, 1)
		require.NotEmpty(t, resources[0].Agents[0].Version)

		// Verify the servers are not listening by checking the log for disabled
		// messages.
		require.Eventually(t, func() bool {
			logContent, err := os.ReadFile(filepath.Join(logDir, "coder-agent.log"))
			if err != nil {
				return false
			}
			logStr := string(logContent)
			return strings.Contains(logStr, "pprof address is empty, disabling pprof server") &&
				strings.Contains(logStr, "prometheus address is empty, disabling prometheus server") &&
				strings.Contains(logStr, "debug address is empty, disabling debug server")
		}, testutil.WaitLong, testutil.IntervalMedium)
	})
}

func TestWorkspaceAgent_AISandbox(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("AI sandbox controller integration requires Linux")
	}
	t.Parallel()

	tempDir := t.TempDir()
	workspaceEnvFile := filepath.Join(tempDir, "workspace-script.env")
	workspaceConnectedMarker := filepath.Join(tempDir, "workspace-script-connected")
	workspaceScript := fmt.Sprintf(`env > %q
proxy_host="${CODER_EGRESS_PROXY%%:*}"
proxy_port="${CODER_EGRESS_PROXY##*:}"
bash -c 'exec 3<>/dev/tcp/$1/$2' _ "$proxy_host" "$proxy_port" || exit 1
printf connected > %q`, workspaceEnvFile, workspaceConnectedMarker)

	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	workspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}).WithAgent(func(agents []*sdkproto.Agent) []*sdkproto.Agent {
		agents[0].Scripts = []*sdkproto.Script{{
			DisplayName:    "AI sandbox",
			Script:         workspaceScript,
			RunOnStart:     true,
			TimeoutSeconds: 30,
		}}
		return agents
	}).Do()

	logDir := filepath.Join(tempDir, "logs")
	require.NoError(t, os.MkdirAll(logDir, 0o755))
	controllerEnvFile := filepath.Join(tempDir, "controller.env")
	destroyMarker := filepath.Join(tempDir, "destroyed")
	createScript := filepath.Join(tempDir, "create-sandbox")
	destroyScript := filepath.Join(tempDir, "destroy-sandbox")
	require.NoError(t, os.WriteFile(createScript, []byte(fmt.Sprintf("#!/bin/sh\nenv > %q\n", controllerEnvFile)), 0o600))
	require.NoError(t, os.Chmod(createScript, 0o700))
	require.NoError(t, os.WriteFile(destroyScript, []byte(fmt.Sprintf("#!/bin/sh\nprintf destroyed > %q\n", destroyMarker)), 0o600))
	require.NoError(t, os.Chmod(destroyScript, 0o700))

	processLog, err := os.Create(filepath.Join(tempDir, "process.log"))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = processLog.Close()
	})
	//nolint:gosec // Arguments contain only test-controlled values and coderdtest credentials.
	cmd := exec.CommandContext(context.Background(), "go", "run", "./cmd/coder",
		"agent",
		"--auth", "token",
		"--agent-token", workspace.AgentToken,
		"--agent-url", client.URL.String(),
		"--log-dir", logDir,
		"--log-human", "",
		"--pprof-address", "",
		"--prometheus-address", "",
		"--debug-address", "",
		"--socket-server-enabled=false",
		"--devcontainers-enable=false",
	)
	cmd.Dir = ".."
	configureProcessGroup(cmd)
	cmd.Env = append(os.Environ(),
		confine.EnvAISandboxCreateScript+"="+createScript,
		confine.EnvAISandboxDestroyScript+"="+destroyScript,
		confine.EnvAISandboxEgressEnforcement+"=advisory",
	)
	cmd.Stdout = processLog
	cmd.Stderr = processLog
	require.NoError(t, cmd.Start())
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	stopped := false
	t.Cleanup(func() {
		if stopped {
			return
		}
		_ = interruptProcessGroup(cmd)
		select {
		case <-done:
		case <-time.After(testutil.WaitMedium):
			_ = killProcessGroup(cmd)
			<-done
		}
	})

	var sandboxes []database.AISandbox
	eventuallyCtx := testutil.Context(t, testutil.WaitSuperLong)
	require.True(t, testutil.Eventually(eventuallyCtx, t, func(ctx context.Context) bool {
		var err error
		sandboxes, err = db.GetAISandboxesByWorkspaceID(dbauthz.AsSystemRestricted(ctx), workspace.Workspace.ID)
		return err == nil && len(sandboxes) == 1
	}, testutil.IntervalFast))
	sandbox := sandboxes[0]

	child, err := db.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(eventuallyCtx), sandbox.ChildAgentID)
	require.NoError(t, err)
	require.True(t, child.AIAgentID.Valid)
	require.Equal(t, sandbox.AIAgentID, child.AIAgentID.UUID)

	var controllerEnvironment map[string]string
	require.True(t, testutil.Eventually(eventuallyCtx, t, func(context.Context) bool {
		contents, err := os.ReadFile(controllerEnvFile)
		if err != nil {
			return false
		}
		controllerEnvironment = parseAgentEnvironment(string(contents))
		return true
	}, testutil.IntervalFast))
	require.Equal(t, client.URL.String(), controllerEnvironment[confine.EnvAIAgentURL])
	require.NotEmpty(t, controllerEnvironment[confine.EnvAIAgentToken])
	require.NotEmpty(t, controllerEnvironment[confine.EnvAISessionToken])
	require.Equal(t, sandbox.ID.String(), controllerEnvironment[confine.EnvSandboxID])
	proxyAddress := controllerEnvironment[confine.EnvEgressProxy]
	_, _, err = net.SplitHostPort(proxyAddress)
	require.NoError(t, err)

	var workspaceEnvironment map[string]string
	require.True(t, testutil.Eventually(eventuallyCtx, t, func(context.Context) bool {
		contents, err := os.ReadFile(workspaceEnvFile)
		if err != nil {
			return false
		}
		connected, err := os.ReadFile(workspaceConnectedMarker)
		if err != nil || string(connected) != "connected" {
			return false
		}
		workspaceEnvironment = parseAgentEnvironment(string(contents))
		return true
	}, testutil.IntervalFast))
	require.Equal(t, proxyAddress, workspaceEnvironment[confine.EnvEgressProxy])
	require.Equal(t, sandbox.ID.String(), workspaceEnvironment[confine.EnvSandboxID])

	var sessions []codersdk.AISandboxSession
	require.True(t, testutil.Eventually(eventuallyCtx, t, func(ctx context.Context) bool {
		var err error
		sessions, err = client.WorkspaceAISandboxSessions(ctx, workspace.Workspace.ID)
		return err == nil && len(sessions) == 1
	}, testutil.IntervalFast))
	require.Equal(t, sandbox.ChildAgentID, sessions[0].ConfinedAgentID)
	require.Nil(t, sessions[0].EndedAt)

	require.NoError(t, interruptProcessGroup(cmd))
	shutdownCtx := testutil.Context(t, testutil.WaitSuperLong)
	select {
	case <-done:
		stopped = true
	case <-shutdownCtx.Done():
		require.NoError(t, shutdownCtx.Err(), "agent process did not stop")
	}

	closedCtx := testutil.Context(t, testutil.WaitLong)
	require.True(t, testutil.Eventually(closedCtx, t, func(ctx context.Context) bool {
		marker, err := os.ReadFile(destroyMarker)
		if err != nil || string(marker) != "destroyed" {
			return false
		}
		sessions, err = client.WorkspaceAISandboxSessions(ctx, workspace.Workspace.ID)
		return err == nil && len(sessions) == 1 && sessions[0].EndedAt != nil
	}, testutil.IntervalFast))
}

func parseAgentEnvironment(contents string) map[string]string {
	environment := make(map[string]string)
	for line := range strings.Lines(contents) {
		key, value, ok := strings.Cut(strings.TrimSuffix(line, "\n"), "=")
		if ok {
			environment[key] = value
		}
	}
	return environment
}

func TestWorkspaceAgent_ConfineProxy(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("agent confinement requires Linux")
	}
	t.Parallel()

	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	workspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}).WithAgent().Do()

	logDir := t.TempDir()
	processLog, err := os.Create(filepath.Join(logDir, "process.log"))
	require.NoError(t, err)
	ctx := testutil.Context(t, testutil.WaitLong)
	//nolint:gosec // Arguments contain only test-controlled values and coderdtest credentials.
	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/coder",
		"agent",
		"--auth", "token",
		"--agent-token", workspace.AgentToken,
		"--agent-url", client.URL.String(),
		"--confine", "proxy",
		"--log-dir", logDir,
		"--log-human", "",
		"--pprof-address", "",
		"--prometheus-address", "",
		"--debug-address", "",
		"--socket-server-enabled=false",
		"--devcontainers-enable=false",
	)
	cmd.Dir = ".."
	cmd.Stdout = processLog
	cmd.Stderr = processLog
	require.NoError(t, cmd.Start())
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-done:
		case <-time.After(testutil.WaitMedium):
			_ = cmd.Process.Kill()
			<-done
		}
		_ = processLog.Close()
	})

	coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.Workspace.ID).
		MatchResources(matchAgentWithVersion).Wait()

	var proxyAddress string
	proxyPattern := regexp.MustCompile(`http://127\.0\.0\.1:[0-9]+`)
	initLogPath := filepath.Join(logDir, "coder-agent-init.log")
	require.Eventually(t, func() bool {
		logContent, err := os.ReadFile(initLogPath)
		if err != nil {
			return false
		}
		for _, line := range strings.Split(string(logContent), "\n") {
			if !strings.Contains(line, "ai egress proxy started") {
				continue
			}
			proxyURL := proxyPattern.FindString(line)
			if proxyURL == "" {
				return false
			}
			proxyAddress = strings.TrimPrefix(proxyURL, "http://")
			// The policy endpoint serves the template's revision-0
			// default (deny-all beyond the implicit coderd allow), so
			// the supervisor bootstraps with a fetched policy rather
			// than the degraded fetch-failure fallback.
			return strings.Contains(string(logContent), "applied ai egress policy")
		}
		return false
	}, testutil.WaitLong, testutil.IntervalMedium)

	conn, err := (&net.Dialer{}).DialContext(testutil.Context(t, testutil.WaitShort), "tcp", proxyAddress)
	require.NoError(t, err)
	defer conn.Close()
	_, err = fmt.Fprint(conn, "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n")
	require.NoError(t, err)
	response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodConnect})
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusForbidden, response.StatusCode)
}

func matchAgentWithVersion(rs []codersdk.WorkspaceResource) bool {
	if len(rs) < 1 {
		return false
	}
	if len(rs[0].Agents) < 1 {
		return false
	}
	if rs[0].Agents[0].Version == "" {
		return false
	}
	return true
}

func matchAgentWithSubsystems(rs []codersdk.WorkspaceResource) bool {
	if len(rs) < 1 {
		return false
	}
	if len(rs[0].Agents) < 1 {
		return false
	}
	if len(rs[0].Agents[0].Subsystems) < 1 {
		return false
	}
	return true
}
