package agentcontainers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentcontainers"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty"
	"github.com/coder/coder/v2/testutil"
)

func TestDevcontainerCLI_ArgsAndParsing(t *testing.T) {
	t.Parallel()

	testExePath, err := os.Executable()
	require.NoError(t, err, "get test executable path")

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	t.Run("Up", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name            string
			logFile         string
			workspace       string
			config          string
			opts            []agentcontainers.DevcontainerCLIUpOptions
			wantArgs        string
			wantError       bool
			wantContainerID bool // If true, expect a container ID even when wantError is true.
		}{
			{
				name:            "success",
				logFile:         "up.log",
				workspace:       "/test/workspace",
				wantArgs:        "up --log-format json --workspace-folder /test/workspace",
				wantError:       false,
				wantContainerID: true,
			},
			{
				name:            "success with config",
				logFile:         "up.log",
				workspace:       "/test/workspace",
				config:          "/test/config.json",
				wantArgs:        "up --log-format json --workspace-folder /test/workspace --config /test/config.json",
				wantError:       false,
				wantContainerID: true,
			},
			{
				name:            "already exists",
				logFile:         "up-already-exists.log",
				workspace:       "/test/workspace",
				wantArgs:        "up --log-format json --workspace-folder /test/workspace",
				wantError:       false,
				wantContainerID: true,
			},
			{
				name:            "docker error",
				logFile:         "up-error-docker.log",
				workspace:       "/test/workspace",
				wantArgs:        "up --log-format json --workspace-folder /test/workspace",
				wantError:       true,
				wantContainerID: false,
			},
			{
				name:            "bad outcome",
				logFile:         "up-error-bad-outcome.log",
				workspace:       "/test/workspace",
				wantArgs:        "up --log-format json --workspace-folder /test/workspace",
				wantError:       true,
				wantContainerID: false,
			},
			{
				name:            "does not exist",
				logFile:         "up-error-does-not-exist.log",
				workspace:       "/test/workspace",
				wantArgs:        "up --log-format json --workspace-folder /test/workspace",
				wantError:       true,
				wantContainerID: false,
			},
			{
				name:      "with remove existing container",
				logFile:   "up.log",
				workspace: "/test/workspace",
				opts: []agentcontainers.DevcontainerCLIUpOptions{
					agentcontainers.WithRemoveExistingContainer(),
				},
				wantArgs:        "up --log-format json --workspace-folder /test/workspace --remove-existing-container",
				wantError:       false,
				wantContainerID: true,
			},
			{
				// This test verifies that when a lifecycle script like
				// postCreateCommand fails, the CLI returns both an error
				// and a container ID. The caller can then proceed with
				// agent injection into the created container.
				name:            "lifecycle script failure with container",
				logFile:         "up-error-lifecycle-script.log",
				workspace:       "/test/workspace",
				wantArgs:        "up --log-format json --workspace-folder /test/workspace",
				wantError:       true,
				wantContainerID: true,
			},
			{
				name:      "with cache-from single",
				logFile:   "up.log",
				workspace: "/test/workspace",
				opts: []agentcontainers.DevcontainerCLIUpOptions{
					agentcontainers.WithCacheFrom("ghcr.io/coder/test:cache"),
				},
				wantArgs:        "up --log-format json --workspace-folder /test/workspace --cache-from ghcr.io/coder/test:cache",
				wantError:       false,
				wantContainerID: true,
			},
			{
				name:      "with cache-from multiple",
				logFile:   "up.log",
				workspace: "/test/workspace",
				opts: []agentcontainers.DevcontainerCLIUpOptions{
					agentcontainers.WithCacheFrom("ghcr.io/coder/test:cache", "ghcr.io/coder/test:latest"),
				},
				wantArgs:        "up --log-format json --workspace-folder /test/workspace --cache-from ghcr.io/coder/test:cache --cache-from ghcr.io/coder/test:latest",
				wantError:       false,
				wantContainerID: true,
			},
			{
				name:      "cache-from filters empty strings",
				logFile:   "up.log",
				workspace: "/test/workspace",
				opts: []agentcontainers.DevcontainerCLIUpOptions{
					agentcontainers.WithCacheFrom("", "ghcr.io/coder/test:latest", ""),
				},
				wantArgs:        "up --log-format json --workspace-folder /test/workspace --cache-from ghcr.io/coder/test:latest",
				wantError:       false,
				wantContainerID: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitMedium)

				testExecer := &testDevcontainerExecer{
					testExePath: testExePath,
					wantArgs:    tt.wantArgs,
					wantError:   tt.wantError,
					logFile:     filepath.Join("testdata", "devcontainercli", "parse", tt.logFile),
				}

				dccli := agentcontainers.NewDevcontainerCLI(logger, testExecer)
				containerID, err := dccli.Up(ctx, tt.workspace, tt.config, tt.opts...)
				if tt.wantError {
					assert.Error(t, err, "want error")
				} else {
					assert.NoError(t, err, "want no error")
				}
				if tt.wantContainerID {
					assert.NotEmpty(t, containerID, "expected non-empty container ID")
				} else {
					assert.Empty(t, containerID, "expected empty container ID")
				}
			})
		}
	})

	t.Run("Exec", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name            string
			workspaceFolder string
			configPath      string
			cmd             string
			cmdArgs         []string
			opts            []agentcontainers.DevcontainerCLIExecOptions
			wantArgs        string
			wantError       bool
		}{
			{
				name:            "simple command",
				workspaceFolder: "/test/workspace",
				configPath:      "",
				cmd:             "echo",
				cmdArgs:         []string{"hello"},
				wantArgs:        "exec --workspace-folder /test/workspace echo hello",
				wantError:       false,
			},
			{
				name:            "command with multiple args",
				workspaceFolder: "/test/workspace",
				configPath:      "/test/config.json",
				cmd:             "ls",
				cmdArgs:         []string{"-la", "/workspace"},
				wantArgs:        "exec --workspace-folder /test/workspace --config /test/config.json ls -la /workspace",
				wantError:       false,
			},
			{
				name:            "empty command args",
				workspaceFolder: "/test/workspace",
				configPath:      "",
				cmd:             "bash",
				cmdArgs:         nil,
				wantArgs:        "exec --workspace-folder /test/workspace bash",
				wantError:       false,
			},
			{
				name:            "workspace not found",
				workspaceFolder: "/nonexistent/workspace",
				configPath:      "",
				cmd:             "echo",
				cmdArgs:         []string{"test"},
				wantArgs:        "exec --workspace-folder /nonexistent/workspace echo test",
				wantError:       true,
			},
			{
				name:            "with container ID",
				workspaceFolder: "/test/workspace",
				configPath:      "",
				cmd:             "echo",
				cmdArgs:         []string{"hello"},
				opts:            []agentcontainers.DevcontainerCLIExecOptions{agentcontainers.WithExecContainerID("test-container-123")},
				wantArgs:        "exec --workspace-folder /test/workspace --container-id test-container-123 echo hello",
				wantError:       false,
			},
			{
				name:            "with container ID and config",
				workspaceFolder: "/test/workspace",
				configPath:      "/test/config.json",
				cmd:             "bash",
				cmdArgs:         []string{"-c", "ls -la"},
				opts:            []agentcontainers.DevcontainerCLIExecOptions{agentcontainers.WithExecContainerID("my-container")},
				wantArgs:        "exec --workspace-folder /test/workspace --config /test/config.json --container-id my-container bash -c ls -la",
				wantError:       false,
			},
			{
				name:            "with container ID and output capture",
				workspaceFolder: "/test/workspace",
				configPath:      "",
				cmd:             "cat",
				cmdArgs:         []string{"/etc/hostname"},
				opts: []agentcontainers.DevcontainerCLIExecOptions{
					agentcontainers.WithExecContainerID("test-container-789"),
				},
				wantArgs:  "exec --workspace-folder /test/workspace --container-id test-container-789 cat /etc/hostname",
				wantError: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitMedium)

				testExecer := &testDevcontainerExecer{
					testExePath: testExePath,
					wantArgs:    tt.wantArgs,
					wantError:   tt.wantError,
					logFile:     "", // Exec doesn't need log file parsing
				}

				dccli := agentcontainers.NewDevcontainerCLI(logger, testExecer)
				err := dccli.Exec(ctx, tt.workspaceFolder, tt.configPath, tt.cmd, tt.cmdArgs, tt.opts...)
				if tt.wantError {
					assert.Error(t, err, "want error")
				} else {
					assert.NoError(t, err, "want no error")
				}
			})
		}
	})

	t.Run("ReadConfig", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name            string
			logFile         string
			workspaceFolder string
			configPath      string
			opts            []agentcontainers.DevcontainerCLIReadConfigOptions
			wantArgs        string
			wantError       bool
			wantConfig      agentcontainers.DevcontainerConfig
		}{
			{
				name:            "WithCoderCustomization",
				logFile:         "read-config-with-coder-customization.log",
				workspaceFolder: "/test/workspace",
				configPath:      "",
				wantArgs:        "read-configuration --include-merged-configuration --workspace-folder /test/workspace",
				wantError:       false,
				wantConfig: agentcontainers.DevcontainerConfig{
					MergedConfiguration: agentcontainers.DevcontainerMergedConfiguration{
						Customizations: agentcontainers.DevcontainerMergedCustomizations{
							Coder: []agentcontainers.CoderCustomization{
								{
									DisplayApps: map[codersdk.DisplayApp]bool{
										codersdk.DisplayAppVSCodeDesktop: true,
										codersdk.DisplayAppWebTerminal:   true,
									},
								},
								{
									DisplayApps: map[codersdk.DisplayApp]bool{
										codersdk.DisplayAppVSCodeInsiders: true,
										codersdk.DisplayAppWebTerminal:    false,
									},
								},
							},
						},
					},
				},
			},
			{
				name:            "WithoutCoderCustomization",
				logFile:         "read-config-without-coder-customization.log",
				workspaceFolder: "/test/workspace",
				configPath:      "/test/config.json",
				wantArgs:        "read-configuration --include-merged-configuration --workspace-folder /test/workspace --config /test/config.json",
				wantError:       false,
				wantConfig: agentcontainers.DevcontainerConfig{
					MergedConfiguration: agentcontainers.DevcontainerMergedConfiguration{
						Customizations: agentcontainers.DevcontainerMergedCustomizations{
							Coder: nil,
						},
					},
				},
			},
			{
				name:            "FileNotFound",
				logFile:         "read-config-error-not-found.log",
				workspaceFolder: "/nonexistent/workspace",
				configPath:      "",
				wantArgs:        "read-configuration --include-merged-configuration --workspace-folder /nonexistent/workspace",
				wantError:       true,
				wantConfig:      agentcontainers.DevcontainerConfig{},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitMedium)

				testExecer := &testDevcontainerExecer{
					testExePath: testExePath,
					wantArgs:    tt.wantArgs,
					wantError:   tt.wantError,
					logFile:     filepath.Join("testdata", "devcontainercli", "readconfig", tt.logFile),
				}

				dccli := agentcontainers.NewDevcontainerCLI(logger, testExecer)
				config, err := dccli.ReadConfig(ctx, tt.workspaceFolder, tt.configPath, []string{}, tt.opts...)
				if tt.wantError {
					assert.Error(t, err, "want error")
					assert.Equal(t, agentcontainers.DevcontainerConfig{}, config, "expected empty config on error")
				} else {
					assert.NoError(t, err, "want no error")
					assert.Equal(t, tt.wantConfig, config, "expected config to match")
				}
			})
		}
	})
}

