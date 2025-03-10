package agentcontainers

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/google/go-cmp/cmp"
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
// CODER_TEST_USE_DOCKER=1 go test ./agent/agentcontainers -run TestDockerCLIContainerLister
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
	testRandHostPort, testRandContainerPort := testutil.RandomPortNoListen(t), testutil.RandomPortNoListen(t)
	ct, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "busybox",
		Tag:        "latest",
		Cmd:        []string{"sleep", "infnity"},
		Labels: map[string]string{
			"com.coder.test":        testLabelValue,
			"devcontainer.metadata": `[{"remoteEnv": {"FOO": "bar", "MULTILINE": "foo\nbar\nbaz"}}]`,
		},
		Mounts:       []string{testTempDir + ":" + testTempDir},
		ExposedPorts: []string{fmt.Sprintf("%d/tcp", testRandContainerPort)},
		PortBindings: map[docker.Port][]docker.PortBinding{
			docker.Port(fmt.Sprintf("%d/tcp", testRandContainerPort)): {
				{
					HostIP:   "0.0.0.0",
					HostPort: strconv.FormatInt(int64(testRandHostPort), 10),
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
	// Wait for container to start
	require.Eventually(t, func() bool {
		ct, ok := pool.ContainerByName(ct.Container.Name)
		return ok && ct.Container.State.Running
	}, testutil.WaitShort, testutil.IntervalSlow, "Container did not start in time")

	dcl := NewDocker(agentexec.DefaultExecer)
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
				assert.Equal(t, testRandHostPort, foundContainer.Ports[0].Port)
				assert.Equal(t, "tcp", foundContainer.Ports[0].Network)
			}
			if assert.Len(t, foundContainer.Volumes, 1) {
				assert.Equal(t, testTempDir, foundContainer.Volumes[testTempDir])
			}
			// Test that EnvInfo is able to correctly modify a command to be
			// executed inside the container.
			dei, err := EnvInfo(ctx, agentexec.DefaultExecer, ct.Container.ID, "")
			require.NoError(t, err, "Expected no error from DockerEnvInfo()")
			ptyWrappedCmd, ptyWrappedArgs := dei.ModifyCommand("/bin/sh", "--norc")
			ptyCmd, ptyPs, err := pty.Start(agentexec.DefaultExecer.PTYCommandContext(ctx, ptyWrappedCmd, ptyWrappedArgs...))
			require.NoError(t, err, "failed to start pty command")
			t.Cleanup(func() {
				_ = ptyPs.Kill()
				_ = ptyCmd.Close()
			})
			tr := testutil.NewTerminalReader(t, ptyCmd.OutputReader())
			matchPrompt := func(line string) bool {
				return strings.Contains(line, "#")
			}
			matchHostnameCmd := func(line string) bool {
				return strings.Contains(strings.TrimSpace(line), "hostname")
			}
			matchHostnameOuput := func(line string) bool {
				return strings.Contains(strings.TrimSpace(line), ct.Container.Config.Hostname)
			}
			matchEnvCmd := func(line string) bool {
				return strings.Contains(strings.TrimSpace(line), "env")
			}
			matchEnvOutput := func(line string) bool {
				return strings.Contains(line, "FOO=bar") || strings.Contains(line, "MULTILINE=foo")
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
			_, err = ptyCmd.InputWriter().Write([]byte("env\r\n"))
			require.NoError(t, err, "failed to write to pty")
			t.Logf("Wrote env command")
			require.NoError(t, tr.ReadUntil(ctx, matchEnvCmd), "failed to match env command")
			t.Logf("Matched env command")
			require.NoError(t, tr.ReadUntil(ctx, matchEnvOutput), "failed to match env output")
			t.Logf("Matched env output")
			break
		}
	}
	assert.True(t, found, "Expected to find container with label 'com.coder.test=%s'", testLabelValue)
}

