package agentcontainers_test

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentcontainers"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/pty"
	"github.com/coder/coder/v2/testutil"
)

func TestDevcontainerCLI_ArgsAndParsing(t *testing.T) {
	t.Parallel()

	testExePath, err := os.Executable()
	require.NoError(t, err, "get test executable path")

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	//nolint:paralleltest // This test is not parallel-safe due to t.Setenv.
	t.Run("Up", func(t *testing.T) {
		tests := []struct {
			name      string
			logFile   string
			workspace string
			config    string
			opts      []agentcontainers.DevcontainerCLIUpOptions
			wantArgs  string
			wantError bool
		}{
			{
				name:      "success",
				logFile:   "up.log",
				workspace: "/test/workspace",
				wantArgs:  "up --log-format json --workspace-folder /test/workspace",
				wantError: false,
			},
			{
				name:      "success with config",
				logFile:   "up.log",
				workspace: "/test/workspace",
				config:    "/test/config.json",
				wantArgs:  "up --log-format json --workspace-folder /test/workspace --config /test/config.json",
				wantError: false,
			},
			{
				name:      "already exists",
				logFile:   "up-already-exists.log",
				workspace: "/test/workspace",
				wantArgs:  "up --log-format json --workspace-folder /test/workspace",
				wantError: false,
			},
			{
				name:      "docker error",
				logFile:   "up-error-docker.log",
				workspace: "/test/workspace",
				wantArgs:  "up --log-format json --workspace-folder /test/workspace",
				wantError: true,
			},
			{
				name:      "bad outcome",
				logFile:   "up-error-bad-outcome.log",
				workspace: "/test/workspace",
				wantArgs:  "up --log-format json --workspace-folder /test/workspace",
				wantError: true,
			},
			{
				name:      "does not exist",
				logFile:   "up-error-does-not-exist.log",
				workspace: "/test/workspace",
				wantArgs:  "up --log-format json --workspace-folder /test/workspace",
				wantError: true,
			},
			{
				name:      "with remove existing container",
				logFile:   "up.log",
				workspace: "/test/workspace",
				opts: []agentcontainers.DevcontainerCLIUpOptions{
					agentcontainers.WithRemoveExistingContainer(),
				},
				wantArgs:  "up --log-format json --workspace-folder /test/workspace --remove-existing-container",
				wantError: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitMedium)

				testExecer := &testDevcontainerExecer{
					testExePath: testExePath,
					extraEnv: []string{
						"TEST_DEVCONTAINER_WANT_ARGS=" + tt.wantArgs,
						"TEST_DEVCONTAINER_WANT_ERROR=" + fmt.Sprintf("%v", tt.wantError),
						"TEST_DEVCONTAINER_LOG_FILE=" + filepath.Join("testdata", "devcontainercli", "parse", tt.logFile),
					},
				}

				dccli := agentcontainers.NewDevcontainerCLI(logger, testExecer)
				containerID, err := dccli.Up(ctx, tt.workspace, tt.config, tt.opts...)
				if tt.wantError {
					assert.Error(t, err, "want error")
					assert.Empty(t, containerID, "expected empty container ID")
				} else {
					assert.NoError(t, err, "want no error")
					assert.NotEmpty(t, containerID, "expected non-empty container ID")
				}
			})
		}
	})
}

// testDevcontainerExecer implements the agentexec.Execer interface for testing.
type testDevcontainerExecer struct {
	testExePath string
	extraEnv    []string
}

// CommandContext returns a test binary command that simulates devcontainer responses.
func (e *testDevcontainerExecer) CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	// Only handle "devcontainer" commands.
	if name != "devcontainer" {
		// For non-devcontainer commands, use a standard execer.
		return agentexec.DefaultExecer.CommandContext(ctx, name, args...)
	}

	// Create a command that runs the test binary with special flags
	// that tell it to simulate a devcontainer command.
	testArgs := []string{
		"-test.run=TestDevcontainerHelperProcess",
		"--",
		name,
	}
	testArgs = append(testArgs, args...)

	//nolint:gosec // This is a test binary, so we don't need to worry about command injection.
	cmd := exec.CommandContext(ctx, e.testExePath, testArgs...)
	cmd.Env = append(os.Environ(), e.extraEnv...)
	// Set this environment va[riable so the child process knows it's the helper.
	cmd.Env = append(cmd.Env, "TEST_DEVCONTAINER_WANT_HELPER_PROCESS=1")

	return cmd
}

// PTYCommandContext returns a PTY command.
func (*testDevcontainerExecer) PTYCommandContext(_ context.Context, name string, args ...string) *pty.Cmd {
	// This method shouldn't be called for our devcontainer tests.
	panic("PTYCommandContext not expected in devcontainer tests")
}

// This is a special test helper that is executed as a subprocess.
// It simulates the behavior of the devcontainer CLI.
//
//nolint:revive,paralleltest // This is a test helper function.
func TestDevcontainerHelperProcess(t *testing.T) {
	// If not called by the test as a helper process, do nothing.
	if os.Getenv("TEST_DEVCONTAINER_WANT_HELPER_PROCESS") != "1" {
		return
	}

	helperArgs := flag.Args()
	if len(helperArgs) < 1 {
		fmt.Fprintf(os.Stderr, "No command\n")
		os.Exit(2)
	}

	if helperArgs[0] != "devcontainer" {
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", helperArgs[0])
		os.Exit(2)
	}

	// Verify arguments against expected arguments and skip
	// "devcontainer", it's not included in the input args.
	wantArgs := os.Getenv("TEST_DEVCONTAINER_WANT_ARGS")
	gotArgs := strings.Join(helperArgs[1:], " ")
	if gotArgs != wantArgs {
		fmt.Fprintf(os.Stderr, "Arguments don't match.\nWant: %q\nGot:  %q\n",
			wantArgs, gotArgs)
		os.Exit(2)
	}

	logFilePath := os.Getenv("TEST_DEVCONTAINER_LOG_FILE")
	output, err := os.ReadFile(logFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Reading log file %s failed: %v\n", logFilePath, err)
		os.Exit(2)
	}

	_, _ = io.Copy(os.Stdout, bytes.NewReader(output))
	if os.Getenv("TEST_DEVCONTAINER_WANT_ERROR") == "true" {
		os.Exit(1)
	}
	os.Exit(0)
}

