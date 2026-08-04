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

// driverStateRoot returns the canonical state root Start resolves the
// configured one to. It falls back to the configured path when the root does
// not exist, which is what a rejected launch leaves behind.
func driverStateRoot(t *testing.T, d *ScriptDriver) string {
	t.Helper()

	resolved, err := filepath.EvalSymlinks(d.stateRoot)
	if err != nil {
		return d.stateRoot
	}
	return filepath.Clean(resolved)
}

// driverPaths returns the execution layout the driver uses, resolved the
// same way Start resolves it.
func driverPaths(t *testing.T, d *ScriptDriver, executionID uuid.UUID) executionPaths {
	t.Helper()

	return d.paths(driverStateRoot(t, d), executionID)
}

// requireNoTokenOnDisk fails when the child's token, or any private file the
// launcher writes, survived under any of roots. It is how a rejected launch
// is shown to have left no credentials behind.
func requireNoTokenOnDisk(t *testing.T, roots ...string) {
	t.Helper()

	for _, root := range roots {
		require.NoError(t, filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			require.NotEqual(t, tokenFileName, entry.Name(), "token file left behind at %s", path)
			if !entry.Type().IsRegular() {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			require.NotContains(t, string(content), testAuthToken, "token value left behind in %s", path)
			return nil
		}), "walk %s", root)
	}
}

