package agentcontainers

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/google/uuid"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontainers/acmock"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// TestIntegrationDocker tests agentcontainers functionality using a real
// Docker container. It starts a container with a known
// label, lists the containers, and verifies that the expected container is
// returned. It also executes a sample command inside the container.
// The container is deleted after the test is complete.
// As this test creates containers, it is skipped by default.
// It can be run manually as follows:
//
// CODER_TEST_USE_DOCKER=1 go test ./agent/agentcontainers -run TestIntegrationDocker
func TestIntegrationDocker(t *testing.T) {
	t.Parallel()
	if ctud, ok := os.LookupEnv("CODER_TEST_USE_DOCKER"); !ok || ctud != "1" {
		t.Skip("Set CODER_TEST_USE_DOCKER=1 to run this test")
	}

	pool, err := dockertest.NewPool("")
	require.NoError(t, err, "Could not connect to docker")
	testLabelValue := uuid.New().String()
	// Create a temporary directory to validate that we surface mounts correctly.
	testTempDir := t.TempDir()
	// Pick a random port to expose for testing port bindings.
	testRandPort := testutil.RandomPortNoListen(t)
	ct, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository:   "busybox",
		Tag:          "latest",
		Cmd:          []string{"sleep", "infnity"},
		Labels:       map[string]string{"com.coder.test": testLabelValue},
		Mounts:       []string{testTempDir + ":" + testTempDir},
		ExposedPorts: []string{fmt.Sprintf("%d/tcp", testRandPort)},
		PortBindings: map[docker.Port][]docker.PortBinding{
			docker.Port(fmt.Sprintf("%d/tcp", testRandPort)): {
				{
					HostIP:   "0.0.0.0",
					HostPort: strconv.FormatInt(int64(testRandPort), 10),
				},
			},
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	require.NoError(t, err, "Could not start test docker container")
	t.Logf("Created container %q", ct.Container.Name)
	t.Cleanup(func() {
		assert.NoError(t, pool.Purge(ct), "Could not purge resource %q", ct.Container.Name)
		t.Logf("Purged container %q", ct.Container.Name)
	})

	dcl := NewDocker(agentexec.DefaultExecer)
	wex := WrapDockerExec(ct.Container.Name, "")
	wexpty := WrapDockerExecPTY(ct.Container.Name, "")
	ctx := testutil.Context(t, testutil.WaitShort)
	actual, err := dcl.List(ctx)
	require.NoError(t, err, "Could not list containers")
	require.Empty(t, actual.Warnings, "Expected no warnings")
	var found bool
	for _, foundContainer := range actual.Containers {
		if foundContainer.ID == ct.Container.ID {
			found = true
			assert.Equal(t, ct.Container.Created, foundContainer.CreatedAt)
			// ory/dockertest pre-pends a forward slash to the container name.
			assert.Equal(t, strings.TrimPrefix(ct.Container.Name, "/"), foundContainer.FriendlyName)
			// ory/dockertest returns the sha256 digest of the image.
			assert.Equal(t, "busybox:latest", foundContainer.Image)
			assert.Equal(t, ct.Container.Config.Labels, foundContainer.Labels)
			assert.True(t, foundContainer.Running)
			assert.Equal(t, "running", foundContainer.Status)
			if assert.Len(t, foundContainer.Ports, 1) {
				assert.Equal(t, testRandPort, foundContainer.Ports[0].Port)
				assert.Equal(t, "tcp", foundContainer.Ports[0].Network)
			}
			if assert.Len(t, foundContainer.Volumes, 1) {
				assert.Equal(t, testTempDir, foundContainer.Volumes[testTempDir])
			}
			// Test command execution
			wrappedCmd, wrappedArgs := wex("cat", "/etc/hostname")
			cmd := agentexec.DefaultExecer.CommandContext(ctx, wrappedCmd, wrappedArgs...)
			out, err := cmd.CombinedOutput()
			if !assert.NoError(t, err) {
				t.Logf("Container %q exited with error: %v", ct.Container.ID, err)
				t.Logf("Output:\n%s", string(out))
				t.FailNow()
			}
			require.Equal(t, ct.Container.Config.Hostname, strings.TrimSpace(string(out)))

			// Test command execution with PTY
			ptyWrappedCmd, ptyWrappedArgs := wexpty("/bin/sh", "--norc")
			ptyCmd, ptyPs, err := pty.Start(agentexec.DefaultExecer.PTYCommandContext(ctx, ptyWrappedCmd, ptyWrappedArgs...))
			require.NoError(t, err, "failed to start pty command")
			t.Cleanup(func() {
				_ = ptyPs.Kill()
				_ = ptyCmd.Close()
			})
			tr := testutil.NewTerminalReader(t, ptyCmd.OutputReader())
			matchPrompt := func(line string) bool {
				return strings.HasPrefix(strings.TrimSpace(line), "/ #")
			}
			matchHostnameCmd := func(line string) bool {
				return strings.Contains(strings.TrimSpace(line), "hostname")
			}
			matchHostnameOuput := func(line string) bool {
				return strings.Contains(strings.TrimSpace(line), ct.Container.Config.Hostname)
			}
			require.NoError(t, tr.ReadUntil(ctx, matchPrompt), "failed to match prompt")
			t.Logf("Matched prompt")
			_, err = ptyCmd.InputWriter().Write([]byte("hostname\r\n"))
			require.NoError(t, err, "failed to write to pty")
			t.Logf("Wrote hostname command")
			require.NoError(t, tr.ReadUntil(ctx, matchHostnameCmd), "failed to match hostname command")
			t.Logf("Matched hostname command")
			require.NoError(t, tr.ReadUntil(ctx, matchHostnameOuput), "failed to match hostname output")
			t.Logf("Matched hostname output")
			break
		}
	}
	assert.True(t, found, "Expected to find container with label 'com.coder.test=%s'", testLabelValue)
}

func TestWrapDockerExec(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		wrapFn  WrapFn
		cmdArgs []string
		wantCmd []string
	}{
		{
			name:    "cmd with no args",
			wrapFn:  WrapDockerExec("my-container", "my-user"),
			cmdArgs: []string{"my-cmd"},
			wantCmd: []string{"docker", "exec", "--interactive", "--user", "my-user", "my-container", "my-cmd"},
		},
		{
			name:    "cmd with args",
			wrapFn:  WrapDockerExec("my-container", "my-user"),
			cmdArgs: []string{"my-cmd", "arg1", "--arg2", "arg3", "--arg4"},
			wantCmd: []string{"docker", "exec", "--interactive", "--user", "my-user", "my-container", "my-cmd", "arg1", "--arg2", "arg3", "--arg4"},
		},
		{
			name:    "no user specified",
			wrapFn:  WrapDockerExec("my-container", ""),
			cmdArgs: []string{"my-cmd"},
			wantCmd: []string{"docker", "exec", "--interactive", "my-container", "my-cmd"},
		},
		{
			name:    "tty cmd with no args",
			wrapFn:  WrapDockerExecPTY("my-container", "my-user"),
			cmdArgs: []string{"my-cmd"},
			wantCmd: []string{"docker", "exec", "--interactive", "--tty", "--user", "my-user", "my-container", "my-cmd"},
		},
		{
			name:    "cmd with args",
			wrapFn:  WrapDockerExecPTY("my-container", "my-user"),
			cmdArgs: []string{"my-cmd", "arg1", "--arg2", "arg3", "--arg4"},
			wantCmd: []string{"docker", "exec", "--interactive", "--tty", "--user", "my-user", "my-container", "my-cmd", "arg1", "--arg2", "arg3", "--arg4"},
		},
		{
			name:    "no user specified",
			wrapFn:  WrapDockerExecPTY("my-container", ""),
			cmdArgs: []string{"my-cmd"},
			wantCmd: []string{"docker", "exec", "--interactive", "--tty", "my-container", "my-cmd"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			actualCmd, actualArgs := tt.wrapFn(tt.cmdArgs[0], tt.cmdArgs[1:]...)
			assert.Equal(t, tt.wantCmd[0], actualCmd)
			assert.Equal(t, tt.wantCmd[1:], actualArgs)
		})
	}
}

