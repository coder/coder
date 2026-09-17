package cli

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/serpent"
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

// TestShardingFlagsDefaults checks that attach installs the unset sentinels, so
// requested() can tell an unset flag from a real shard 0.
func TestShardingFlagsDefaults(t *testing.T) {
	t.Parallel()

	s := &shardingFlags{}
	var opts serpent.OptionSet
	s.attach(&opts)
	require.NoError(t, opts.SetDefaults())

	require.Equal(t, int64(-1), s.index)
	require.Equal(t, int64(0), s.count)
	require.False(t, s.requested())
}

// TestShardingFlagsValidate exercises validate() directly, no coderd client.
// Unset flags use the sentinels attach installs (index -1, count 0).
func TestShardingFlagsValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		index            int64
		count            int64
		targetWorkspaces string
		errText          string // empty means the input is valid
	}{
		{name: "Unset", index: -1, count: 0},
		{name: "Valid", index: 2, count: 4},
		{
			name: "TargetAndShardMutuallyExclusive", index: -1, count: 2,
			targetWorkspaces: "0:10",
			errText:          "--target-workspaces cannot be used with --shard-index/--shard-count",
		},
		{
			name: "ShardIndexRequiresShardCount", index: 1, count: 0,
			errText: "--shard-index requires --shard-count",
		},
		{
			// index 0 is a valid shard, not "unset": a lone --shard-index=0 must
			// still require --shard-count rather than silently target all.
			name: "ShardIndexZeroRequiresShardCount", index: 0, count: 0,
			errText: "--shard-index requires --shard-count",
		},
		{
			name: "ShardCountRequiresShardIndex", index: -1, count: 2,
			errText: "--shard-count requires --shard-index",
		},
		{
			// A negative count is "set" (not the 0 sentinel) and must error rather
			// than silently disable sharding.
			name: "NegativeShardCount", index: 0, count: -1,
			errText: "--shard-count must be a positive integer, got -1",
		},
		{
			name: "ShardIndexOutOfRange", index: 3, count: 3,
			errText: "--shard-index 3 is out of range for --shard-count 3",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := &shardingFlags{index: tc.index, count: tc.count}
			err := s.validate(tc.targetWorkspaces)
			if tc.errText == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.errText)
		})
	}

	t.Run("NilReceiver", func(t *testing.T) {
		t.Parallel()

		// chat passes a nil *shardingFlags to mean "no sharding".
		var s *shardingFlags
		require.False(t, s.requested())
		require.NoError(t, s.validate("0:10"))
	})
}

// TestWorkspaceShardIndex checks the hash assignment is deterministic and in
// range; determinism is also what keeps it stable as the running set churns.
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

// TestShardWorkspaces checks that shardWorkspaces excludes non-running
// workspaces and that the shards are disjoint and cover the whole running set.
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

// TestSelectShard covers selectShard's empty-set handling and diagnostic on an
// already-fetched list (no coderd client).
func TestSelectShard(t *testing.T) {
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

		s := &shardingFlags{index: 1, count: 4}
		var buf bytes.Buffer
		got, err := s.selectShard(workspaces, &buf)
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

		s := &shardingFlags{index: 0, count: 3}
		var buf bytes.Buffer
		got, err := s.selectShard(nonRunning(10), &buf)
		require.ErrorContains(t, err, "no running scaletest workspaces exist")
		require.Nil(t, got)
		require.Empty(t, buf.String())
	})

	// An over-provisioned pod (more shards than running workspaces map to it) must
	// exit 0 with a diagnostic, not error: this is the empty-but-running case the
	// per-shard diagnostic was added for. A regression that errored here would
	// silently fail every over-provisioned pod's Indexed Job.
	t.Run("EmptyShardExitsZero", func(t *testing.T) {
		t.Parallel()

		const shardCount = 8
		workspaces := make([]codersdk.Workspace, 0, 3)
		occupied := make(map[int64]struct{})
		for range 3 {
			ws := codersdk.Workspace{ID: uuid.New()}
			ws.LatestBuild.Status = codersdk.WorkspaceStatusRunning
			workspaces = append(workspaces, ws)
			occupied[workspaceShardIndex(ws.ID, shardCount)] = struct{}{}
		}

		// With 3 running workspaces across 8 shards at least one shard is empty.
		var emptyIndex int64 = -1
		for idx := range int64(shardCount) {
			if _, ok := occupied[idx]; !ok {
				emptyIndex = idx
				break
			}
		}
		require.GreaterOrEqual(t, emptyIndex, int64(0), "expected an empty shard")

		s := &shardingFlags{index: emptyIndex, count: shardCount}
		var buf bytes.Buffer
		got, err := s.selectShard(workspaces, &buf)
		require.NoError(t, err)
		require.Empty(t, got)
		require.Equal(t,
			fmt.Sprintf("shard %d of %d: targeting 0 of 3 running workspaces\n", emptyIndex, shardCount),
			buf.String())
	})
}