// testDriverLaunch builds a launch whose declaration carries script as its
// driver body, which is what the manifest holds for protocol v1. The
// declared shared path is a real directory outside the driver's state root,
// because the launcher resolves it before it writes anything.
func testDriverLaunch(t *testing.T, script string) Launch {
	t.Helper()

	decl := testDeclaration()
	decl.Name = testDeclaredName
	decl.Driver = script
	decl.SharedHostPath = t.TempDir()
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
	launch := testDriverLaunch(t, readyScript(outDir))

	proc, err := driver.Start(ctx, launch)
	require.NoError(t, err)
	requireEventuallyExists(t, filepath.Join(outDir, "ready"))

	paths := driverPaths(t, driver, launch.Declaration.ExecutionID)

	// The layout is state-root/agent-scope/execution-id, all UUIDs.
	require.Equal(t, filepath.Join(driverStateRoot(t, driver), driver.agentScope, launch.Declaration.ExecutionID.String()), paths.dir)
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
	launch := testDriverLaunch(t, recordingScript(outDir, "exit 0"))

	proc, err := driver.Start(ctx, launch)
	require.NoError(t, err)
	require.NoError(t, proc.Wait())

	paths := driverPaths(t, driver, launch.Declaration.ExecutionID)
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
		// The document carries the resolved shared path, which is what the
		// launcher validated the private state against.
		resolvedShared, err := filepath.EvalSymlinks(launch.Declaration.SharedHostPath)
		require.NoError(t, err)
		require.Equal(t, filepath.Clean(resolvedShared), input.SharedHostPath)
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
	launch := testDriverLaunch(t, recordingScript(outDir, "exit 0"))

	proc, err := driver.Start(ctx, launch)
	require.NoError(t, err)
	require.NoError(t, proc.Wait())

	paths := driverPaths(t, driver, launch.Declaration.ExecutionID)
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
	launch := testDriverLaunch(t, recordingScript(outDir, "exit 0"))

	proc, err := driver.Start(ctx, launch)
	require.NoError(t, err)
	require.NoError(t, proc.Wait())

	paths := driverPaths(t, driver, launch.Declaration.ExecutionID)

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
		launch := testDriverLaunch(t, "#!/bin/sh\nexit 0\n")

		proc, err := driver.Start(ctx, launch)
		require.NoError(t, err)
		require.NoError(t, proc.Wait())
	})

	t.Run("NonZeroExit", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		driver := newTestDriver(t, nil)
		launch := testDriverLaunch(t, "#!/bin/sh\nif [ \"$1\" = cleanup ]; then exit 0; fi\nexit 3\n")

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
		launch := testDriverLaunch(t, "#!/bin/sh\nexit 0\n")

		proc, err := driver.Start(ctx, launch)
		require.NoError(t, err)
		require.NoError(t, proc.Wait())

		paths := driverPaths(t, driver, launch.Declaration.ExecutionID)
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
		launch := testDriverLaunch(t, "#!/nonexistent/interpreter\n")

		_, err := driver.Start(ctx, launch)
		require.Error(t, err)

		paths := driverPaths(t, driver, launch.Declaration.ExecutionID)
		require.NoFileExists(t, paths.token)
		require.NoDirExists(t, paths.dir)
	})

	t.Run("RemovedOnStop", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		outDir := t.TempDir()
		driver := newTestDriver(t, nil)
		launch := testDriverLaunch(t, readyScript(outDir))

		proc, err := driver.Start(ctx, launch)
		require.NoError(t, err)
		requireEventuallyExists(t, filepath.Join(outDir, "ready"))

		paths := driverPaths(t, driver, launch.Declaration.ExecutionID)
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
			launch := testDriverLaunch(t, tc.body)
			launch.Declaration.DriverProtocol = tc.protocol

			_, err := driver.Start(ctx, launch)
			require.ErrorContains(t, err, tc.contains)

			// A rejected declaration never reaches the filesystem.
			require.NoDirExists(t, driverPaths(t, driver, launch.Declaration.ExecutionID).dir)
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
	launch := testDriverLaunch(t, fmt.Sprintf(`#!/bin/sh
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
	launch := testDriverLaunch(t, fmt.Sprintf(`#!/bin/sh
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

	require.NoDirExists(t, driverPaths(t, driver, launch.Declaration.ExecutionID).dir)
}

func TestScriptDriver_CleanupRunsExactlyOnce(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	outDir := t.TempDir()
	driver := newTestDriver(t, nil)
	launch := testDriverLaunch(t, fmt.Sprintf(`#!/bin/sh
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
	launch := testDriverLaunch(t, "#!/bin/sh\nif [ \"$1\" = cleanup ]; then exit 9; fi\nexit 0\n")

	proc, err := driver.Start(ctx, launch)
	require.NoError(t, err)
	require.NoError(t, proc.Wait())

	paths := driverPaths(t, driver, launch.Declaration.ExecutionID)
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
	launch := testDriverLaunch(t, `#!/bin/sh
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

func TestScriptDriver_RejectsStateInsideSharedPath(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	driver := newTestDriver(t, nil)
	launch := testDriverLaunch(t, "#!/bin/sh\nexit 0\n")
	// A declaration that shares the directory holding the token file, or
	// any parent of it, must not launch at all.
	launch.Declaration.SharedHostPath = filepath.Dir(driver.stateRoot)

	_, err := driver.Start(ctx, launch)
	require.ErrorContains(t, err, "is inside the declared shared path")
	require.NoDirExists(t, driverPaths(t, driver, launch.Declaration.ExecutionID).dir)
}

func TestScriptDriver_RejectsSharedPathInsideState(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	driver := newTestDriver(t, nil)
	launch := testDriverLaunch(t, "#!/bin/sh\nexit 0\n")
	// A state root that contains the shared path would let the owner of the
	// shared tree walk up into the private state.
	shared := filepath.Join(driver.stateRoot, "project")
	require.NoError(t, os.MkdirAll(shared, 0o700))
	launch.Declaration.SharedHostPath = shared

	_, err := driver.Start(ctx, launch)
	require.ErrorContains(t, err, "contains the declared shared path")
	requireNoTokenOnDisk(t, driver.stateRoot)
}

func TestScriptDriver_RejectsUnresolvableSharedPath(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	for _, tc := range []struct {
		name     string
		shared   func(t *testing.T) string
		contains string
	}{
		{
			name:     "Empty",
			shared:   func(*testing.T) string { return "" },
			contains: "no shared host path",
		},
		{
			name:     "Relative",
			shared:   func(*testing.T) string { return "project" },
			contains: "must be absolute",
		},
		{
			name:     "Missing",
			shared:   func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing") },
			contains: "resolve declared shared path",
		},
		{
			name: "NotADirectory",
			shared: func(t *testing.T) string {
				file := filepath.Join(t.TempDir(), "file")
				require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
				return file
			},
			contains: "is not a directory",
		},
		{
			name: "DanglingSymlink",
			shared: func(t *testing.T) string {
				link := filepath.Join(t.TempDir(), "link")
				require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "missing"), link))
				return link
			},
			contains: "resolve declared shared path",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			driver := newTestDriver(t, nil)
			launch := testDriverLaunch(t, "#!/bin/sh\nexit 0\n")
			launch.Declaration.SharedHostPath = tc.shared(t)

			_, err := driver.Start(ctx, launch)
			require.ErrorContains(t, err, tc.contains)
			require.NoDirExists(t, driver.stateRoot)
		})
	}
}

func TestScriptDriver_RejectsSymlinkedSharedPath(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	// The declared shared path is a symlink whose target holds the state
	// root, so a lexical comparison sees two unrelated directories.
	target := t.TempDir()
	linkDir := t.TempDir()
	shared := filepath.Join(linkDir, "project")
	require.NoError(t, os.Symlink(target, shared))

	driver := newTestDriver(t, func(cfg *ScriptDriverConfig) {
		cfg.StateRoot = filepath.Join(target, "subagentexec")
	})
	launch := testDriverLaunch(t, "#!/bin/sh\nexit 0\n")
	launch.Declaration.SharedHostPath = shared

	_, err := driver.Start(ctx, launch)
	require.ErrorContains(t, err, "is inside the declared shared path")
	// The rejection happens before anything is created, so not even an
	// empty state directory is left inside the shared tree.
	require.NoDirExists(t, driver.stateRoot)
	requireNoTokenOnDisk(t, target, linkDir)
}

func TestScriptDriver_RejectsSymlinkedStateRootAncestor(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	// An ancestor of the state root is a symlink into the shared tree, so
	// the private state would really be created inside it.
	shared := t.TempDir()
	base := t.TempDir()
	link := filepath.Join(base, "link")
	require.NoError(t, os.Symlink(shared, link))

	driver := newTestDriver(t, func(cfg *ScriptDriverConfig) {
		cfg.StateRoot = filepath.Join(link, "state", "subagentexec")
	})
	launch := testDriverLaunch(t, "#!/bin/sh\nexit 0\n")
	launch.Declaration.SharedHostPath = shared

	_, err := driver.Start(ctx, launch)
	require.ErrorContains(t, err, "is inside the declared shared path")
	require.NoDirExists(t, filepath.Join(shared, "state"))
	requireNoTokenOnDisk(t, shared, base)
}

func TestScriptDriver_LaunchesWithDisjointSymlinkedPaths(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	// Both sides are reached through symlinks, but their canonical
	// locations are disjoint, so the launch is legitimate.
	sharedTarget := t.TempDir()
	shared := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.Symlink(sharedTarget, shared))

	stateTarget := t.TempDir()
	stateLink := filepath.Join(t.TempDir(), "state")
	require.NoError(t, os.Symlink(stateTarget, stateLink))

	outDir := t.TempDir()
	driver := newTestDriver(t, func(cfg *ScriptDriverConfig) {
		cfg.StateRoot = filepath.Join(stateLink, "subagentexec")
	})
	launch := testDriverLaunch(t, readyScript(outDir))
	launch.Declaration.SharedHostPath = shared

	proc, err := driver.Start(ctx, launch)
	require.NoError(t, err)
	requireEventuallyExists(t, filepath.Join(outDir, "ready"))

	// The state lives under the canonical state root, not under the
	// symlink, and stays outside the canonical shared directory.
	paths := driver.paths(filepath.Join(stateTarget, "subagentexec"), launch.Declaration.ExecutionID)
	require.FileExists(t, paths.token)
	require.Equal(t, testAuthToken, readFileString(t, paths.token))
	require.False(t, isWithin(paths.dir, sharedTarget))

	input := readFileString(t, filepath.Join(outDir, "input-run.json"))
	var document DriverInput
	require.NoError(t, json.Unmarshal([]byte(input), &document))
	require.Equal(t, sharedTarget, document.SharedHostPath)
	require.Equal(t, paths.dir, document.StatePath)

	require.NoError(t, proc.Stop(ctx))
	require.NoDirExists(t, paths.dir)
	requireNoTokenOnDisk(t, stateTarget, sharedTarget)
}

