package workspacestats

import (
	"testing"

	"github.com/stretchr/testify/require"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk"
)

func TestSessionCountsFromProto(t *testing.T) {
	t.Parallel()

	const (
		ssh  = codersdk.ConnectionLogMethodSSH
		rpty = codersdk.ConnectionLogMethodReconnectingPTY
		pf   = codersdk.ConnectionLogMethodPortForwarding
	)
	// The deprecated fields simulate agents older than API v2.11.
	for _, tc := range []struct {
		name  string
		stats *agentproto.Stats
		want  map[codersdk.ConnectionLogMethod]map[string]int64
	}{
		{
			"Empty",
			&agentproto.Stats{},
			map[codersdk.ConnectionLogMethod]map[string]int64{},
		},
		{
			"NormalizesAndMergesNames",
			&agentproto.Stats{SessionCounts: map[string]int64{"VSCode": 1, "vscode": 2, "Reconnecting-PTY": 1}},
			map[codersdk.ConnectionLogMethod]map[string]int64{ssh: {"vscode": 3}, rpty: {"reconnecting_pty": 1}},
		},
		{
			"DerivesMethodFromName",
			&agentproto.Stats{SessionCounts: map[string]int64{"port-forwarding": 2, "cursor": 1}},
			map[codersdk.ConnectionLogMethod]map[string]int64{pf: {"port_forwarding": 2}, ssh: {"cursor": 1}},
		},
		{
			"DropsNonPositiveEntries",
			&agentproto.Stats{SessionCounts: map[string]int64{"vscode": 1, "reconnecting_pty": 0, "bogus": -1}},
			map[codersdk.ConnectionLogMethod]map[string]int64{ssh: {"vscode": 1}},
		},
		{
			"ReportsClientOverflowAsUnknown",
			&agentproto.Stats{SessionCounts: map[string]int64{"Overflow": 2, "unknown": 1}},
			map[codersdk.ConnectionLogMethod]map[string]int64{ssh: {"unknown": 3}},
		},
		{
			"MapTakesPrecedenceOverDeprecatedFields",
			&agentproto.Stats{SessionCounts: map[string]int64{"cursor": 2, "ssh": 1}, SessionCountVscode: 9},
			map[codersdk.ConnectionLogMethod]map[string]int64{ssh: {"cursor": 2, "ssh": 1}},
		},
		{
			"OldAgentFallback",
			&agentproto.Stats{SessionCountVscode: 3, SessionCountJetbrains: 1, SessionCountSsh: 2},
			map[codersdk.ConnectionLogMethod]map[string]int64{ssh: {"vscode": 3, "jetbrains": 1, "ssh": 2}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, normalizedSessionCounts(tc.stats))
		})
	}
}

func TestHasSessionCounts(t *testing.T) {
	t.Parallel()

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

	st := &agentproto.Stats{
		SessionCounts:      map[string]int64{"vscode": 1},
		SessionCountVscode: 1,
		SessionCountSsh:    2,
	}
	ClearSessionCounts(st)
	require.Empty(t, normalizedSessionCounts(st))
	require.False(t, HasSessionCounts(st))
}
