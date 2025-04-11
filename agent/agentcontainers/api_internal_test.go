package agentcontainers

import (
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentcontainers/acmock"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestAPI(t *testing.T) {
	t.Parallel()

	// List tests the API.getContainers method using a mock
	// implementation. It specifically tests caching behavior.
	t.Run("List", func(t *testing.T) {
		t.Parallel()

		fakeCt := fakeContainer(t)
		fakeCt2 := fakeContainer(t)
		makeResponse := func(cts ...codersdk.WorkspaceAgentContainer) codersdk.WorkspaceAgentListContainersResponse {
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
					logger     = slogtest.Make(t, nil).Leveled(slog.LevelDebug)
					api        = NewAPI(logger, WithLister(mockLister))
				)
				api.cacheDuration = tc.cacheDur
				api.clock = clk
				api.containers = tc.cacheData
				if tc.cacheAge != 0 {
					api.mtime = now.Add(-tc.cacheAge)
				}
				if tc.setupMock != nil {
					tc.setupMock(mockLister)
				}

				clk.Set(now).MustWait(ctx)

				// Repeat the test to ensure idempotency
				for i := 0; i < 2; i++ {
					actual, err := api.getContainers(ctx)
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
		Ports: []codersdk.WorkspaceAgentContainerPort{
			{
				Network:  "tcp",
				Port:     testutil.RandomPortNoListen(t),
				HostPort: testutil.RandomPortNoListen(t),
				//nolint:gosec // this is a test
				HostIP: []string{"127.0.0.1", "[::1]", "localhost", "0.0.0.0", "[::]", testutil.GetRandomName(t)}[rand.Intn(6)],
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
