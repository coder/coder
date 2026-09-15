package cli

import (
	"fmt"
	"testing"

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

// TestShardBounds verifies that partitioning n items across count shards yields
// disjoint slices that cover [0, n) exactly, with sizes differing by at most
// one (relatively even distribution).
func TestShardBounds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		n     int
		count int
	}{
		{n: 0, count: 1},
		{n: 1, count: 1},
		{n: 10, count: 1},
		{n: 10, count: 3},
		{n: 10, count: 10},
		{n: 6667, count: 27}, // 20k scenario, per region.
		{n: 200, count: 7},
		{n: 3, count: 5}, // more shards than items: some shards are empty.
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("n=%d,count=%d", tc.n, tc.count), func(t *testing.T) {
			t.Parallel()

			var (
				covered  int
				prevEnd  int
				minSize  = -1
				maxSize  int
				baseSize = tc.n / tc.count
			)
			for i := 0; i < tc.count; i++ {
				start, end := shardBounds(tc.n, i, tc.count)

				require.LessOrEqual(t, start, end, "start must not exceed end")
				require.GreaterOrEqual(t, start, 0)
				require.LessOrEqual(t, end, tc.n)
				// Shards must be contiguous: this shard starts where the
				// previous one ended.
				require.Equal(t, prevEnd, start, "shards must be contiguous and non-overlapping")
				prevEnd = end

				size := end - start
				covered += size
				if minSize == -1 || size < minSize {
					minSize = size
				}
				if size > maxSize {
					maxSize = size
				}
			}

			// The union of shards covers every item exactly once.
			require.Equal(t, tc.n, covered, "shards must cover all items")
			require.Equal(t, tc.n, prevEnd, "last shard must end at n")
			// Sizes differ by at most one and straddle the floor size.
			require.LessOrEqual(t, maxSize-minSize, 1, "shard sizes must differ by at most one")
			require.GreaterOrEqual(t, maxSize, baseSize)
		})
	}
}