// TestContainersHandler tests the containersHandler.getContainers method using
// a mock implementation. It specifically tests caching behavior.
func TestContainersHandler(t *testing.T) {
	t.Parallel()

	t.Run("list", func(t *testing.T) {
		t.Parallel()

		fakeCt := fakeContainer(t)
		fakeCt2 := fakeContainer(t)
		makeResponse := func(cts ...codersdk.WorkspaceAgentDevcontainer) codersdk.WorkspaceAgentListContainersResponse {
			return codersdk.WorkspaceAgentListContainersResponse{Containers: cts}
		}

		// Each test case is called multiple times to ensure idempotency
		for _, tc := range []struct {
			name string
			// data to be stored in the handler
			cacheData codersdk.WorkspaceAgentListContainersResponse
			// duration of cache
			cacheDur time.Duration
			// relative age of the cached data
			cacheAge time.Duration
			// function to set up expectations for the mock
			setupMock func(*acmock.MockLister)
			// expected result
			expected codersdk.WorkspaceAgentListContainersResponse
			// expected error
			expectedErr string
		}{
			{
				name: "no cache",
				setupMock: func(mcl *acmock.MockLister) {
					mcl.EXPECT().List(gomock.Any()).Return(makeResponse(fakeCt), nil).AnyTimes()
				},
				expected: makeResponse(fakeCt),
			},
			{
				name:      "no data",
				cacheData: makeResponse(),
				cacheAge:  2 * time.Second,
				cacheDur:  time.Second,
				setupMock: func(mcl *acmock.MockLister) {
					mcl.EXPECT().List(gomock.Any()).Return(makeResponse(fakeCt), nil).AnyTimes()
				},
				expected: makeResponse(fakeCt),
			},
			{
				name:      "cached data",
				cacheAge:  time.Second,
				cacheData: makeResponse(fakeCt),
				cacheDur:  2 * time.Second,
				expected:  makeResponse(fakeCt),
			},
			{
				name: "lister error",
				setupMock: func(mcl *acmock.MockLister) {
					mcl.EXPECT().List(gomock.Any()).Return(makeResponse(), assert.AnError).AnyTimes()
				},
				expectedErr: assert.AnError.Error(),
			},
			{
				name:      "stale cache",
				cacheAge:  2 * time.Second,
				cacheData: makeResponse(fakeCt),
				cacheDur:  time.Second,
				setupMock: func(mcl *acmock.MockLister) {
					mcl.EXPECT().List(gomock.Any()).Return(makeResponse(fakeCt2), nil).AnyTimes()
				},
				expected: makeResponse(fakeCt2),
			},
		} {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				var (
					ctx        = testutil.Context(t, testutil.WaitShort)
					clk        = quartz.NewMock(t)
					ctrl       = gomock.NewController(t)
					mockLister = acmock.NewMockLister(ctrl)
					now        = time.Now().UTC()
					ch         = devcontainersHandler{
						cacheDuration: tc.cacheDur,
						cl:            mockLister,
						clock:         clk,
						containers:    &tc.cacheData,
						lockCh:        make(chan struct{}, 1),
					}
				)
				if tc.cacheAge != 0 {
					ch.mtime = now.Add(-tc.cacheAge)
				}
				if tc.setupMock != nil {
					tc.setupMock(mockLister)
				}

				clk.Set(now).MustWait(ctx)

				// Repeat the test to ensure idempotency
				for i := 0; i < 2; i++ {
					actual, err := ch.getContainers(ctx)
					if tc.expectedErr != "" {
						require.Empty(t, actual, "expected no data (attempt %d)", i)
						require.ErrorContains(t, err, tc.expectedErr, "expected error (attempt %d)", i)
					} else {
						require.NoError(t, err, "expected no error (attempt %d)", i)
						require.Equal(t, tc.expected, actual, "expected containers to be equal (attempt %d)", i)
					}
				}
			})
		}
	})
}

