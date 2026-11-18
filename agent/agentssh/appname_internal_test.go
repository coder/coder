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
			fmt.Sprintf("%s=%s", AppNameEnvironmentVariable, value),
			"BAZ=qux",
		}
	}

	for _, tc := range []struct {
		name       string
		env        []string
		rawCommand string
		// keepEnv, when set, must survive filtering.
		keepEnv string
		want    string
	}{
		{"NoEnvDefaultsToSSH", []string{"FOO=bar"}, "", "FOO=bar", "ssh"},
		{"EmptyValueDefaultsToSSH", envWith(""), "", "", "ssh"},
		{"VSCode", envWith("vscode"), "", "", "vscode"},
		{"JetBrainsLegacyCasing", envWith("JetBrains"), "", "", "jetbrains"},
		{"UnknownTypeNormalized", envWith("Cursor-Nightly"), "", "", "cursor_nightly"},
		{"LastInstanceWins", append(envWith("vscode"), AppNameEnvironmentVariable+"=cursor"), "", "", "cursor"},
		// Command shapes Zed sends over SSH (crates/remote/src/transport/ssh.rs).
		{"ZedProxy", nil, "cd; env RUST_LOG=info .zed_server/zed-remote-server-stable-1.28.1+stable.390.e8bc16ac proxy --identifier workspace-2 --reconnect", "", "zed"},
		{"ZedProxyQuotedPath", nil, "cd; env '.zed_server/zed-remote-server-preview-0.201.0' proxy --identifier 'abc-1'", "", "zed"},
		{"ZedVersionProbeIsSSH", nil, "cd; .zed_server/zed-remote-server-stable-0.200.0 version", "", "ssh"},
		{"ExplicitAppNameWinsOverZed", envWith("vscode"), "cd; env .zed_server/zed-remote-server-stable-0.200.0 proxy --identifier abc", "", "vscode"},
		// A variable that merely starts with the same characters is a
		// different variable, not an app name.
		{
			"LongerVariableIgnored",
			[]string{AppNameEnvironmentVariable + "_FOO=bar"},
			"",
			AppNameEnvironmentVariable + "_FOO=bar",
			"ssh",
		},
		{
			"LongerVariableDoesNotShadowAppName",
			append(envWith("cursor"), AppNameEnvironmentVariable+"_FOO=bar"),
			"",
			AppNameEnvironmentVariable + "_FOO=bar",
			"cursor",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			appName, _, filteredEnv := extractAppName(tc.env, tc.rawCommand)
			require.Equal(t, tc.want, appName)
			for _, kv := range filteredEnv {
				require.NotContains(t, kv, AppNameEnvironmentVariable+"=")
			}
			if tc.keepEnv != "" {
				require.Contains(t, filteredEnv, tc.keepEnv)
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