// TestDevcontainerCLI_WithOutput tests that WithUpOutput and WithExecOutput capture CLI
// logs to provided writers.
func TestDevcontainerCLI_WithOutput(t *testing.T) {
	t.Parallel()

	// Prepare test executable and logger.
	testExePath, err := os.Executable()
	require.NoError(t, err, "get test executable path")

	t.Run("Up", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip("Windows uses CRLF line endings, golden file is LF")
		}

		// Buffers to capture stdout and stderr.
		outBuf := &bytes.Buffer{}
		errBuf := &bytes.Buffer{}

		// Simulate CLI execution with a standard up.log file.
		wantArgs := "up --log-format json --workspace-folder /test/workspace"
		testExecer := &testDevcontainerExecer{
			testExePath: testExePath,
			wantArgs:    wantArgs,
			wantError:   false,
			logFile:     filepath.Join("testdata", "devcontainercli", "parse", "up.log"),
		}
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		dccli := agentcontainers.NewDevcontainerCLI(logger, testExecer)

		// Call Up with WithUpOutput to capture CLI logs.
		ctx := testutil.Context(t, testutil.WaitMedium)
		containerID, err := dccli.Up(ctx, "/test/workspace", "", agentcontainers.WithUpOutput(outBuf, errBuf))
		require.NoError(t, err, "Up should succeed")
		require.NotEmpty(t, containerID, "expected non-empty container ID")

		// Read expected log content.
		expLog, err := os.ReadFile(filepath.Join("testdata", "devcontainercli", "parse", "up.golden"))
		require.NoError(t, err, "reading expected log file")

		// Verify stdout buffer contains the CLI logs and stderr is empty.
		assert.Equal(t, string(expLog), outBuf.String(), "stdout buffer should match CLI logs")
		assert.Empty(t, errBuf.String(), "stderr buffer should be empty on success")
	})

	t.Run("Exec", func(t *testing.T) {
		t.Parallel()

		logFile := filepath.Join(t.TempDir(), "exec.log")
		f, err := os.Create(logFile)
		require.NoError(t, err, "create exec log file")
		_, err = f.WriteString("exec command log\n")
		require.NoError(t, err, "write to exec log file")
		err = f.Close()
		require.NoError(t, err, "close exec log file")

		// Buffers to capture stdout and stderr.
		outBuf := &bytes.Buffer{}
		errBuf := &bytes.Buffer{}

		// Simulate CLI execution for exec command with container ID.
		wantArgs := "exec --workspace-folder /test/workspace --container-id test-container-456 echo hello"
		testExecer := &testDevcontainerExecer{
			testExePath: testExePath,
			wantArgs:    wantArgs,
			wantError:   false,
			logFile:     logFile,
		}
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		dccli := agentcontainers.NewDevcontainerCLI(logger, testExecer)

		// Call Exec with WithExecOutput and WithContainerID to capture any command output.
		ctx := testutil.Context(t, testutil.WaitMedium)
		err = dccli.Exec(ctx, "/test/workspace", "", "echo", []string{"hello"},
			agentcontainers.WithExecContainerID("test-container-456"),
			agentcontainers.WithExecOutput(outBuf, errBuf),
		)
		require.NoError(t, err, "Exec should succeed")

		assert.NotEmpty(t, outBuf.String(), "stdout buffer should not be empty for exec with log file")
		assert.Empty(t, errBuf.String(), "stderr buffer should be empty")
	})
}