func TestIsWithin(t *testing.T) {
	t.Parallel()

	require.True(t, isWithin("/a/b/c", "/a/b"))
	require.True(t, isWithin("/a/b", "/a/b"))
	require.True(t, isWithin("/a/b/c", "/a/b/"))
	require.False(t, isWithin("/a/bc", "/a/b"))
	require.False(t, isWithin("/a", "/a/b"))
	require.False(t, isWithin("/x/y", "/a/b"))
	require.False(t, isWithin("/a/b", ""))
}

func TestScriptDriver_RejectsLaunchWithoutToken(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	driver := newTestDriver(t, nil)
	launch := testDriverLaunch(t, "#!/bin/sh\nexit 0\n")
	launch.authToken = ""

	_, err := driver.Start(ctx, launch)
	require.ErrorContains(t, err, "no child auth token")
	require.NoDirExists(t, driverPaths(t, driver, launch.Declaration.ExecutionID).dir)
}

func TestScriptDriver_ReusesExecutionDirectoryAcrossGenerations(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	outDir := t.TempDir()
	driver := newTestDriver(t, nil)
	launch := testDriverLaunch(t, recordingScript(outDir, "exit 0"))

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

	decl := m.declaration()
	decl.Name = testDeclaredName
	decl.Driver = readyScript(outDir)
	m.reconcile(controller, decl)

	report := testutil.RequireReceive(ctx, t, controller.reportCh)
	require.Equal(t, proto.ReportSubagentExecutionStatusRequest_RUNNING, report.GetStatus())
	requireEventuallyExists(t, filepath.Join(outDir, "ready"))

	paths := driverPaths(t, driver, decl.ExecutionID)
	require.FileExists(t, paths.token)
	require.Equal(t, testAuthToken, readFileString(t, paths.token))

	statuses := m.Statuses()
	require.Len(t, statuses, 1)
	require.Equal(t, StateRunning, statuses[0].State)

	// Withdrawing the declaration stops the driver and reclaims the token.
	m.reconcile(controller)
	report = testutil.RequireReceive(ctx, t, controller.reportCh)
	require.Equal(t, proto.ReportSubagentExecutionStatusRequest_STOPPED, report.GetStatus())
	require.NoFileExists(t, paths.token)
	require.NoDirExists(t, paths.dir)
}

