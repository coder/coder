package agentssh

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractAppName(t *testing.T) {
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
		{"NoEnvDefaultsToSSH", []string{"FOO=bar"}, "ssh"},
		{"EmptyValueDefaultsToSSH", envWith(""), "ssh"},
		{"VSCode", envWith("vscode"), "vscode"},
		{"JetBrainsLegacyCasing", envWith("JetBrains"), "jetbrains"},
		{"UnknownTypeNormalized", envWith("Cursor-Nightly"), "cursor_nightly"},
		{"LastInstanceWins", append(envWith("vscode"), MagicSessionTypeEnvironmentVariable+"=cursor"), "cursor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			appName, _, filteredEnv := extractAppName(tc.env)
			require.Equal(t, tc.want, appName)
			for _, kv := range filteredEnv {
				require.NotContains(t, kv, MagicSessionTypeEnvironmentVariable+"=")
			}
		})
	}
}

func TestAppNameMetricLabel(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		appName string
		want    string
	}{
		{"SSH", "ssh", "ssh"},
		{"ForkUsesFamily", "cursor", "vscode"},
		// No alias for the nightly variant, so it does not get a label.
		{"UnlistedVariant", "cursor_nightly", "unknown"},
		{"Unknown", "some_new_ide", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, appNameMetricLabel(tc.appName))
		})
	}
}

func TestSessionCounts(t *testing.T) {
	t.Parallel()

	s := &Server{sessionCounts: make(map[string]int64)}

	// Counters are created on demand and names are normalized.
	_ = s.startSession("ssh")
	_ = s.startSession("vscode")
	endCursor1 := s.startSession("Cursor")
	endCursor2 := s.startSession("cursor")
	require.Equal(t, map[string]int64{
		"ssh":    1,
		"vscode": 1,
		"cursor": 2,
	}, s.SessionCounts())

	// Zero-count entries are dropped, not reported as idle apps.
	endCursor1()
	endCursor2()
	require.Equal(t, map[string]int64{
		"ssh":    1,
		"vscode": 1,
	}, s.SessionCounts())
}