// testDevcontainerExecer implements the agentexec.Execer interface for testing.
type testDevcontainerExecer struct {
	testExePath string
	wantArgs    string
	wantError   bool
	logFile     string
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
	// Set this environment variable so the child process knows it's the helper.
	cmd.Env = append(os.Environ(),
		"TEST_DEVCONTAINER_WANT_HELPER_PROCESS=1",
		"TEST_DEVCONTAINER_WANT_ARGS="+e.wantArgs,
		"TEST_DEVCONTAINER_WANT_ERROR="+fmt.Sprintf("%v", e.wantError),
		"TEST_DEVCONTAINER_LOG_FILE="+e.logFile,
	)

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
	if logFilePath != "" {
		// Read and output log file for commands that need it (like "up")
		output, err := os.ReadFile(logFilePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Reading log file %s failed: %v\n", logFilePath, err)
			os.Exit(2)
		}
		_, _ = io.Copy(os.Stdout, bytes.NewReader(output))
	}

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
	if _, err := exec.LookPath("devcontainer"); err != nil {
		t.Fatal("this test requires the devcontainer CLI: npm install -g @devcontainers/cli")
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
	"runArgs": ["--label=com.coder.test=devcontainercli", "--label=` + agentcontainers.DevcontainerIsTestRunLabel + `=true"]
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

func TestDevcontainerFeatures_OptionsAsEnvs(t *testing.T) {
	t.Parallel()

	realConfigJSON := `{
		"mergedConfiguration": {
			"features": {
				"./code-server": {
					"port": 9090
				},
				"ghcr.io/devcontainers/features/docker-in-docker:2": {
					"moby": "false"
				}
			}
		}
	}`
	var realConfig agentcontainers.DevcontainerConfig
	err := json.Unmarshal([]byte(realConfigJSON), &realConfig)
	require.NoError(t, err, "unmarshal JSON payload")

	tests := []struct {
		name     string
		features agentcontainers.DevcontainerFeatures
		want     []string
	}{
		{
			name: "code-server feature",
			features: agentcontainers.DevcontainerFeatures{
				"./code-server": map[string]any{
					"port": 9090,
				},
			},
			want: []string{
				"FEATURE_CODE_SERVER_OPTION_PORT=9090",
			},
		},
		{
			name: "docker-in-docker feature",
			features: agentcontainers.DevcontainerFeatures{
				"ghcr.io/devcontainers/features/docker-in-docker:2": map[string]any{
					"moby": "false",
				},
			},
			want: []string{
				"FEATURE_DOCKER_IN_DOCKER_OPTION_MOBY=false",
			},
		},
		{
			name: "multiple features with multiple options",
			features: agentcontainers.DevcontainerFeatures{
				"./code-server": map[string]any{
					"port":     9090,
					"password": "secret",
				},
				"ghcr.io/devcontainers/features/docker-in-docker:2": map[string]any{
					"moby":                        "false",
					"docker-dash-compose-version": "v2",
				},
			},
			want: []string{
				"FEATURE_CODE_SERVER_OPTION_PASSWORD=secret",
				"FEATURE_CODE_SERVER_OPTION_PORT=9090",
				"FEATURE_DOCKER_IN_DOCKER_OPTION_DOCKER_DASH_COMPOSE_VERSION=v2",
				"FEATURE_DOCKER_IN_DOCKER_OPTION_MOBY=false",
			},
		},
		{
			name: "feature with non-map value (should be ignored)",
			features: agentcontainers.DevcontainerFeatures{
				"./code-server": map[string]any{
					"port": 9090,
				},
				"./invalid-feature": "not-a-map",
			},
			want: []string{
				"FEATURE_CODE_SERVER_OPTION_PORT=9090",
			},
		},
		{
			name:     "real config example",
			features: realConfig.MergedConfiguration.Features,
			want: []string{
				"FEATURE_CODE_SERVER_OPTION_PORT=9090",
				"FEATURE_DOCKER_IN_DOCKER_OPTION_MOBY=false",
			},
		},
		{
			name:     "empty features",
			features: agentcontainers.DevcontainerFeatures{},
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.features.OptionsAsEnvs()
			if diff := cmp.Diff(tt.want, got); diff != "" {
				require.Failf(t, "OptionsAsEnvs() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