func TestWrapDockerExec(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		containerUser string
		cmdArgs       []string
		wantCmd       []string
	}{
		{
			name:          "cmd with no args",
			containerUser: "my-user",
			cmdArgs:       []string{"my-cmd"},
			wantCmd:       []string{"docker", "exec", "--interactive", "--user", "my-user", "my-container", "my-cmd"},
		},
		{
			name:          "cmd with args",
			containerUser: "my-user",
			cmdArgs:       []string{"my-cmd", "arg1", "--arg2", "arg3", "--arg4"},
			wantCmd:       []string{"docker", "exec", "--interactive", "--user", "my-user", "my-container", "my-cmd", "arg1", "--arg2", "arg3", "--arg4"},
		},
		{
			name:          "no user specified",
			containerUser: "",
			cmdArgs:       []string{"my-cmd"},
			wantCmd:       []string{"docker", "exec", "--interactive", "my-container", "my-cmd"},
		},
	}
	for _, tt := range tests {
		tt := tt // appease the linter even though this isn't needed anymore
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			actualCmd, actualArgs := wrapDockerExec("my-container", tt.containerUser, tt.cmdArgs[0], tt.cmdArgs[1:]...)
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

func TestDockerPortBinding(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		networkPorts  map[string][]dockerPortBinding
		expectedPorts []codersdk.WorkspaceAgentListeningPort
		expectedWarns int
	}{
		{
			name:          "nil",
			networkPorts:  nil,
			expectedPorts: []codersdk.WorkspaceAgentListeningPort{},
		},
		{
			name: "simple port binding",
			networkPorts: map[string][]dockerPortBinding{
				"8080/tcp": {
					{HostIP: "0.0.0.0", HostPort: "9090"},
				},
			},
			expectedPorts: []codersdk.WorkspaceAgentListeningPort{
				{Network: "tcp", Port: 9090},
			},
		},
		{
			name: "multiple port bindings",
			networkPorts: map[string][]dockerPortBinding{
				"8080/tcp": {
					{HostIP: "0.0.0.0", HostPort: "9090"},
				},
				"8081/tcp": {
					{HostIP: "0.0.0.0", HostPort: "9091"},
				},
			},
			expectedPorts: []codersdk.WorkspaceAgentListeningPort{
				{Network: "tcp", Port: 9090},
				{Network: "tcp", Port: 9091},
			},
		},
		{
			name: "duplicate host ports on different interfaces",
			networkPorts: map[string][]dockerPortBinding{
				"8080/tcp": {
					{HostIP: "0.0.0.0", HostPort: "9090"},
					{HostIP: "127.0.0.1", HostPort: "9090"},
				},
			},
			expectedPorts: []codersdk.WorkspaceAgentListeningPort{
				{Network: "tcp", Port: 9090},
			},
		},
		{
			name: "udp protocol",
			networkPorts: map[string][]dockerPortBinding{
				"8080/udp": {
					{HostIP: "0.0.0.0", HostPort: "9090"},
				},
			},
			expectedPorts: []codersdk.WorkspaceAgentListeningPort{
				{Network: "udp", Port: 9090},
			},
		},
		{
			name: "no protocol defaults to tcp",
			networkPorts: map[string][]dockerPortBinding{
				"8080": {
					{HostIP: "0.0.0.0", HostPort: "9090"},
				},
			},
			expectedPorts: []codersdk.WorkspaceAgentListeningPort{
				{Network: "tcp", Port: 9090},
			},
		},
		{
			name: "no bindings should not create ports",
			networkPorts: map[string][]dockerPortBinding{
				"8080/tcp": {},
			},
			expectedPorts: []codersdk.WorkspaceAgentListeningPort{},
		},
		{
			name: "invalid host port",
			networkPorts: map[string][]dockerPortBinding{
				"8080/tcp": {
					{HostIP: "0.0.0.0", HostPort: "invalid"},
				},
			},
			expectedPorts: []codersdk.WorkspaceAgentListeningPort{},
			expectedWarns: 1,
		},
		{
			name: "mix of valid and invalid ports",
			networkPorts: map[string][]dockerPortBinding{
				"8080/tcp": {
					{HostIP: "0.0.0.0", HostPort: "9090"},
				},
				"8081/tcp": {
					{HostIP: "0.0.0.0", HostPort: "invalid"},
				},
			},
			expectedPorts: []codersdk.WorkspaceAgentListeningPort{
				{Network: "tcp", Port: 9090},
			},
			expectedWarns: 1,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dockerData := dockerInspect{
				ID:      "test-container",
				Created: time.Now(),
				Config: dockerInspectConfig{
					Image:  "test-image",
					Labels: map[string]string{"test": "value"},
				},
				Name:  "test-container",
				State: dockerInspectState{Running: true},
				NetworkSettings: dockerNetworkSettings{
					Ports: tc.networkPorts,
				},
			}

			container, warns := convertDockerInspect(dockerData)
			if diff := cmp.Diff(tc.expectedPorts, container.Ports); diff != "" {
				assert.Failf(t, "port mismatch", "(-want +got):\n%s", diff)
			}
			assert.Len(t, warns, tc.expectedWarns, "wrong number of warnings")
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

// TestDockerEnvInfoer tests the ability of EnvInfo to extract information from
// running containers. Containers are deleted after the test is complete.
// As this test creates containers, it is skipped by default.
// It can be run manually as follows:
//
// CODER_TEST_USE_DOCKER=1 go test ./agent/agentcontainers -run TestDockerEnvInfoer
func TestDockerEnvInfoer(t *testing.T) {
	t.Parallel()
	if ctud, ok := os.LookupEnv("CODER_TEST_USE_DOCKER"); !ok || ctud != "1" {
		t.Skip("Set CODER_TEST_USE_DOCKER=1 to run this test")
	}

	pool, err := dockertest.NewPool("")
	require.NoError(t, err, "Could not connect to docker")
	// nolint:paralleltest // variable recapture no longer required
	for idx, tt := range []struct {
		image             string
		labels            map[string]string
		expectedEnv       []string
		containerUser     string
		expectedUsername  string
		expectedUserShell string
	}{
		{
			image:  "busybox:latest",
			labels: map[string]string{`devcontainer.metadata`: `[{"remoteEnv": {"FOO": "bar", "MULTILINE": "foo\nbar\nbaz"}}]`},

			expectedEnv:       []string{"FOO=bar", "MULTILINE=foo\nbar\nbaz"},
			expectedUsername:  "root",
			expectedUserShell: "/bin/sh",
		},
		{
			image:             "busybox:latest",
			labels:            map[string]string{`devcontainer.metadata`: `[{"remoteEnv": {"FOO": "bar", "MULTILINE": "foo\nbar\nbaz"}}]`},
			expectedEnv:       []string{"FOO=bar", "MULTILINE=foo\nbar\nbaz"},
			containerUser:     "root",
			expectedUsername:  "root",
			expectedUserShell: "/bin/sh",
		},
		{
			image:             "codercom/enterprise-minimal:ubuntu",
			labels:            map[string]string{`devcontainer.metadata`: `[{"remoteEnv": {"FOO": "bar", "MULTILINE": "foo\nbar\nbaz"}}]`},
			expectedEnv:       []string{"FOO=bar", "MULTILINE=foo\nbar\nbaz"},
			expectedUsername:  "coder",
			expectedUserShell: "/bin/bash",
		},
		{
			image:             "codercom/enterprise-minimal:ubuntu",
			labels:            map[string]string{`devcontainer.metadata`: `[{"remoteEnv": {"FOO": "bar", "MULTILINE": "foo\nbar\nbaz"}}]`},
			expectedEnv:       []string{"FOO=bar", "MULTILINE=foo\nbar\nbaz"},
			containerUser:     "coder",
			expectedUsername:  "coder",
			expectedUserShell: "/bin/bash",
		},
		{
			image:             "codercom/enterprise-minimal:ubuntu",
			labels:            map[string]string{`devcontainer.metadata`: `[{"remoteEnv": {"FOO": "bar", "MULTILINE": "foo\nbar\nbaz"}}]`},
			expectedEnv:       []string{"FOO=bar", "MULTILINE=foo\nbar\nbaz"},
			containerUser:     "root",
			expectedUsername:  "root",
			expectedUserShell: "/bin/bash",
		},
		{
			image:             "codercom/enterprise-minimal:ubuntu",
			labels:            map[string]string{`devcontainer.metadata`: `[{"remoteEnv": {"FOO": "bar"}},{"remoteEnv": {"MULTILINE": "foo\nbar\nbaz"}}]`},
			expectedEnv:       []string{"FOO=bar", "MULTILINE=foo\nbar\nbaz"},
			containerUser:     "root",
			expectedUsername:  "root",
			expectedUserShell: "/bin/bash",
		},
	} {
		t.Run(fmt.Sprintf("#%d", idx), func(t *testing.T) {
			t.Parallel()

			// Start a container with the given image
			// and environment variables
			image := strings.Split(tt.image, ":")[0]
			tag := strings.Split(tt.image, ":")[1]
			ct, err := pool.RunWithOptions(&dockertest.RunOptions{
				Repository: image,
				Tag:        tag,
				Cmd:        []string{"sleep", "infinity"},
				Labels:     tt.labels,
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

			ctx := testutil.Context(t, testutil.WaitShort)
			dei, err := EnvInfo(ctx, agentexec.DefaultExecer, ct.Container.ID, tt.containerUser)
			require.NoError(t, err, "Expected no error from DockerEnvInfo()")

			u, err := dei.User()
			require.NoError(t, err, "Expected no error from CurrentUser()")
			require.Equal(t, tt.expectedUsername, u.Username, "Expected username to match")

			hd, err := dei.HomeDir()
			require.NoError(t, err, "Expected no error from UserHomeDir()")
			require.NotEmpty(t, hd, "Expected user homedir to be non-empty")

			sh, err := dei.Shell(tt.containerUser)
			require.NoError(t, err, "Expected no error from UserShell()")
			require.Equal(t, tt.expectedUserShell, sh, "Expected user shell to match")

			// We don't need to test the actual environment variables here.
			environ := dei.Environ()
			require.NotEmpty(t, environ, "Expected environ to be non-empty")

			// Test that the environment variables are present in modified command
			// output.
			envCmd, envArgs := dei.ModifyCommand("env")
			for _, env := range tt.expectedEnv {
				require.Subset(t, envArgs, []string{"--env", env})
			}
			// Run the command in the container and check the output
			// HACK: we remove the --tty argument because we're not running in a tty
			envArgs = slices.DeleteFunc(envArgs, func(s string) bool { return s == "--tty" })
			stdout, stderr, err := run(ctx, agentexec.DefaultExecer, envCmd, envArgs...)
			require.Empty(t, stderr, "Expected no stderr output")
			require.NoError(t, err, "Expected no error from running command")
			for _, env := range tt.expectedEnv {
				require.Contains(t, stdout, env)
			}
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
