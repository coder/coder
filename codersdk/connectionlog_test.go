package codersdk_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestConnectionTypeMatchingTypes(t *testing.T) {
	t.Parallel()

	t.Run("FamilyMatchesItsApps", func(t *testing.T) {
		t.Parallel()

		got := codersdk.ConnectionTypeVSCode.MatchingTypes()
		require.Contains(t, got, "vscode")
		require.Contains(t, got, "cursor")
		require.Contains(t, got, "windsurf")
		require.NotContains(t, got, "jetbrains")
	})

	t.Run("SSHFamilyCoversZed", func(t *testing.T) {
		t.Parallel()

		require.Contains(t, codersdk.ConnectionTypeSSH.MatchingTypes(), "zed")
	})

	// A type with no apps under it, so it matches only itself.
	t.Run("WebTypeMatchesItself", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, []string{"tunnel"}, codersdk.ConnectionTypeTunnel.MatchingTypes())
	})

	t.Run("EmptyFiltersNothing", func(t *testing.T) {
		t.Parallel()

		require.Nil(t, codersdk.ConnectionType("").MatchingTypes())
	})
}

func TestConnectionTypeDisplayName(t *testing.T) {
	t.Parallel()

	for typ, want := range map[codersdk.ConnectionType]string{
		codersdk.ConnectionTypeVSCode:          "VS Code",
		codersdk.ConnectionTypeSSH:             "SSH",
		codersdk.ConnectionTypeReconnectingPTY: "Web Terminal",
		codersdk.ConnectionTypeWorkspaceApp:    "Workspace App",
		codersdk.ConnectionTypePortForwarding:  "Port Forwarding",
		codersdk.ConnectionTypeTunnel:          "Tunnel",
		"cursor":                               "Cursor",
		"code_server":                          "code-server",
		"an_unregistered_ide":                  "an_unregistered_ide",
	} {
		require.Equal(t, want, typ.DisplayName(), "type %q", typ)
	}
}
