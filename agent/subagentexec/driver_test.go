//go:build unix

package subagentexec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

// testDeclaredName is deliberately distinctive: no private path may ever
// contain it, because the layout is derived from UUIDs only.
const testDeclaredName = "declared-name-must-not-appear"

// testStopGrace is short enough that the SIGKILL escalation test does not
// have to wait for the production grace period.
const testStopGrace = testutil.IntervalMedium

// newTestDriver returns a driver rooted in a fresh temporary state root.
func newTestDriver(t *testing.T, mutate func(*ScriptDriverConfig)) *ScriptDriver {
	t.Helper()

	cfg := ScriptDriverConfig{
		Logger:          testLogger(t),
		StateRoot:       filepath.Join(t.TempDir(), "subagentexec"),
		AgentScope:      uuid.NewString(),
		CoderURL:        "https://coder.example.com",
		CoderBinaryPath: "/opt/coder/bin/coder",
		StopGracePeriod: testStopGrace,
		CleanupTimeout:  testutil.WaitShort,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	driver, err := NewScriptDriver(cfg)
	require.NoError(t, err)
	return driver
}

// testDriverLaunch builds a launch whose declaration carries script as its
// driver body, which is what the manifest holds for protocol v1.
func testDriverLaunch(script string) Launch {
	decl := testDeclaration()
	decl.Name = testDeclaredName
	decl.Driver = script
	return Launch{
		Declaration:        decl,
		ChildAgentID:       uuid.New(),
		AcquisitionVersion: 7,
		authToken:          testAuthToken,
	}
}

// recordingScript returns a driver script that records its argument list,
// its environment, and its protocol document into outDir for both
// operations, then runs body. Recording into outDir, which is outside the
// per-execution state, keeps the evidence after cleanup removes the state.
func recordingScript(outDir, body string) string {
	return fmt.Sprintf(`#!/bin/sh
op="$1"
printf '%%s\n' "$0" "$#" "$1" "$2" > %[1]q/argv-"$op".txt
cp "$2" %[1]q/input-"$op".json
env > %[1]q/env-"$op".txt
%[2]s
`, outDir, body)
}

// blockUntilSignalled keeps the run operation in the foreground and lets
// the cleanup operation return immediately.
const blockUntilSignalled = `if [ "$op" = "cleanup" ]; then exit 0; fi
touch "$READY_MARKER"
while true; do sleep 0.05; done`

// readyScript composes a recording script that blocks in the run
// operation, announcing itself by creating outDir/ready.
func readyScript(outDir string) string {
	body := strings.ReplaceAll(blockUntilSignalled, `"$READY_MARKER"`, fmt.Sprintf("%q", filepath.Join(outDir, "ready")))
	return recordingScript(outDir, body)
}

func requirePerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	require.NoError(t, err, "stat %s", path)
	require.Equal(t, want, info.Mode().Perm(), "permissions of %s", path)
}

func requireEventuallyExists(t *testing.T, path string) {
	t.Helper()
	require.Eventually(t, func() bool {
		_, err := os.Lstat(path)
		return err == nil
	}, testutil.WaitShort, testutil.IntervalFast, "waiting for %s", path)
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err, "read %s", path)
	return string(content)
}

// syncBuffer collects log output written from the driver's goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestScriptDriver_PrivateStateLayoutAndModes(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	outDir := t.TempDir()
	driver := newTestDriver(t, nil)
	launch := testDriverLaunch(readyScript(outDir))

	proc, err := driver.Start(ctx, launch)
	require.NoError(t, err)
	requireEventuallyExists(t, filepath.Join(outDir, "ready"))

	paths := driver.paths(launch.Declaration.ExecutionID)

	// The layout is state-root/agent-scope/execution-id, all UUIDs.
	require.Equal(t, filepath.Join(driver.stateRoot, driver.agentScope, launch.Declaration.ExecutionID.String()), paths.dir)
	require.NotContains(t, paths.dir, testDeclaredName)
	require.DirExists(t, paths.dir)

	requirePerm(t, driver.stateRoot, 0o700)
	requirePerm(t, filepath.Dir(paths.dir), 0o700)
	requirePerm(t, paths.dir, 0o700)
	requirePerm(t, paths.home, 0o700)
	requirePerm(t, paths.tmp, 0o700)
	requirePerm(t, paths.runtime, 0o700)

	requirePerm(t, paths.driver, 0o700)
	require.Equal(t, launch.Declaration.Driver, readFileString(t, paths.driver))

	requirePerm(t, paths.token, 0o600)
	require.Equal(t, testAuthToken, readFileString(t, paths.token))

	requirePerm(t, paths.runInput, 0o600)
	requirePerm(t, paths.cleanupInput, 0o600)

	// Every private path stays outside the declared shared directory.
	for _, path := range []string{paths.dir, paths.token, paths.runInput, paths.cleanupInput, paths.home, paths.tmp, paths.runtime} {
		require.False(t, strings.HasPrefix(path, launch.Declaration.SharedHostPath),
			"%s must not be inside the shared project directory", path)
	}

	require.NoError(t, proc.Stop(ctx))
	require.NoDirExists(t, paths.dir)
}

