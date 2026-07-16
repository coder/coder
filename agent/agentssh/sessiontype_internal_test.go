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

func TestConnCounts(t *testing.T) {
	t.Parallel()

	s := &Server{}

	// Counters are created dynamically per session type, including types
	// unknown to this build of the agent.
	s.getOrCreateConnCounter(MagicSessionTypeSSH).Add(1)
	s.getOrCreateConnCounter(MagicSessionTypeVSCode).Add(1)
	s.getOrCreateConnCounter(MagicSessionType("Cursor")).Add(2)
	require.Equal(t, map[string]int64{
		"ssh":    1,
		"vscode": 1,
		"Cursor": 2,
	}, s.SessionCounts())

	// The same counter instance is reused per type, and idle types are
	// omitted from the snapshot.
	s.getOrCreateConnCounter(MagicSessionType("Cursor")).Add(-2)
	require.Equal(t, map[string]int64{
		"ssh":    1,
		"vscode": 1,
	}, s.SessionCounts())
}

func TestConnCountsCapped(t *testing.T) {
	t.Parallel()

	s := &Server{}
	for i := range maxSessionTypes {
		s.getOrCreateConnCounter(MagicSessionType(fmt.Sprintf("ide-%03d", i))).Add(1)
	}
	// Once the cap is reached, new session types share the unknown counter.
	s.getOrCreateConnCounter(MagicSessionType("one-too-many")).Add(1)
	s.getOrCreateConnCounter(MagicSessionType("two-too-many")).Add(1)

	counts := s.SessionCounts()
	require.Len(t, counts, maxSessionTypes+1)
	require.EqualValues(t, 2, counts[idemetadata.AppNameUnknown])
	require.NotContains(t, counts, "one-too-many")

	// Existing counters keep working at the cap.
	s.getOrCreateConnCounter(MagicSessionType("ide-000")).Add(1)
	require.EqualValues(t, 2, s.SessionCounts()["ide-000"])
}
