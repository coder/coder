package cli

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/coder/v2/scaletest/workspaceupdates"
)

// TestBuildWorkspaceUpdatesConfigs covers the per-user config wiring, in
// particular the reuse-user assignment and the powerUserCount+i offset between
// the power-user and regular-user groups, without standing up a server.
func TestBuildWorkspaceUpdatesConfigs(t *testing.T) {
	t.Parallel()

	baseParams := func() workspaceUpdatesConfigParams {
		return workspaceUpdatesConfigParams{
			organizationID:            uuid.New(),
			templateID:                uuid.New(),
			powerUserCount:            2,
			powerUserWorkspaces:       5,
			regularUserCount:          3,
			regularUserWorkspaceCount: 1,
			workspaceUpdatesTimeout:   time.Minute,
			dialTimeout:               time.Minute,
			metrics:                   workspaceupdates.NewMetrics(prometheus.NewRegistry()),
			dialBarrier:               &sync.WaitGroup{},
		}
	}

	makeReuse := func(n int) []loadtestutil.ReuseUser {
		reuse := make([]loadtestutil.ReuseUser, 0, n)
		for i := range n {
			reuse = append(reuse, loadtestutil.ReuseUser{
				User: codersdk.User{
					ReducedUser: codersdk.ReducedUser{
						MinimalUser: codersdk.MinimalUser{ID: uuid.New()},
					},
				},
				SessionToken: fmt.Sprintf("token-%d", i),
			})
		}
		return reuse
	}

	t.Run("Reuse", func(t *testing.T) {
		t.Parallel()

		params := baseParams()
		params.reuseUsers = true
		total := params.powerUserCount + params.regularUserCount
		reuse := makeReuse(int(total))

		configs, err := buildWorkspaceUpdatesConfigs(params, reuse)
		require.NoError(t, err)
		require.Len(t, configs, int(total))

		// Power users come first, each owning powerUserWorkspaces, assigned
		// reuse[0..powerUserCount).
		for i := range params.powerUserCount {
			require.Equal(t, params.powerUserWorkspaces, configs[i].WorkspaceCount)
			require.Equal(t, reuse[i].SessionToken, configs[i].SessionToken)
			require.Equal(t, reuse[i].User.ID, configs[i].PreCreatedUser.ID)
		}
		// Regular users follow, each owning regularUserWorkspaceCount, assigned
		// reuse[powerUserCount..total): the offset the review flagged.
		for i := range params.regularUserCount {
			cfg := configs[params.powerUserCount+i]
			require.Equal(t, int64(params.regularUserWorkspaceCount), cfg.WorkspaceCount)
			require.Equal(t, reuse[params.powerUserCount+i].SessionToken, cfg.SessionToken)
			require.Equal(t, reuse[params.powerUserCount+i].User.ID, cfg.PreCreatedUser.ID)
		}

		// Every runner gets a distinct token; a wrong offset would reuse one.
		seen := make(map[string]struct{}, len(configs))
		for _, cfg := range configs {
			require.NotEmpty(t, cfg.SessionToken)
			_, dup := seen[cfg.SessionToken]
			require.Falsef(t, dup, "token %q assigned to more than one runner", cfg.SessionToken)
			seen[cfg.SessionToken] = struct{}{}
		}
	})

	t.Run("NoReuse", func(t *testing.T) {
		t.Parallel()

		params := baseParams()
		configs, err := buildWorkspaceUpdatesConfigs(params, nil)
		require.NoError(t, err)
		require.Len(t, configs, int(params.powerUserCount+params.regularUserCount))
		for _, cfg := range configs {
			require.Empty(t, cfg.SessionToken)
			require.Equal(t, uuid.Nil, cfg.PreCreatedUser.ID)
		}
	})

	t.Run("ReuseWrongCount", func(t *testing.T) {
		t.Parallel()

		params := baseParams()
		params.reuseUsers = true
		_, err := buildWorkspaceUpdatesConfigs(params, makeReuse(4))
		require.ErrorContains(t, err, "expected 5 reuse users, got 4")
	})
}