// TestManager_RejectedPathsLeaveNoPrivateStateOnDisk pairs the path policy
// with the concrete driver: a declaration the policy rejects never reaches
// the driver, so no state directory, protocol document, or token file is
// created for it.
func TestManager_RejectedPathsLeaveNoPrivateStateOnDisk(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	paths := newTestPaths(t)
	driver := newTestDriver(t, func(cfg *ScriptDriverConfig) {
		cfg.StateRoot = paths.state
	})
	controller := newFakeController()
	m := newManagerWithPaths(t, driver, paths)

	decl := m.declaration()
	decl.Driver = "#!/bin/sh\nexit 0\n"
	// A child path over a directory the sandbox owns is rejected, so the
	// otherwise valid declaration never launches.
	decl.SharedChildPath = "/etc/project"
	m.reconcile(controller, decl)

	report := testutil.RequireReceive(ctx, t, controller.reportCh)
	require.Equal(t, proto.ReportSubagentExecutionStatusRequest_FAILED, report.GetStatus())
	require.Contains(t, report.GetError(), reasonChildPathReserved)
	require.NotContains(t, report.GetError(), testAuthToken)

	require.NoDirExists(t, paths.state)
	requireNoTokenOnDisk(t, paths.home)
}

func TestManager_ScriptDriverReportsFailure(t *testing.T) {
	t.Parallel()

	t.Run("UnexpectedExit", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		driver := newTestDriver(t, nil)
		controller := newFakeController()
		m := newManager(t, driver)

		decl := m.declaration()
		decl.Driver = "#!/bin/sh\nif [ \"$1\" = cleanup ]; then exit 0; fi\nexit 5\n"
		m.reconcile(controller, decl)

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

		decl := m.declaration()
		decl.Driver = "not-a-script"
		m.reconcile(controller, decl)

		report := testutil.RequireReceive(ctx, t, controller.reportCh)
		require.Equal(t, proto.ReportSubagentExecutionStatusRequest_FAILED, report.GetStatus())
		require.Contains(t, report.GetError(), "shebang")
		require.NotContains(t, report.GetError(), testAuthToken)

		statuses := m.Statuses()
		require.Len(t, statuses, 1)
		require.Equal(t, StateFailed, statuses[0].State)
	})
}
