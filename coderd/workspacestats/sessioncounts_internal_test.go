package workspacestats

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/idemetadata"
)

// sessionCountsFromProto is what the batcher applies on ingest.
func sessionCountsFromProto(st *agentproto.Stats) map[string]int64 {
	return capSessionCounts(normalizedSessionCounts(st))
}

func TestSessionCountsFromProto(t *testing.T) {
	t.Parallel()

	//nolint:staticcheck // Deprecated fields simulate old agents.
	for _, tc := range []struct {
		name  string
		stats *agentproto.Stats
		want  map[string]int64
	}{
		{
			"Empty",
			&agentproto.Stats{},
			map[string]int64{},
		},
		{
			"NormalizesAndMergesNames",
			&agentproto.Stats{SessionCounts: map[string]int64{"VSCode": 1, "vscode": 2, "Reconnecting-PTY": 1}},
			map[string]int64{"vscode": 3, "reconnecting_pty": 1},
		},
		{
			"DropsNonPositiveEntries",
			&agentproto.Stats{SessionCounts: map[string]int64{"vscode": 1, "reconnecting_pty": 0, "bogus": -1}},
			map[string]int64{"vscode": 1},
		},
		{
			"MapTakesPrecedenceOverDeprecatedFields",
			&agentproto.Stats{SessionCounts: map[string]int64{"cursor": 2, "ssh": 1}, SessionCountVscode: 9},
			map[string]int64{"cursor": 2, "ssh": 1},
		},
		{
			"OldAgentFallback",
			&agentproto.Stats{SessionCountVscode: 3, SessionCountJetbrains: 1, SessionCountSsh: 2},
			map[string]int64{"vscode": 3, "jetbrains": 1, "ssh": 2},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, sessionCountsFromProto(tc.stats))
		})
	}
}

func TestCapSessionCounts(t *testing.T) {
	t.Parallel()

	// The well-known names carry the lowest counts and the busiest name sorts
	// last, so surviving the cap can only come from the ordering rules.
	counts := map[string]int64{idemetadata.AppNameSSH: 1, idemetadata.AppNameVSCode: 1, "zzz-busy-ide": 5}
	const overflowing = 200
	for i := range overflowing {
		counts[fmt.Sprintf("0ide-%03d", i)] = 2
	}

	got := sessionCountsFromProto(&agentproto.Stats{SessionCounts: counts})

	require.Len(t, got, idemetadata.MaxSessionCountEntries+1, "cap plus the unknown bucket")
	require.EqualValues(t, 1, got[idemetadata.AppNameSSH])
	require.EqualValues(t, 1, got[idemetadata.AppNameVSCode])
	require.EqualValues(t, 5, got["zzz_busy_ide"])
	// Those three plus 61 more fill the cap; the other 139 sum into unknown.
	require.EqualValues(t, 2, got["0ide_000"])
	require.NotContains(t, got, "0ide_199")
	require.EqualValues(t, (overflowing-61)*2, got[idemetadata.AppNameUnknown])

	var reported, stored int64
	for _, count := range counts {
		reported += count
	}
	for _, count := range got {
		stored += count
	}
	require.Equal(t, reported, stored, "capping must not lose counts")
}

func TestHasSessionCounts(t *testing.T) {
	t.Parallel()

	//nolint:staticcheck // Deprecated fields simulate old agents.
	for _, tc := range []struct {
		name  string
		stats *agentproto.Stats
		want  bool
	}{
		{"Empty", &agentproto.Stats{}, false},
		{"MapPositive", &agentproto.Stats{SessionCounts: map[string]int64{"cursor": 1}}, true},
		{"MapZeroOnly", &agentproto.Stats{SessionCounts: map[string]int64{"ssh": 0}}, false},
		{"DeprecatedFieldPositive", &agentproto.Stats{SessionCountSsh: 2}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, HasSessionCounts(tc.stats))
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
	ClearSessionCounts(st)
	require.Empty(t, sessionCountsFromProto(st))
	require.False(t, HasSessionCounts(st))
}