func TestScriptDriver_ProtocolInputOmitsToken(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	outDir := t.TempDir()
	driver := newTestDriver(t, nil)
	launch := testDriverLaunch(recordingScript(outDir, "exit 0"))

	proc, err := driver.Start(ctx, launch)
	require.NoError(t, err)
	require.NoError(t, proc.Wait())

	paths := driver.paths(launch.Declaration.ExecutionID)
	for _, op := range []Operation{OperationRun, OperationCleanup} {
		raw := readFileString(t, filepath.Join(outDir, fmt.Sprintf("input-%s.json", op)))
		require.NotContains(t, raw, testAuthToken, "%s input must not carry the token value", op)

		var input DriverInput
		require.NoError(t, json.Unmarshal([]byte(raw), &input))
		require.Equal(t, op, input.Operation)
		require.Equal(t, ProtocolVersion, input.ProtocolVersion)
		require.Equal(t, launch.Declaration.ExecutionID, input.ExecutionID)
		require.Equal(t, launch.Declaration.Generation, input.Generation)
		require.Equal(t, launch.ChildAgentID, input.ChildAgentID)
		require.Equal(t, testDeclaredName, input.ChildAgentName)
		require.Equal(t, "https://coder.example.com", input.CoderURL)
		require.Equal(t, "/opt/coder/bin/coder", input.CoderBinaryPath)
		require.Equal(t, paths.token, input.TokenFilePath)
		require.Equal(t, launch.Declaration.SharedHostPath, input.SharedHostPath)
		require.Equal(t, launch.Declaration.SharedChildPath, input.SharedChildPath)
		require.Equal(t, paths.dir, input.StatePath)
		require.Equal(t, paths.home, input.HomePath)
		require.Equal(t, paths.tmp, input.TmpPath)
		require.Equal(t, paths.runtime, input.RuntimePath)

		// The document names the token file and nothing more.
		var fields map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &fields))
		require.NotContains(t, fields, "auth_token")
		require.NotContains(t, fields, "token")
	}
}

func TestScriptDriver_ArgvContract(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	outDir := t.TempDir()
	driver := newTestDriver(t, nil)
	launch := testDriverLaunch(recordingScript(outDir, "exit 0"))

	proc, err := driver.Start(ctx, launch)
	require.NoError(t, err)
	require.NoError(t, proc.Wait())

	paths := driver.paths(launch.Declaration.ExecutionID)
	for op, inputPath := range map[Operation]string{
		OperationRun:     paths.runInput,
		OperationCleanup: paths.cleanupInput,
	} {
		argv := strings.Split(strings.TrimSuffix(readFileString(t, filepath.Join(outDir, fmt.Sprintf("argv-%s.txt", op))), "\n"), "\n")
		require.Equal(t, []string{paths.driver, "2", string(op), inputPath}, argv,
			"driver must be invoked as <driver-path> <operation> <json-path>")
	}
}

