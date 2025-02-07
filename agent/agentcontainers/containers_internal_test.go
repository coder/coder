package agentcontainers

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/agent/agentcontainers/acmock"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// TestDockerCLIContainerLister tests the happy path of the
// dockerCLIContainerLister.List method. It starts a container with a known
// label, lists the containers, and verifies that the expected container is
// returned. The container is deleted after the test is complete.
func TestDockerCLIContainerLister(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("creating containers on non-linux runners is slow and flaky")
	}

	// Conditionally skip if Docker is not available.
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not found in PATH")
	}

	pool, err := dockertest.NewPool("")
	require.NoError(t, err, "Could not connect to docker")
	testLabelValue := uuid.New().String()
	ct, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "busybox",
		Tag:        "latest",
		Cmd:        []string{"sleep", "infnity"},
		Labels:     map[string]string{"com.coder.test": testLabelValue},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	require.NoError(t, err, "Could not start test docker container")
	t.Cleanup(func() {
		assert.NoError(t, pool.Purge(ct), "Could not purge resource %q", ct.Container.Name)
	})

	dcl := DockerCLILister{}
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
			assert.Len(t, foundContainer.Ports, 0)
			assert.Len(t, foundContainer.Volumes, 0)
			break
		}
	}
	assert.True(t, found, "Expected to find container with label 'com.coder.test=%s'", testLabelValue)
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
