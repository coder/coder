package agent

import (
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
		assert.NoError(t, pool.Purge(ct), "Could not purge resource")
	})

	dcl := dockerCLIContainerLister{}
	ctx := testutil.Context(t, testutil.WaitShort)
	actual, err := dcl.List(ctx)
	require.NoError(t, err, "Could not list containers")
	var found bool
	for _, foundContainer := range actual {
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

		// Each test case is called multiple times to ensure idempotency
		for _, tc := range []struct {
			name string
			// data to be stored in the handler
			cacheData []codersdk.WorkspaceAgentContainer
			// duration of cache
			cacheDur time.Duration
			// relative age of the cached data
			cacheAge time.Duration
			// function to set up expectations for the mock
			setupMock func(*MockContainerLister)
			// expected result
			expected []codersdk.WorkspaceAgentContainer
			// expected error
			expectedErr string
		}{
			{
				name: "no cache",
				setupMock: func(mcl *MockContainerLister) {
					mcl.EXPECT().List(gomock.Any()).Return([]codersdk.WorkspaceAgentContainer{fakeCt}, nil).AnyTimes()
				},
				expected: []codersdk.WorkspaceAgentContainer{fakeCt},
			},
			{
				name:      "no data",
				cacheData: nil,
				cacheAge:  2 * time.Second,
				cacheDur:  time.Second,
				setupMock: func(mcl *MockContainerLister) {
					mcl.EXPECT().List(gomock.Any()).Return([]codersdk.WorkspaceAgentContainer{fakeCt}, nil).AnyTimes()
				},
				expected: []codersdk.WorkspaceAgentContainer{fakeCt},
			},
			{
				name:      "cached data",
				cacheAge:  time.Second,
				cacheData: []codersdk.WorkspaceAgentContainer{fakeCt},
				cacheDur:  2 * time.Second,
				expected:  []codersdk.WorkspaceAgentContainer{fakeCt},
			},
			{
				name: "lister error",
				setupMock: func(mcl *MockContainerLister) {
					mcl.EXPECT().List(gomock.Any()).Return(nil, assert.AnError).AnyTimes()
				},
				expectedErr: assert.AnError.Error(),
			},
			{
				name:      "stale cache",
				cacheAge:  2 * time.Second,
				cacheData: []codersdk.WorkspaceAgentContainer{fakeCt},
				cacheDur:  time.Second,
				setupMock: func(mcl *MockContainerLister) {
					mcl.EXPECT().List(gomock.Any()).Return([]codersdk.WorkspaceAgentContainer{fakeCt2}, nil).AnyTimes()
				},
				expected: []codersdk.WorkspaceAgentContainer{fakeCt2},
			},
		} {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				var (
					ctx        = testutil.Context(t, testutil.WaitShort)
					clk        = quartz.NewMock(t)
					ctrl       = gomock.NewController(t)
					mockLister = NewMockContainerLister(ctrl)
					now        = time.Now().UTC()
					ch         = containersHandler{
						cacheDuration: tc.cacheDur,
						cl:            mockLister,
						clock:         clk,
						containers:    tc.cacheData,
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

func fakeContainer(t *testing.T, mut ...func(*codersdk.WorkspaceAgentContainer)) codersdk.WorkspaceAgentContainer {
	t.Helper()
	ct := codersdk.WorkspaceAgentContainer{
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
