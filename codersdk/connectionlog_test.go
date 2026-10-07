package codersdk_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestConnectionTypeOfApp(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		appName string
		want    codersdk.ConnectionType
	}{
		{"App", "cursor", codersdk.ConnectionTypeVSCode},
		{"AppNamedLikeItsFamily", "jetbrains", codersdk.ConnectionTypeJetBrains},
		{"JetBrainsIDE", "goland", codersdk.ConnectionTypeJetBrains},
		{"Normalized", "Code-Server", codersdk.ConnectionTypeVSCode},
		// Every other identity, including an absent one, is plain SSH.
		{"Absent", "", codersdk.ConnectionTypeSSH},
		{"SSHFamily", "zed", codersdk.ConnectionTypeSSH},
		{"Unregistered", "an_unregistered_ide", codersdk.ConnectionTypeSSH},
		// Clients pick their own names, so this is just an unregistered app.
		{"WebTypeName", "tunnel", codersdk.ConnectionTypeSSH},
		{"SFTP", "sftp", codersdk.ConnectionTypeSSH},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, codersdk.ConnectionTypeOfApp(tc.appName))
		})
	}
}

func TestConnectionTypeAppNames(t *testing.T) {
	t.Parallel()

	require.Empty(t, codersdk.ConnectionTypeTunnel.AppNames())
	require.Empty(t, codersdk.ConnectionType("cursor").AppNames())

	// The families grow, so check membership rather than the list.
	vscode := codersdk.ConnectionTypeVSCode.AppNames()
	require.Contains(t, vscode, "vscode")
	require.Contains(t, vscode, "cursor")
	require.NotContains(t, vscode, "jetbrains")

	jetbrains := codersdk.ConnectionTypeJetBrains.AppNames()
	require.Contains(t, jetbrains, "jetbrains")
	require.Contains(t, jetbrains, "goland")
	require.NotContains(t, jetbrains, "vscode")

	ssh := codersdk.ConnectionTypeSSH.AppNames()
	require.Contains(t, ssh, "ssh")
	require.Contains(t, ssh, "zed")

	// Each registered identity belongs to exactly the type it maps to.
	seen := map[string]codersdk.ConnectionType{}
	for _, typ := range codersdk.FilterableConnectionTypes() {
		for _, appName := range typ.AppNames() {
			require.Equal(t, typ, codersdk.ConnectionTypeOfApp(appName), appName)
			prev, dup := seen[appName]
			require.False(t, dup, "%q listed under %q and %q", appName, prev, typ)
			seen[appName] = typ
		}
	}
}

func TestConnectionTypeDisplayName(t *testing.T) {
	t.Parallel()

	for typ, want := range map[codersdk.ConnectionType]string{
		codersdk.ConnectionTypeSSH:             "SSH",
		codersdk.ConnectionTypeVSCode:          "VS Code Family",
		codersdk.ConnectionTypeJetBrains:       "JetBrains",
		codersdk.ConnectionTypeReconnectingPTY: "Web Terminal",
		codersdk.ConnectionTypeWorkspaceApp:    "Workspace App",
	} {
		require.Equal(t, want, typ.DisplayName(), typ)
	}
}

// The type filter accepts the legacy types only. Unknown never shipped.
func TestFilterableConnectionTypes(t *testing.T) {
	t.Parallel()

	declared := []codersdk.ConnectionType{
		codersdk.ConnectionTypeSSH,
		codersdk.ConnectionTypeVSCode,
		codersdk.ConnectionTypeJetBrains,
		codersdk.ConnectionTypeReconnectingPTY,
		codersdk.ConnectionTypeWorkspaceApp,
		codersdk.ConnectionTypePortForwarding,
		codersdk.ConnectionTypeTunnel,
	}
	require.ElementsMatch(t, declared, codersdk.FilterableConnectionTypes())

	for _, typ := range declared {
		require.True(t, typ.Valid(), typ)
		require.NotEqual(t, string(typ), typ.DisplayName(), "ConnectionType %q has no display name", typ)
	}
	require.False(t, codersdk.ConnectionType("unknown").Valid())
	require.False(t, codersdk.ConnectionType("cursor").Valid())
}

func TestConnectionLogMethodValid(t *testing.T) {
	t.Parallel()

	for _, m := range []codersdk.ConnectionLogMethod{
		codersdk.ConnectionLogMethodSSH,
		codersdk.ConnectionLogMethodReconnectingPTY,
		codersdk.ConnectionLogMethodWorkspaceApp,
		codersdk.ConnectionLogMethodPortForwarding,
		codersdk.ConnectionLogMethodTunnel,
	} {
		require.True(t, m.Valid(), m)
	}
	for _, m := range []codersdk.ConnectionLogMethod{"", "vscode", "jetbrains", "unknown"} {
		require.False(t, m.Valid(), m)
	}
}

// The deprecated type stays a plain string, and an absent app and its display
// name are omitted.
func TestConnectionLogJSON(t *testing.T) {
	t.Parallel()

	encode := func(l codersdk.ConnectionLog) map[string]any {
		raw, err := json.Marshal(l)
		require.NoError(t, err)
		var out map[string]any
		require.NoError(t, json.Unmarshal(raw, &out))
		return out
	}

	got := encode(codersdk.ConnectionLog{
		Type:             "ssh",
		ConnectionMethod: codersdk.ConnectionLogMethodSSH,
	})
	require.Equal(t, "ssh", got["type"])
	require.Equal(t, "ssh", got["connection_method"])
	require.NotContains(t, got, "app_name")
	require.NotContains(t, got, "app_display_name")

	got = encode(codersdk.ConnectionLog{
		Type:             "vscode",
		ConnectionMethod: codersdk.ConnectionLogMethodSSH,
		AppName:          "cursor",
		AppDisplayName:   "Cursor",
	})
	require.Equal(t, "vscode", got["type"])
	require.Equal(t, "cursor", got["app_name"])
	require.Equal(t, "Cursor", got["app_display_name"])
}
