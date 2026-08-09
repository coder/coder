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
		{"NoEnvDefaultsToSSH", []string{"FOO=bar"}, MagicSessionTypeSSH},
		{"EmptyValueDefaultsToSSH", envWith(""), MagicSessionTypeSSH},
		{"VSCode", envWith("vscode"), MagicSessionTypeVSCode},
		{"JetBrainsLegacyCasing", envWith("JetBrains"), MagicSessionTypeJetBrains},
		{"UnknownTypeCanonicalized", envWith("Cursor-Nightly"), MagicSessionType("cursor_nightly")},
		{"LastInstanceWins", append(envWith("vscode"), MagicSessionTypeEnvironmentVariable+"=cursor"), MagicSessionType("cursor")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sessionType, _, filteredEnv := extractMagicSessionType(tc.env)
			require.Equal(t, tc.want, sessionType)
			for _, kv := range filteredEnv {
				require.NotContains(t, kv, MagicSessionTypeEnvironmentVariable+"=")
			}
		})
	}
}

func TestMagicTypeMetricLabel(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		magicType MagicSessionType
		want      string
	}{
		{"SSH", MagicSessionTypeSSH, idemetadata.AppNameSSH},
		{"ForkUsesFamily", MagicSessionType("cursor"), idemetadata.AppNameVSCode},
		// No alias for the nightly variant, so it does not get a label.
		{"UnlistedVariant", MagicSessionType("cursor_nightly"), idemetadata.AppNameUnknown},
		{"Unknown", MagicSessionType("some_new_ide"), idemetadata.AppNameUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, magicTypeMetricLabel(tc.magicType))
		})
	}
}

func TestSessionCounts(t *testing.T) {
	t.Parallel()

	s := &Server{sessionCounts: make(map[string]int64)}

	// Counters are created on demand and names are canonicalized.
	_ = s.startSession(MagicSessionTypeSSH)
	_ = s.startSession(MagicSessionTypeVSCode)
	endCursor1 := s.startSession(MagicSessionType("Cursor"))
	endCursor2 := s.startSession(MagicSessionType("cursor"))
	require.Equal(t, map[string]int64{
		idemetadata.AppNameSSH:    1,
		idemetadata.AppNameVSCode: 1,
		"cursor":                  2,
	}, s.SessionCounts())

	// Zero-count entries are dropped, not reported as idle apps.
	endCursor1()
	endCursor2()
	require.Equal(t, map[string]int64{
		idemetadata.AppNameSSH:    1,
		idemetadata.AppNameVSCode: 1,
	}, s.SessionCounts())
}
