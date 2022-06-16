package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
)

func TestDurationDisplay(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		Duration string
		Expected string
	}{
		{"-1s", "<1m"},
		{"0s", "0s"},
		{"1s", "<1m"},
		{"59s", "<1m"},
		{"1m", "1m"},
		{"1m1s", "1m"},
		{"2m", "2m"},
		{"59m", "59m"},
		{"1h", "1h"},
		{"1h1m1s", "1h1m"},
		{"2h", "2h"},
		{"23h", "23h"},
		{"24h", "1d"},
		{"24h1m1s", "1d"},
		{"25h", "1d1h"},
	} {
		testCase := testCase
		t.Run(testCase.Duration, func(t *testing.T) {
			t.Parallel()
			d, err := time.ParseDuration(testCase.Duration)
			require.NoError(t, err)
			actual := durationDisplay(d)
			assert.Equal(t, testCase.Expected, actual)
		})
	}
}

func TestSign(t *testing.T) {
	t.Parallel()
	assert.Equal(t, sign(0), "+")
	assert.Equal(t, sign(1), "+")
	assert.Equal(t, sign(-1), "-")
}

func TestHasExtension(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		Name             string
		Workspace        codersdk.Workspace
		ExpectedFound    bool
		ExpectedDuration time.Duration
	}{
		{
			Name: "Stopped",
			Workspace: codersdk.Workspace{
				LatestBuild: codersdk.WorkspaceBuild{
					Transition: codersdk.WorkspaceTransitionStop,
				},
			},
			ExpectedFound:    false,
			ExpectedDuration: 0,
		},
		{
			Name: "Building",
			Workspace: codersdk.Workspace{
				LatestBuild: codersdk.WorkspaceBuild{
					Job: codersdk.ProvisionerJob{
						CompletedAt: nil,
					},
					Transition: codersdk.WorkspaceTransitionStart,
				},
			},
			ExpectedFound:    false,
			ExpectedDuration: 0,
		},
		{
			Name: "BuiltNoDeadline",
			Workspace: codersdk.Workspace{
				LatestBuild: codersdk.WorkspaceBuild{
					Deadline: time.Time{},
					Job: codersdk.ProvisionerJob{
						CompletedAt: ptr.Ref(time.Now()),
					},
					Transition: codersdk.WorkspaceTransitionStart,
				},
			},
			ExpectedFound:    false,
			ExpectedDuration: 0,
		},
		{
			Name: "BuiltNoTTL",
			Workspace: codersdk.Workspace{
				LatestBuild: codersdk.WorkspaceBuild{
					Deadline: time.Now().Add(8 * time.Hour),
					Job: codersdk.ProvisionerJob{
						CompletedAt: ptr.Ref(time.Now()),
					},
					Transition: codersdk.WorkspaceTransitionStart,
				},
				TTLMillis: nil, // explicit
			},
			ExpectedFound:    false,
			ExpectedDuration: 0,
		},
		{
			Name: "PositiveDelta",
			Workspace: codersdk.Workspace{
				LatestBuild: codersdk.WorkspaceBuild{
					Deadline: time.Now().Add(9*time.Hour + 30*time.Second),
					Job: codersdk.ProvisionerJob{
						CompletedAt: ptr.Ref(time.Now()),
					},
					Transition: codersdk.WorkspaceTransitionStart,
				},
				TTLMillis: ptr.Ref(8 * time.Hour.Milliseconds()),
			},
			ExpectedFound:    true,
			ExpectedDuration: time.Hour,
		},
		{
			Name: "NegativeDelta",
			Workspace: codersdk.Workspace{
				LatestBuild: codersdk.WorkspaceBuild{
					Deadline: time.Now().Add(7 * time.Hour),
					Job: codersdk.ProvisionerJob{
						CompletedAt: ptr.Ref(time.Now()),
					},
					Transition: codersdk.WorkspaceTransitionStart,
				},
				TTLMillis: ptr.Ref(8 * time.Hour.Milliseconds()),
			},
			ExpectedFound:    true,
			ExpectedDuration: -time.Hour,
		},
		{
			Name: "Epsilon",
			Workspace: codersdk.Workspace{
				LatestBuild: codersdk.WorkspaceBuild{
					Deadline: time.Now().Add(8 * time.Hour),
					Job: codersdk.ProvisionerJob{
						CompletedAt: ptr.Ref(time.Now()),
					},
					Transition: codersdk.WorkspaceTransitionStart,
				},
				TTLMillis: ptr.Ref(8 * time.Hour.Milliseconds()),
			},
			ExpectedFound:    false,
			ExpectedDuration: 0,
		},
	} {
		testCase := testCase
		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()
			actualFound, actualDuration := hasExtension(testCase.Workspace)
			if assert.Equal(t, testCase.ExpectedFound, actualFound) {
				assert.InDelta(t, testCase.ExpectedDuration, actualDuration, float64(time.Minute))
			}
		})
	}
}
