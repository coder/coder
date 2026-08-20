package agentssh

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
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
		want string
	}{
		{"NoEnvDefaultsToSSH", []string{"FOO=bar"}, codersdk.AppNameSSH},
		{"EmptyValueDefaultsToSSH", envWith(""), codersdk.AppNameSSH},
		{"VSCode", envWith("vscode"), codersdk.AppNameVSCode},
		{"JetBrainsLegacyCasing", envWith("JetBrains"), codersdk.AppNameJetBrains},
		{"UnknownTypeNormalized", envWith("Cursor-Nightly"), ("cursor_nightly")},
		{"LastInstanceWins", append(envWith("vscode"), MagicSessionTypeEnvironmentVariable+"=cursor"), ("cursor")},
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
		magicType string
		want      string
	}{
		{"SSH", codersdk.AppNameSSH, codersdk.AppNameSSH},
		{"ForkUsesFamily", ("cursor"), codersdk.AppNameVSCode},
		// No alias for the nightly variant, so it does not get a label.
		{"UnlistedVariant", ("cursor_nightly"), codersdk.AppNameUnknown},
		{"Unknown", ("some_new_ide"), codersdk.AppNameUnknown},
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

	// Counters are created on demand and names are normalized.
	_ = s.startSession(codersdk.AppNameSSH)
	_ = s.startSession(codersdk.AppNameVSCode)
	endCursor1 := s.startSession(("Cursor"))
	endCursor2 := s.startSession(("cursor"))
	require.Equal(t, map[string]int64{
		codersdk.AppNameSSH:    1,
		codersdk.AppNameVSCode: 1,
		"cursor":               2,
	}, s.SessionCounts())

	// Zero-count entries are dropped, not reported as idle apps.
	endCursor1()
	endCursor2()
	require.Equal(t, map[string]int64{
		codersdk.AppNameSSH:    1,
		codersdk.AppNameVSCode: 1,
	}, s.SessionCounts())
}