// This case cannot run in parallel: it seeds the launcher's own
// environment with t.Setenv to prove nothing is inherited.
func TestScriptDriver_EnvironmentIsBuiltFromScratch(t *testing.T) {
	ctx := testutil.Context(t, testutil.WaitShort)

	// Seed the launcher's own environment with material a driver must
	// never inherit.
	secrets := map[string]string{
		"CODER_AGENT_TOKEN":     "parent-agent-token",
		"SSH_AUTH_SOCK":         "/tmp/parent-ssh-agent.sock",
		"GIT_ASKPASS":           "/usr/local/bin/parent-askpass",
		"GIT_SSH_COMMAND":       "ssh -i /home/parent/.ssh/id_ed25519",
		"DOCKER_HOST":           "unix:///var/run/docker.sock",
		"KUBECONFIG":            "/home/parent/.kube/config",
		"AWS_SECRET_ACCESS_KEY": "parent-aws-secret",
		"CODER_SESSION_TOKEN":   "parent-owner-session",
	}
	for name, value := range secrets {
		t.Setenv(name, value)
	}

	outDir := t.TempDir()
	driver := newTestDriver(t, nil)
	launch := testDriverLaunch(recordingScript(outDir, "exit 0"))

	proc, err := driver.Start(ctx, launch)
	require.NoError(t, err)
	require.NoError(t, proc.Wait())

	paths := driver.paths(launch.Declaration.ExecutionID)

	// The allowlist itself is exact: only the four controlled variables
	// are ever handed to a driver.
	require.Equal(t, []string{
		"PATH=" + DefaultPath,
		"HOME=" + paths.home,
		"TMPDIR=" + paths.tmp,
		"XDG_RUNTIME_DIR=" + paths.runtime,
	}, driver.environ(paths))

	observed := map[string]string{}
	for _, line := range strings.Split(readFileString(t, filepath.Join(outDir, "env-run.txt")), "\n") {
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		observed[name] = value
	}

	for name := range secrets {
		require.NotContains(t, observed, name, "%s must not reach the driver", name)
	}
	for _, value := range secrets {
		for name, observedValue := range observed {
			require.NotEqual(t, value, observedValue, "%s leaked a parent secret value", name)
		}
	}
	require.NotContains(t, readFileString(t, filepath.Join(outDir, "env-run.txt")), testAuthToken)

	require.Equal(t, DefaultPath, observed["PATH"])
	require.Equal(t, paths.home, observed["HOME"])
	require.Equal(t, paths.tmp, observed["TMPDIR"])
	require.Equal(t, paths.runtime, observed["XDG_RUNTIME_DIR"])
}

func TestScriptDriver_ForegroundRunResult(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		driver := newTestDriver(t, nil)
		launch := testDriverLaunch("#!/bin/sh\nexit 0\n")

		proc, err := driver.Start(ctx, launch)
		require.NoError(t, err)
		require.NoError(t, proc.Wait())
	})

	t.Run("NonZeroExit", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		driver := newTestDriver(t, nil)
		launch := testDriverLaunch("#!/bin/sh\nif [ \"$1\" = cleanup ]; then exit 0; fi\nexit 3\n")

		proc, err := driver.Start(ctx, launch)
		require.NoError(t, err)

		err = proc.Wait()
		require.Error(t, err)
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr)
		require.Equal(t, 3, exitErr.ExitCode())
	})
}

func TestScriptDriver_TokenFileLifetime(t *testing.T) {
	t.Parallel()

	t.Run("RemovedOnNormalExit", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		driver := newTestDriver(t, nil)
		launch := testDriverLaunch("#!/bin/sh\nexit 0\n")

		proc, err := driver.Start(ctx, launch)
		require.NoError(t, err)
		require.NoError(t, proc.Wait())

		paths := driver.paths(launch.Declaration.ExecutionID)
		require.NoFileExists(t, paths.token)
		require.NoDirExists(t, paths.dir)
	})

	t.Run("RemovedOnStartFailure", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		driver := newTestDriver(t, nil)
		// An absolute but nonexistent interpreter passes validation and
		// fails at exec time, which is the failure path that must not
		// leave a token file behind.
		launch := testDriverLaunch("#!/nonexistent/interpreter\n")

		_, err := driver.Start(ctx, launch)
		require.Error(t, err)

		paths := driver.paths(launch.Declaration.ExecutionID)
		require.NoFileExists(t, paths.token)
		require.NoDirExists(t, paths.dir)
	})

	t.Run("RemovedOnStop", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		outDir := t.TempDir()
		driver := newTestDriver(t, nil)
		launch := testDriverLaunch(readyScript(outDir))

		proc, err := driver.Start(ctx, launch)
		require.NoError(t, err)
		requireEventuallyExists(t, filepath.Join(outDir, "ready"))

		paths := driver.paths(launch.Declaration.ExecutionID)
		require.FileExists(t, paths.token)

		require.NoError(t, proc.Stop(ctx))
		require.NoFileExists(t, paths.token)
		require.NoDirExists(t, paths.dir)
	})
}