func TestConvertDockerPort(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		in            string
		expectPort    uint16
		expectNetwork string
		expectError   string
	}{
		{
			name:        "empty port",
			in:          "",
			expectError: "invalid port",
		},
		{
			name:          "valid tcp port",
			in:            "8080/tcp",
			expectPort:    8080,
			expectNetwork: "tcp",
		},
		{
			name:          "valid udp port",
			in:            "8080/udp",
			expectPort:    8080,
			expectNetwork: "udp",
		},
		{
			name:          "valid port no network",
			in:            "8080",
			expectPort:    8080,
			expectNetwork: "tcp",
		},
		{
			name:        "invalid port",
			in:          "invalid/tcp",
			expectError: "invalid port",
		},
		{
			name:        "invalid port no network",
			in:          "invalid",
			expectError: "invalid port",
		},
		{
			name:        "multiple network",
			in:          "8080/tcp/udp",
			expectError: "invalid port",
		},
	} {
		tc := tc // not needed anymore but makes the linter happy
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			actualPort, actualNetwork, actualErr := convertDockerPort(tc.in)
			if tc.expectError != "" {
				assert.Zero(t, actualPort, "expected no port")
				assert.Empty(t, actualNetwork, "expected no network")
				assert.ErrorContains(t, actualErr, tc.expectError)
			} else {
				assert.NoError(t, actualErr, "expected no error")
				assert.Equal(t, tc.expectPort, actualPort, "expected port to match")
				assert.Equal(t, tc.expectNetwork, actualNetwork, "expected network to match")
			}
		})
	}
}

