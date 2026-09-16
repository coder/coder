package cli

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
)

// TestFilterScaletestUsersByPrefix covers the pure user-selection logic behind
// the notifications scaletest user reuse: an infix pool must not pick up users
// from the default pool (isolation), non-scaletest users are ignored, and the
// username guard rejects users that only match the prefix in another field.
func TestFilterScaletestUsersByPrefix(t *testing.T) {
	t.Parallel()

	users := []codersdk.User{
		scaletestUser("scaletest-notif-aaaaaaaa-0", "aaaaaaaa-0@scaletest.local"),
		scaletestUser("scaletest-notif-bbbbbbbb-1", "bbbbbbbb-1@scaletest.local"),
		// Default pool: a scaletest user that is NOT in the notif pool.
		scaletestUser("scaletest-cccccccc-0", "cccccccc-0@scaletest.local"),
		// Not a scaletest user at all.
		scaletestUser("regular-user", "regular@example.com"),
		// Scaletest email but a username that does not start with the prefix; the
		// guard must reject it even though a search could surface it.
		scaletestUser("admin", "scaletest-notif-dddddddd-9@scaletest.local"),
	}

	cases := []struct {
		name   string
		prefix string
		want   []string
	}{
		{
			name:   "infix isolates its own pool",
			prefix: "scaletest-notif-",
			want: []string{
				"scaletest-notif-aaaaaaaa-0",
				"scaletest-notif-bbbbbbbb-1",
			},
		},
		{
			name:   "default prefix selects all scaletest users",
			prefix: loadtestutil.ScaleTestPrefix + "-",
			want: []string{
				"scaletest-notif-aaaaaaaa-0",
				"scaletest-notif-bbbbbbbb-1",
				"scaletest-cccccccc-0",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := filterScaletestUsersByPrefix(users, tc.prefix)

			gotNames := make([]string, 0, len(got))
			for _, u := range got {
				gotNames = append(gotNames, u.Username)
			}
			require.ElementsMatch(t, tc.want, gotNames)
		})
	}
}

func scaletestUser(username, email string) codersdk.User {
	return codersdk.User{
		ReducedUser: codersdk.ReducedUser{
			MinimalUser: codersdk.MinimalUser{Username: username},
			Email:       email,
		},
	}
}

// TestWorkspaceShardIndex verifies that hash-based shard assignment is
// deterministic, in range, disjoint-and-complete over a set, and stable when
// the set changes (a workspace's shard depends only on its ID and the shard
// count, not on the other workspaces present).
func TestWorkspaceShardIndex(t *testing.T) {
	t.Parallel()

	counts := []int64{1, 2, 3, 7, 27}

	// A fixed pool of workspace IDs.
	ids := make([]uuid.UUID, 1000)
	for i := range ids {
		ids[i] = uuid.New()
	}

	for _, count := range counts {
		t.Run(fmt.Sprintf("count=%d", count), func(t *testing.T) {
			t.Parallel()

			for _, id := range ids {
				idx := workspaceShardIndex(id, count)

				// In range [0, count).
				require.GreaterOrEqual(t, idx, int64(0))
				require.Less(t, idx, count)

				// Deterministic: same inputs, same output. This is also what makes
				// assignment stable as the running set churns, since the result
				// depends only on the ID and the count, not on the other
				// workspaces present.
				require.Equal(t, idx, workspaceShardIndex(id, count))
			}
		})
	}
}

