package cli

import (
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

			perShard := make([]int, count)
			for _, id := range ids {
				idx := workspaceShardIndex(id, count)

				// In range.
				require.GreaterOrEqual(t, idx, int64(0))
				require.Less(t, idx, count)

				// Deterministic: same inputs, same output.
				require.Equal(t, idx, workspaceShardIndex(id, count))

				perShard[idx]++
			}

			// Disjoint and complete: every ID counted exactly once across shards.
			total := 0
			for _, n := range perShard {
				total += n
			}
			require.Equal(t, len(ids), total)
		})
	}
}

// TestWorkspaceShardIndexStable asserts that removing workspaces from the set
// does not change the shard any surviving workspace maps to (the property that
// makes replicas tolerant of a churning running set).
func TestWorkspaceShardIndexStable(t *testing.T) {
	t.Parallel()

	const count = 12
	ids := make([]uuid.UUID, 500)
	for i := range ids {
		ids[i] = uuid.New()
	}

	// Baseline assignment for every ID.
	want := make(map[uuid.UUID]int64, len(ids))
	for _, id := range ids {
		want[id] = workspaceShardIndex(id, count)
	}

	// Drop half the workspaces; the survivors must map to the same shard.
	for i, id := range ids {
		if i%2 == 0 {
			continue // pretend this workspace disappeared
		}
		require.Equal(t, want[id], workspaceShardIndex(id, count),
			"assignment must not depend on which other workspaces are present")
	}
}
