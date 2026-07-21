package workspacestats_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/workspacestats"
)

func TestSessionCountsFromProto(t *testing.T) {
	t.Parallel()

	t.Run("MapTakesPrecedence", func(t *testing.T) {
		t.Parallel()
		//nolint:staticcheck // Deprecated fields are set to verify precedence.
		st := &agentproto.Stats{
			SessionCounts:      map[string]int64{"cursor": 2, "ssh": 1},
			SessionCountVscode: 9,
		}
		require.Equal(t, map[string]int64{"cursor": 2, "ssh": 1}, workspacestats.SessionCountsFromProto(st))
	})

	t.Run("NormalizesAndMergesNames", func(t *testing.T) {
		t.Parallel()
		st := &agentproto.Stats{
			SessionCounts: map[string]int64{"VSCode": 1, "vscode": 2, "Reconnecting-PTY": 1},
		}
		require.Equal(t, map[string]int64{
			"vscode":           3,
			"reconnecting_pty": 1,
		}, workspacestats.SessionCountsFromProto(st))
	})

	t.Run("DropsNonPositiveEntries", func(t *testing.T) {
		t.Parallel()
		st := &agentproto.Stats{
			SessionCounts: map[string]int64{"vscode": 1, "reconnecting_pty": 0, "bogus": -1},
		}
		require.Equal(t, map[string]int64{"vscode": 1}, workspacestats.SessionCountsFromProto(st))
	})

	t.Run("OldAgentFallback", func(t *testing.T) {
		t.Parallel()
		//nolint:staticcheck // Deprecated fields simulate an old agent.
		st := &agentproto.Stats{
			SessionCountVscode:    3,
			SessionCountJetbrains: 1,
			SessionCountSsh:       2,
		}
		require.Equal(t, map[string]int64{
			"vscode":    3,
			"jetbrains": 1,
			"ssh":       2,
		}, workspacestats.SessionCountsFromProto(st))
	})

	t.Run("ZeroValuedMapEntriesDropped", func(t *testing.T) {
		t.Parallel()
		// Zero-valued entries must be dropped, not stored as phantom counts.
		st := &agentproto.Stats{
			SessionCounts: map[string]int64{"ssh": 0, "reconnecting_pty": 0},
		}
		require.Empty(t, workspacestats.SessionCountsFromProto(st))
	})

	t.Run("CapsEntriesAggregatingIntoUnknown", func(t *testing.T) {
		t.Parallel()
		sessionCounts := map[string]int64{"vscode": 5}
		for i := range 200 {
			sessionCounts[fmt.Sprintf("zz-ide-%03d", i)] = 1
		}
		got := workspacestats.SessionCountsFromProto(&agentproto.Stats{SessionCounts: sessionCounts})
		// 64 named entries (well-known first, then lexicographic) plus
		// "unknown" holding the 137 overflow counts. Hyphens fold to underscores.
		require.Len(t, got, 65)
		require.EqualValues(t, 5, got["vscode"])
		require.EqualValues(t, 1, got["zz_ide_000"])
		require.EqualValues(t, 137, got["unknown"])
		require.NotContains(t, got, "zz_ide_199")
	})

	t.Run("CapKeepsWellKnownNames", func(t *testing.T) {
		t.Parallel()
		// Junk names sort before "ssh", so only the known-first
		// ordering keeps the canonical entry under the cap.
		sessionCounts := map[string]int64{"ssh": 5}
		for i := range 100 {
			sessionCounts[fmt.Sprintf("0ide-%03d", i)] = 1
		}
		got := workspacestats.SessionCountsFromProto(&agentproto.Stats{SessionCounts: sessionCounts})
		require.Len(t, got, 65)
		require.EqualValues(t, 5, got["ssh"])
		require.EqualValues(t, 37, got["unknown"])
		require.NotContains(t, got, "0ide_099")
	})

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		require.Empty(t, workspacestats.SessionCountsFromProto(&agentproto.Stats{}))
	})
}

func TestHasSessionCounts(t *testing.T) {
	t.Parallel()

	//nolint:staticcheck // Deprecated fields simulate old agents.
	cases := []struct {
		name string
		st   *agentproto.Stats
		want bool
	}{
		{"Empty", &agentproto.Stats{}, false},
		{"MapPositive", &agentproto.Stats{SessionCounts: map[string]int64{"cursor": 1}}, true},
		{"MapZeroOnly", &agentproto.Stats{SessionCounts: map[string]int64{"ssh": 0}}, false},
		{"LegacyPositive", &agentproto.Stats{SessionCountSsh: 2}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, workspacestats.HasSessionCounts(tc.st))
			// Must agree with the map-building form it replaces.
			require.Equal(t, tc.want, len(workspacestats.SessionCountsFromProto(tc.st)) > 0)
		})
	}
}

func TestClearSessionCounts(t *testing.T) {
	t.Parallel()

	//nolint:staticcheck // Deprecated fields simulate an old agent.
	st := &agentproto.Stats{
		SessionCounts:      map[string]int64{"vscode": 1},
		SessionCountVscode: 1,
		SessionCountSsh:    2,
	}
	workspacestats.ClearSessionCounts(st)
	require.Empty(t, workspacestats.SessionCountsFromProto(st))
}
