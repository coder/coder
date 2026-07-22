package agentssh

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/coder/coder/v2/coderd/idemetadata"
	"github.com/coder/coder/v2/coderd/util/syncmap"
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

	s := &Server{sessionCounts: syncmap.New[string, *atomic.Int64]()}

	// Counters are created dynamically per session type, including types
	// unknown to this build of the agent.
	s.getOrCreateSessionCounter(MagicSessionTypeSSH).Add(1)
	s.getOrCreateSessionCounter(MagicSessionTypeVSCode).Add(1)
	s.getOrCreateSessionCounter(MagicSessionType("Cursor")).Add(2)
	require.Equal(t, map[string]int64{
		"ssh":    1,
		"vscode": 1,
		"Cursor": 2,
	}, s.SessionCounts())

	// The same counter instance is reused per type, and idle types are
	// omitted from the snapshot.
	s.getOrCreateSessionCounter(MagicSessionType("Cursor")).Add(-2)
	require.Equal(t, map[string]int64{
		"ssh":    1,
		"vscode": 1,
	}, s.SessionCounts())
}

func TestSessionCountsCapped(t *testing.T) {
	t.Parallel()

	s := &Server{sessionCounts: syncmap.New[string, *atomic.Int64]()}
	for i := range idemetadata.MaxSessionCountEntries {
		s.getOrCreateSessionCounter(MagicSessionType(fmt.Sprintf("ide-%03d", i))).Add(1)
	}
	// Once the cap is reached, new unrecognized session types share the
	// unknown counter.
	s.getOrCreateSessionCounter(MagicSessionType("one-too-many")).Add(1)
	s.getOrCreateSessionCounter(MagicSessionType("two-too-many")).Add(1)

	counts := s.SessionCounts()
	require.Len(t, counts, idemetadata.MaxSessionCountEntries+1)
	require.EqualValues(t, 2, counts[idemetadata.AppNameUnknown])
	require.NotContains(t, counts, "one-too-many")

	// Known-family types bypass the cap and keep their own counter.
	s.getOrCreateSessionCounter(MagicSessionTypeSSH).Add(1)
	require.EqualValues(t, 1, s.SessionCounts()["ssh"])

	// Existing counters keep working at the cap.
	s.getOrCreateSessionCounter(MagicSessionType("ide-000")).Add(1)
	require.EqualValues(t, 2, s.SessionCounts()["ide-000"])
}