// TestShardWorkspaces exercises the running-status filter plus shard assignment
// as a unit, without a coderd client: it builds mixed-status workspaces, runs
// shardWorkspaces for every shard index, and asserts that non-running
// workspaces are excluded and the union of shards is exactly the running set
// with no overlap.
func TestShardWorkspaces(t *testing.T) {
	t.Parallel()

	statuses := []codersdk.WorkspaceStatus{
		codersdk.WorkspaceStatusRunning,
		codersdk.WorkspaceStatusStopped,
		codersdk.WorkspaceStatusFailed,
		codersdk.WorkspaceStatusStarting,
		codersdk.WorkspaceStatusPending,
	}

	workspaces := make([]codersdk.Workspace, 0, 600)
	runningIDs := make(map[uuid.UUID]struct{})
	for i := range 600 {
		status := statuses[i%len(statuses)]
		ws := codersdk.Workspace{ID: uuid.New()}
		ws.LatestBuild.Status = status
		workspaces = append(workspaces, ws)
		if status == codersdk.WorkspaceStatusRunning {
			runningIDs[ws.ID] = struct{}{}
		}
	}

	for _, shardCount := range []int64{1, 4, 27} {
		t.Run(fmt.Sprintf("count=%d", shardCount), func(t *testing.T) {
			t.Parallel()

			seen := make(map[uuid.UUID]struct{})
			for idx := range shardCount {
				shard, running := shardWorkspaces(workspaces, idx, shardCount)

				// Total running is reported consistently on every call.
				require.Equal(t, len(runningIDs), running)

				for _, ws := range shard {
					// Only running workspaces are selected.
					_, isRunning := runningIDs[ws.ID]
					require.True(t, isRunning, "non-running workspace must not be targeted")
					// This workspace really belongs to this shard.
					require.Equal(t, idx, workspaceShardIndex(ws.ID, shardCount))
					// No workspace appears in more than one shard.
					_, dup := seen[ws.ID]
					require.False(t, dup, "workspace assigned to more than one shard")
					seen[ws.ID] = struct{}{}
				}
			}

			// The union of all shards is exactly the running set.
			require.Len(t, seen, len(runningIDs))
			for id := range runningIDs {
				_, ok := seen[id]
				require.True(t, ok, "every running workspace must be covered by some shard")
			}
		})
	}
}

// TestApplyShard covers the shard wiring in getTargetedWorkspaces (empty-set
// handling and the per-shard diagnostic) without a coderd client: applyShard
// operates on an already-fetched workspace list.
func TestApplyShard(t *testing.T) {
	t.Parallel()

	nonRunning := func(n int) []codersdk.Workspace {
		ws := make([]codersdk.Workspace, n)
		for i := range ws {
			ws[i] = codersdk.Workspace{ID: uuid.New()}
			ws[i].LatestBuild.Status = codersdk.WorkspaceStatusStopped
		}
		return ws
	}

	t.Run("SelectsShardAndLogs", func(t *testing.T) {
		t.Parallel()

		workspaces := make([]codersdk.Workspace, 0, 350)
		for range 300 {
			ws := codersdk.Workspace{ID: uuid.New()}
			ws.LatestBuild.Status = codersdk.WorkspaceStatusRunning
			workspaces = append(workspaces, ws)
		}
		workspaces = append(workspaces, nonRunning(50)...)

		f := &workspaceTargetFlags{shardIndex: 1, shardCount: 4}
		var buf bytes.Buffer
		got, err := f.applyShard(workspaces, &buf)
		require.NoError(t, err)

		for _, ws := range got {
			require.Equal(t, codersdk.WorkspaceStatusRunning, ws.LatestBuild.Status)
			require.Equal(t, int64(1), workspaceShardIndex(ws.ID, 4))
		}
		require.Equal(t,
			fmt.Sprintf("shard 1 of 4: targeting %d of 300 running workspaces\n", len(got)),
			buf.String())
	})

	t.Run("NoRunningErrors", func(t *testing.T) {
		t.Parallel()

		f := &workspaceTargetFlags{shardIndex: 0, shardCount: 3}
		var buf bytes.Buffer
		got, err := f.applyShard(nonRunning(10), &buf)
		require.ErrorContains(t, err, "no running scaletest workspaces exist")
		require.Nil(t, got)
		require.Empty(t, buf.String())
	})
}
