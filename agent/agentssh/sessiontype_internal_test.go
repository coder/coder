package agentssh

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/idemetadata"
)

func TestExtractMagicSessionType(t *testing.T) {
	t.Parallel()

	envWith := func(value string) []string {
		return []string{
			"FOO=bar",
			fmt.Sprintf("%s=%s", MagicSessionTypeEnvironmentVariable, value),
			"BAZ=qux",
		}
	}

	for _, tc := range []struct {
		name string
		env  []string
		want MagicSessionType
	}{
		{name: "NoEnvDefaultsToSSH", env: []string{"FOO=bar"}, want: MagicSessionTypeSSH},
		{name: "EmptyValueDefaultsToSSH", env: envWith(""), want: MagicSessionTypeSSH},
		{name: "VSCode", env: envWith("vscode"), want: MagicSessionTypeVSCode},
		{name: "JetBrainsLegacyCasing", env: envWith("JetBrains"), want: MagicSessionTypeJetBrains},
		{name: "UnknownFoldedToCanonicalForm", env: envWith("Cursor Nightly"), want: MagicSessionType("cursor nightly")},
		{name: "LastInstanceWins", env: append(envWith("vscode"), MagicSessionTypeEnvironmentVariable+"=cursor"), want: MagicSessionType("cursor")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sessionType, _, filteredEnv := extractMagicSessionType(tc.env)
			require.Equal(t, tc.want, sessionType)
			for _, kv := range filteredEnv {
				require.NotContains(t, kv, MagicSessionTypeEnvironmentVariable+"=", "env should be stripped")
			}
		})
	}
}

func TestMagicTypeMetricLabel(t *testing.T) {
	t.Parallel()

	// The label carries the family, not the raw type, so a divergent type
	// like cursor maps to vscode.
	require.Equal(t, "vscode", magicTypeMetricLabel(MagicSessionType("cursor")))
	require.Equal(t, "ssh", magicTypeMetricLabel(MagicSessionTypeSSH))
	require.Equal(t, idemetadata.AppNameUnknown, magicTypeMetricLabel(MagicSessionType("some-new-ide")))
}

func TestSessionCounts(t *testing.T) {
	t.Parallel()

	s := &Server{sessionCounts: make(map[string]int64)}

	// Counters are created dynamically per session type, including types
	// unknown to this build of the agent.
	_ = s.startSession(MagicSessionTypeSSH)
	_ = s.startSession(MagicSessionTypeVSCode)
	endCursor1 := s.startSession(MagicSessionType("Cursor"))
	endCursor2 := s.startSession(MagicSessionType("Cursor"))
	require.Equal(t, map[string]int64{
		"ssh":    1,
		"vscode": 1,
		"Cursor": 2,
	}, s.SessionCounts())

	// Ending a session decrements its type; zero-count entries are removed
	// from the snapshot.
	endCursor1()
	endCursor2()
	require.Equal(t, map[string]int64{
		"ssh":    1,
		"vscode": 1,
	}, s.SessionCounts())
}

func TestSessionCountsCapped(t *testing.T) {
	t.Parallel()

	s := &Server{sessionCounts: make(map[string]int64)}
	for i := range idemetadata.MaxSessionCountEntries {
		_ = s.startSession(MagicSessionType(fmt.Sprintf("ide-%03d", i)))
	}
	// Once the cap is reached, new unrecognized session types are counted
	// under unknown.
	endOverflow := s.startSession(MagicSessionType("one-too-many"))
	_ = s.startSession(MagicSessionType("two-too-many"))

	counts := s.SessionCounts()
	require.Len(t, counts, idemetadata.MaxSessionCountEntries+1)
	require.EqualValues(t, 2, counts[idemetadata.AppNameUnknown])
	require.NotContains(t, counts, "one-too-many")

	// Known-family types bypass the cap and keep their own counter.
	_ = s.startSession(MagicSessionTypeSSH)
	require.EqualValues(t, 1, s.SessionCounts()["ssh"])

	// Existing counters keep working at the cap.
	_ = s.startSession(MagicSessionType("ide-000"))
	require.EqualValues(t, 2, s.SessionCounts()["ide-000"])

	// Ending a session that was counted under unknown decrements unknown;
	// the cap applies to concurrently active types, not every type ever
	// seen.
	endOverflow()
	require.EqualValues(t, 1, s.SessionCounts()[idemetadata.AppNameUnknown])
}