func TestScriptDriver_RejectsInvalidDriverBody(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		protocol int32
		body     string
		contains string
	}{
		{name: "UnsupportedProtocol", protocol: 2, body: "#!/bin/sh\nexit 0\n", contains: "unsupported driver protocol"},
		{name: "ZeroProtocol", protocol: 0, body: "#!/bin/sh\nexit 0\n", contains: "unsupported driver protocol"},
		{name: "EmptyBody", protocol: 1, body: "", contains: "empty driver body"},
		{name: "WhitespaceBody", protocol: 1, body: "   \n\t\n", contains: "empty driver body"},
		{name: "NoShebang", protocol: 1, body: "exit 0\n", contains: "must begin with a shebang"},
		{name: "EmptyShebang", protocol: 1, body: "#!\nexit 0\n", contains: "empty shebang"},
		{name: "RelativeInterpreter", protocol: 1, body: "#!sh\nexit 0\n", contains: "absolute interpreter path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)

			driver := newTestDriver(t, nil)
			launch := testDriverLaunch(tc.body)
			launch.Declaration.DriverProtocol = tc.protocol

			_, err := driver.Start(ctx, launch)
			require.ErrorContains(t, err, tc.contains)

			// A rejected declaration never reaches the filesystem.
			require.NoDirExists(t, driver.paths(launch.Declaration.ExecutionID).dir)
		})
	}
}

func TestScriptDriver_StopTerminatesGracefully(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	outDir := t.TempDir()
	// A generous grace period keeps the assertion below about the driver
	// honoring SIGTERM, not about scheduling latency.
	gracePeriod := testutil.WaitShort
	driver := newTestDriver(t, func(cfg *ScriptDriverConfig) { cfg.StopGracePeriod = gracePeriod })
	launch := testDriverLaunch(fmt.Sprintf(`#!/bin/sh
if [ "$1" = cleanup ]; then
	printf 'cleanup\n' >> %[1]q/operations.txt
	exit 0
fi
trap 'printf "term\n" >> %[1]q/operations.txt; exit 0' TERM
touch %[1]q/ready
while true; do sleep 0.05; done
`, outDir))

	proc, err := driver.Start(ctx, launch)
	require.NoError(t, err)
	requireEventuallyExists(t, filepath.Join(outDir, "ready"))

	start := time.Now()
	require.NoError(t, proc.Stop(ctx))
	// A driver that honors SIGTERM must not wait out the grace period.
	require.Less(t, time.Since(start), gracePeriod)
	require.NoError(t, proc.Wait())

	require.Equal(t, []string{"term", "cleanup"},
		strings.Split(strings.TrimSuffix(readFileString(t, filepath.Join(outDir, "operations.txt")), "\n"), "\n"))
}

func TestScriptDriver_StopEscalatesToKill(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	outDir := t.TempDir()
	driver := newTestDriver(t, nil)
	launch := testDriverLaunch(fmt.Sprintf(`#!/bin/sh
if [ "$1" = cleanup ]; then exit 0; fi
trap '' TERM
touch %[1]q/ready
while true; do sleep 0.05; done
`, outDir))

	proc, err := driver.Start(ctx, launch)
	require.NoError(t, err)
	requireEventuallyExists(t, filepath.Join(outDir, "ready"))

	start := time.Now()
	require.NoError(t, proc.Stop(ctx))
	require.GreaterOrEqual(t, time.Since(start), testStopGrace,
		"stop must wait out the grace period before killing")

	err = proc.Wait()
	require.Error(t, err)
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	require.Contains(t, exitErr.ProcessState.String(), "killed")

	require.NoDirExists(t, driver.paths(launch.Declaration.ExecutionID).dir)
}