func TestConvertDockerVolume(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name                string
		in                  string
		expectHostPath      string
		expectContainerPath string
		expectError         string
	}{
		{
			name:        "empty volume",
			in:          "",
			expectError: "invalid volume",
		},
		{
			name:                "length 1 volume",
			in:                  "/path/to/something",
			expectHostPath:      "/path/to/something",
			expectContainerPath: "/path/to/something",
		},
		{
			name:                "length 2 volume",
			in:                  "/path/to/something=/path/to/something/else",
			expectHostPath:      "/path/to/something",
			expectContainerPath: "/path/to/something/else",
		},
		{
			name:        "invalid length volume",
			in:          "/path/to/something=/path/to/something/else=/path/to/something/else/else",
			expectError: "invalid volume",
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
		})
	}
}

func fakeContainer(t *testing.T, mut ...func(*codersdk.WorkspaceAgentDevcontainer)) codersdk.WorkspaceAgentDevcontainer {
	t.Helper()
	ct := codersdk.WorkspaceAgentDevcontainer{
		CreatedAt:    time.Now().UTC(),
		ID:           uuid.New().String(),
		FriendlyName: testutil.GetRandomName(t),
		Image:        testutil.GetRandomName(t) + ":" + strings.Split(uuid.New().String(), "-")[0],
		Labels: map[string]string{
			testutil.GetRandomName(t): testutil.GetRandomName(t),
		},
		Running: true,
		Ports: []codersdk.WorkspaceAgentListeningPort{
			{
				Network: "tcp",
				Port:    testutil.RandomPortNoListen(t),
			},
		},
		Status:  testutil.MustRandString(t, 10),
		Volumes: map[string]string{testutil.GetRandomName(t): testutil.GetRandomName(t)},
	}
	for _, m := range mut {
		m(&ct)
	}
	return ct
}