// TestDockerDevcontainerCLI tests the DevcontainerCLI component with real Docker containers.
// This test verifies that containers can be created and recreated using the actual
// devcontainer CLI and Docker. It is skipped by default and can be run with:
//
//	CODER_TEST_USE_DOCKER=1 go test ./agent/agentcontainers -run TestDockerDevcontainerCLI
//
// The test requires Docker to be installed and running.
func TestDockerDevcontainerCLI(t *testing.T) {
	t.Parallel()
	if os.Getenv("CODER_TEST_USE_DOCKER") != "1" {
		t.Skip("skipping Docker test; set CODER_TEST_USE_DOCKER=1 to run")
	}

	// Connect to Docker.
	pool, err := dockertest.NewPool("")
	require.NoError(t, err, "connect to Docker")

	t.Run("ContainerLifecycle", func(t *testing.T) {
		t.Parallel()

		// Set up workspace directory with a devcontainer configuration.
		workspaceFolder := t.TempDir()
		configPath := setupDevcontainerWorkspace(t, workspaceFolder)

		// Use a long timeout because container operations are slow.
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

		// Create the devcontainer CLI under test.
		dccli := agentcontainers.NewDevcontainerCLI(logger, agentexec.DefaultExecer)

		// Create a container.
		firstID, err := dccli.Up(ctx, workspaceFolder, configPath)
		require.NoError(t, err, "create container")
		require.NotEmpty(t, firstID, "container ID should not be empty")
		defer removeDevcontainerByID(t, pool, firstID)

		// Verify container exists.
		firstContainer, found := findDevcontainerByID(t, pool, firstID)
		require.True(t, found, "container should exist")

		// Remember the container creation time.
		firstCreated := firstContainer.Created

		// Recreate the container.
		secondID, err := dccli.Up(ctx, workspaceFolder, configPath, agentcontainers.WithRemoveExistingContainer())
		require.NoError(t, err, "recreate container")
		require.NotEmpty(t, secondID, "recreated container ID should not be empty")
		defer removeDevcontainerByID(t, pool, secondID)

		// Verify the new container exists and is different.
		secondContainer, found := findDevcontainerByID(t, pool, secondID)
		require.True(t, found, "recreated container should exist")

		// Verify it's a different container by checking creation time.
		secondCreated := secondContainer.Created
		assert.NotEqual(t, firstCreated, secondCreated, "recreated container should have different creation time")

		// Verify the first container is removed by the recreation.
		_, found = findDevcontainerByID(t, pool, firstID)
		assert.False(t, found, "first container should be removed")
	})
}

// setupDevcontainerWorkspace prepares a test environment with a minimal
// devcontainer.json configuration and returns the path to the config file.
func setupDevcontainerWorkspace(t *testing.T, workspaceFolder string) string {
	t.Helper()

	// Create the devcontainer directory structure.
	devcontainerDir := filepath.Join(workspaceFolder, ".devcontainer")
	err := os.MkdirAll(devcontainerDir, 0o755)
	require.NoError(t, err, "create .devcontainer directory")

	// Write a minimal configuration with test labels for identification.
	configPath := filepath.Join(devcontainerDir, "devcontainer.json")
	content := `{
	"image": "alpine:latest",
	"containerEnv": {
		"TEST_CONTAINER": "true"
	},
	"runArgs": ["--label", "com.coder.test=devcontainercli"]
}`
	err = os.WriteFile(configPath, []byte(content), 0o600)
	require.NoError(t, err, "create devcontainer.json file")

	return configPath
}

// findDevcontainerByID locates a container by its ID and verifies it has our
// test label. Returns the container and whether it was found.
func findDevcontainerByID(t *testing.T, pool *dockertest.Pool, id string) (*docker.Container, bool) {
	t.Helper()

	container, err := pool.Client.InspectContainer(id)
	if err != nil {
		t.Logf("Inspect container failed: %v", err)
		return nil, false
	}
	require.Equal(t, "devcontainercli", container.Config.Labels["com.coder.test"], "sanity check failed: container should have the test label")

	return container, true
}

// removeDevcontainerByID safely cleans up a test container by ID, verifying
// it has our test label before removal to prevent accidental deletion.
func removeDevcontainerByID(t *testing.T, pool *dockertest.Pool, id string) {
	t.Helper()

	errNoSuchContainer := &docker.NoSuchContainer{}

	// Check if the container has the expected label.
	container, err := pool.Client.InspectContainer(id)
	if err != nil {
		if errors.As(err, &errNoSuchContainer) {
			t.Logf("Container %s not found, skipping removal", id)
			return
		}
		require.NoError(t, err, "inspect container")
	}
	require.Equal(t, "devcontainercli", container.Config.Labels["com.coder.test"], "sanity check failed: container should have the test label")

	t.Logf("Removing container with ID: %s", id)
	err = pool.Client.RemoveContainer(docker.RemoveContainerOptions{
		ID:            container.ID,
		Force:         true,
		RemoveVolumes: true,
	})
	if err != nil && !errors.As(err, &errNoSuchContainer) {
		assert.NoError(t, err, "remove container failed")
	}
}