func TestScriptDriver_CleanupRunsExactlyOnce(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	outDir := t.TempDir()
	driver := newTestDriver(t, nil)
	launch := testDriverLaunch(fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$1" >> %[1]q/operations.txt
exit 0
`, outDir))

	proc, err := driver.Start(ctx, launch)
	require.NoError(t, err)
	require.NoError(t, proc.Wait())

	// Stopping an execution that already ended, and waiting again, must
	// not repeat the cleanup operation.
	require.NoError(t, proc.Stop(ctx))
	require.NoError(t, proc.Wait())
	require.NoError(t, proc.Stop(ctx))

	require.Equal(t, []string{"run", "cleanup"},
		strings.Split(strings.TrimSuffix(readFileString(t, filepath.Join(outDir, "operations.txt")), "\n"), "\n"))
}

func TestScriptDriver_CleanupFailureStillRemovesState(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	driver := newTestDriver(t, nil)
	launch := testDriverLaunch("#!/bin/sh\nif [ \"$1\" = cleanup ]; then exit 9; fi\nexit 0\n")

	proc, err := driver.Start(ctx, launch)
	require.NoError(t, err)
	require.NoError(t, proc.Wait())

	paths := driver.paths(launch.Declaration.ExecutionID)
	require.NoFileExists(t, paths.token)
	require.NoDirExists(t, paths.dir)
}

func TestScriptDriver_RedactsTokenFromOutput(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	logs := &syncBuffer{}
	driver := newTestDriver(t, func(cfg *ScriptDriverConfig) {
		cfg.Logger = slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug)
	})
	// The script finds its token file through the protocol document and
	// prints it to both streams, which is exactly what must not reach the
	// parent agent's log.
	launch := testDriverLaunch(`#!/bin/sh
if [ "$1" = cleanup ]; then exit 0; fi
token_file="$(sed -n 's/.*"token_file_path":"\([^"]*\)".*/\1/p' "$2")"
printf 'stdout token: %s\n' "$(cat "$token_file")"
printf 'stderr token: %s\n' "$(cat "$token_file")" >&2
exit 0
`)

	proc, err := driver.Start(ctx, launch)
	require.NoError(t, err)
	require.NoError(t, proc.Wait())

	captured := logs.String()
	require.Contains(t, captured, "stdout token: "+redactedPlaceholder)
	require.Contains(t, captured, "stderr token: "+redactedPlaceholder)
	require.NotContains(t, captured, testAuthToken)
}

func TestNewScriptDriver_Validation(t *testing.T) {
	t.Parallel()

	base := func() ScriptDriverConfig {
		return ScriptDriverConfig{
			StateRoot:       t.TempDir(),
			CoderURL:        "https://coder.example.com",
			CoderBinaryPath: "/opt/coder/bin/coder",
		}
	}

	t.Run("Defaults", func(t *testing.T) {
		t.Parallel()
		driver, err := NewScriptDriver(base())
		require.NoError(t, err)
		require.Equal(t, defaultAgentScope, driver.agentScope)
		require.Equal(t, DefaultPath, driver.path)
		require.Equal(t, agentexec.DefaultExecer, driver.execer)
		require.Equal(t, defaultStopGracePeriod, driver.stopGrace)
		require.Equal(t, defaultCleanupTimeout, driver.cleanupTimeout)
	})

	for _, tc := range []struct {
		name     string
		mutate   func(*ScriptDriverConfig)
		contains string
	}{
		{name: "NoStateRoot", mutate: func(c *ScriptDriverConfig) { c.StateRoot = "" }, contains: "state root is required"},
		{name: "RelativeStateRoot", mutate: func(c *ScriptDriverConfig) { c.StateRoot = "state" }, contains: "must be absolute"},
		{name: "NoCoderURL", mutate: func(c *ScriptDriverConfig) { c.CoderURL = "" }, contains: "coder url is required"},
		{name: "NoCoderBinary", mutate: func(c *ScriptDriverConfig) { c.CoderBinaryPath = "" }, contains: "coder binary path is required"},
		{name: "UnsafeAgentScope", mutate: func(c *ScriptDriverConfig) { c.AgentScope = "../escape" }, contains: "not a safe path segment"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := base()
			tc.mutate(&cfg)
			_, err := NewScriptDriver(cfg)
			require.ErrorContains(t, err, tc.contains)
		})
	}
}

func TestScriptDriver_RejectsLaunchWithoutToken(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	driver := newTestDriver(t, nil)
	launch := testDriverLaunch("#!/bin/sh\nexit 0\n")
	launch.authToken = ""

	_, err := driver.Start(ctx, launch)
	require.ErrorContains(t, err, "no child auth token")
	require.NoDirExists(t, driver.paths(launch.Declaration.ExecutionID).dir)
}

func TestScriptDriver_ReusesExecutionDirectoryAcrossGenerations(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	outDir := t.TempDir()
	driver := newTestDriver(t, nil)
	launch := testDriverLaunch(recordingScript(outDir, "exit 0"))

	proc, err := driver.Start(ctx, launch)
	require.NoError(t, err)
	require.NoError(t, proc.Wait())

	// A new generation of the same declaration reuses the execution's
	// UUID directory after the previous run cleaned it up.
	next := launch
	next.Declaration.Generation = uuid.New()
	proc, err = driver.Start(ctx, next)
	require.NoError(t, err)
	require.NoError(t, proc.Wait())

	var input DriverInput
	require.NoError(t, json.Unmarshal([]byte(readFileString(t, filepath.Join(outDir, "input-run.json"))), &input))
	require.Equal(t, next.Declaration.Generation, input.Generation)
}

// TestManager_ScriptDriverLifecycle covers the normal declaration path
// end to end: the manager launches the concrete driver, reports RUNNING,
// and reports STOPPED once the declaration is withdrawn.
func TestManager_ScriptDriverLifecycle(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	outDir := t.TempDir()
	driver := newTestDriver(t, nil)
	controller := newFakeController()
	m := newManager(t, driver)

	decl := testDeclaration()
	decl.Name = testDeclaredName
	decl.Driver = readyScript(outDir)
	m.Reconcile(controller, []agentsdk.SubagentExecution{decl})

	report := testutil.RequireReceive(ctx, t, controller.reportCh)
	require.Equal(t, proto.ReportSubagentExecutionStatusRequest_RUNNING, report.GetStatus())
	requireEventuallyExists(t, filepath.Join(outDir, "ready"))

	paths := driver.paths(decl.ExecutionID)
	require.FileExists(t, paths.token)
	require.Equal(t, testAuthToken, readFileString(t, paths.token))

	statuses := m.Statuses()
	require.Len(t, statuses, 1)
	require.Equal(t, StateRunning, statuses[0].State)

	// Withdrawing the declaration stops the driver and reclaims the token.
	m.Reconcile(controller, nil)
	report = testutil.RequireReceive(ctx, t, controller.reportCh)
	require.Equal(t, proto.ReportSubagentExecutionStatusRequest_STOPPED, report.GetStatus())
	require.NoFileExists(t, paths.token)
	require.NoDirExists(t, paths.dir)
}

func TestManager_ScriptDriverReportsFailure(t *testing.T) {
	t.Parallel()

	t.Run("UnexpectedExit", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		driver := newTestDriver(t, nil)
		controller := newFakeController()
		m := newManager(t, driver)

		decl := testDeclaration()
		decl.Driver = "#!/bin/sh\nif [ \"$1\" = cleanup ]; then exit 0; fi\nexit 5\n"
		m.Reconcile(controller, []agentsdk.SubagentExecution{decl})

		// RUNNING is reported when the foreground driver starts, then the
		// early exit is reported as a failure.
		report := testutil.RequireReceive(ctx, t, controller.reportCh)
		require.Equal(t, proto.ReportSubagentExecutionStatusRequest_RUNNING, report.GetStatus())

		report = testutil.RequireReceive(ctx, t, controller.reportCh)
		require.Equal(t, proto.ReportSubagentExecutionStatusRequest_FAILED, report.GetStatus())
		require.Contains(t, report.GetError(), "exit status 5")
		require.NotContains(t, report.GetError(), testAuthToken)
	})

	t.Run("StartFailure", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		driver := newTestDriver(t, nil)
		controller := newFakeController()
		m := newManager(t, driver)

		decl := testDeclaration()
		decl.Driver = "not-a-script"
		m.Reconcile(controller, []agentsdk.SubagentExecution{decl})

		report := testutil.RequireReceive(ctx, t, controller.reportCh)
		require.Equal(t, proto.ReportSubagentExecutionStatusRequest_FAILED, report.GetStatus())
		require.Contains(t, report.GetError(), "shebang")
		require.NotContains(t, report.GetError(), testAuthToken)

		statuses := m.Statuses()
		require.Len(t, statuses, 1)
		require.Equal(t, StateFailed, statuses[0].State)
	})
}
